package review

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Results are the verdicts a review posts: PASS when nothing is left to report, PASS+NOTES when the change may land and the notes say what the human should still read, and FAIL when a confirmed finding is unfixed or part of the change went unreviewed.
var Results = []string{"PASS", "PASS+NOTES", "FAIL"}

// Verdict is one review's result on a pull request: what it read, base..sha as full commit ids, at which level, and what it found.
type Verdict struct {
	Result string `json:"result"`
	Base   string `json:"base"`
	SHA    string `json:"sha"`
	Level  string `json:"level"`
}

// markerRe is the hidden first line that makes a comment a verdict. Only a comment that starts with it counts, so quoting one inside another comment forges nothing.
var markerRe = regexp.MustCompile(`\A<!-- harness-review verdict=(PASS|PASS\+NOTES|FAIL) base=([0-9a-f]{40}) sha=([0-9a-f]{40}) level=(medium|high) -->\r?\n`)

// Validate checks a verdict before it is posted. Notes are required for PASS+NOTES and FAIL, since those results are only as useful as what they say.
func (v Verdict) Validate(notes string) error {
	full := regexp.MustCompile(`^[0-9a-f]{40}$`)
	switch {
	case !slices.Contains(Results, v.Result):
		return fmt.Errorf("%w: result %q is not one of %s", ErrInvalid, v.Result, strings.Join(Results, ", "))
	case v.Level != "medium" && v.Level != "high":
		return fmt.Errorf("%w: level %q is not medium or high", ErrInvalid, v.Level)
	case !full.MatchString(v.Base) || !full.MatchString(v.SHA):
		return fmt.Errorf("%w: base and sha must be full commit ids", ErrInvalid)
	case v.Result != "PASS" && strings.TrimSpace(notes) == "":
		return fmt.Errorf("%w: a %s verdict needs --notes saying what the human should read", ErrInvalid, v.Result)
	}
	return nil
}

// Body is the verdict comment: the hidden marker land check parses, a heading with the result, the reviewed range and level, and the notes as given.
func Body(v Verdict, notes string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- harness-review verdict=%s base=%s sha=%s level=%s -->\n", v.Result, v.Base, v.SHA, v.Level)
	fmt.Fprintf(&b, "## Review verdict: %s\n\nReviewed `%s..%s` at level %s.\n", v.Result, v.Base[:12], v.SHA[:12], v.Level)
	if notes = strings.TrimSpace(notes); notes != "" {
		fmt.Fprintf(&b, "\n%s\n", notes)
	}
	return b.String()
}

// Parse reads a comment body as a verdict, and reports false for any comment that does not start with the verdict marker.
func Parse(body string) (Verdict, bool) {
	m := markerRe.FindStringSubmatch(body)
	if m == nil {
		return Verdict{}, false
	}
	return Verdict{Result: m[1], Base: m[2], SHA: m[3], Level: m[4]}, true
}

// PostVerdict posts body on pull request number as one comment through forge, from a file so the text never passes through a shell.
func PostVerdict(forge Forge, number int, body string) error {
	tmp, err := os.MkdirTemp("", "harness-review-verdict-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "verdict.md")
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		return err
	}
	_, err = forge("pr", "comment", strconv.Itoa(number), "--body-file", file)
	return err
}
