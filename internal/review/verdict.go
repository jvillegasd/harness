package review

import (
	"fmt"
	"github.com/jvillegasd/harness/internal/run"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var Results = []string{"PASS", "PASS+NOTES", "FAIL"}

type Verdict struct {
	Run    bool   `json:"run,omitempty"`
	Result string `json:"result"`
	Base   string `json:"base,omitempty"`
	SHA    string `json:"sha"`
	Level  string `json:"level,omitempty"`
}

var (
	markerRe    = regexp.MustCompile(`\A<!-- harness-review verdict=(PASS|PASS\+NOTES|FAIL) base=([0-9a-f]{40}) sha=([0-9a-f]{40}) level=(medium|high) -->\r?\n`)
	runMarkerRe = regexp.MustCompile(`\A<!-- harness-run verdict=(PASS|PASS\+NOTES|FAIL) sha=([0-9a-f]{40}) -->\r?\n`)
)

func (v Verdict) Validate(notes string) error {
	full := regexp.MustCompile(`^[0-9a-f]{40}$`)
	switch {
	case !slices.Contains(Results, v.Result):
		return fmt.Errorf("%w: result %q is not one of %s", ErrInvalid, v.Result, strings.Join(Results, ", "))
	case v.Run && (v.Base != "" || v.Level != ""):
		return fmt.Errorf("%w: a run verdict names one commit, so it takes no --base or --level", ErrInvalid)
	case !v.Run && v.Level != "medium" && v.Level != "high":
		return fmt.Errorf("%w: level %q is not medium or high", ErrInvalid, v.Level)
	case !full.MatchString(v.SHA) || !v.Run && !full.MatchString(v.Base):
		return fmt.Errorf("%w: base and sha must be full commit ids", ErrInvalid)
	case v.Result != "PASS" && strings.TrimSpace(notes) == "":
		return fmt.Errorf("%w: a %s verdict needs --notes saying what the human should read", ErrInvalid, v.Result)
	}
	return nil
}

func Body(v Verdict, notes string) string {
	var b strings.Builder
	if v.Run {
		fmt.Fprintf(&b, "<!-- harness-run verdict=%s sha=%s -->\n", v.Result, v.SHA)
		fmt.Fprintf(&b, "## Run-as-user verdict: %s\n\nDrove `%s` as a user.\n", v.Result, v.SHA[:12])
	} else {
		fmt.Fprintf(&b, "<!-- harness-review verdict=%s base=%s sha=%s level=%s -->\n", v.Result, v.Base, v.SHA, v.Level)
		fmt.Fprintf(&b, "## Review verdict: %s\n\nReviewed `%s..%s` at level %s.\n", v.Result, v.Base[:12], v.SHA[:12], v.Level)
	}
	if notes = strings.TrimSpace(notes); notes != "" {
		fmt.Fprintf(&b, "\n%s\n", notes)
	}
	return b.String()
}

func Parse(body string) (Verdict, bool) {
	if m := markerRe.FindStringSubmatch(body); m != nil {
		return Verdict{Result: m[1], Base: m[2], SHA: m[3], Level: m[4]}, true
	}
	if m := runMarkerRe.FindStringSubmatch(body); m != nil {
		return Verdict{Run: true, Result: m[1], SHA: m[2]}, true
	}
	return Verdict{}, false
}

func PostVerdict(forge run.Forge, number int, body string) error {
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
