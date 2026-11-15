package land

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ErrMergeRefused is a merge refused before anything was written to the forge.
var ErrMergeRefused = errors.New("refused")

// Merged is a pull request as read back after its squash merge.
type Merged struct {
	Number      int    `json:"number"`
	Head        string `json:"head"`
	State       string `json:"state"`
	MergeCommit string `json:"mergeCommit"`
}

var (
	tradeoffsHeadRe = regexp.MustCompile(`(?m)^## Tradeoffs\s*$`)
	nextHeadRe      = regexp.MustCompile(`(?m)^## `)
)

// tradeoffs returns a description's `## Tradeoffs` section, heading included, or "" when it has none.
func tradeoffs(body string) string {
	loc := tradeoffsHeadRe.FindStringIndex(body)
	if loc == nil {
		return ""
	}
	rest := body[loc[1]:]
	if next := nextHeadRe.FindStringIndex(rest); next != nil {
		rest = rest[:next[0]]
	}
	return strings.TrimSpace(body[loc[0]:loc[1]] + rest)
}

// MergeBody is the squash commit body: the given text, with the description's tradeoffs appended when the text carries none.
func MergeBody(text, description string) string {
	text = strings.TrimSpace(text)
	if t := tradeoffs(description); t != "" && !tradeoffsHeadRe.MatchString(text) {
		text += "\n\n" + t
	}
	return text + "\n"
}

// Merge squash-merges one open pull request at the head the caller checked. It checks the pull request again first and refuses, writing nothing, when it is not on the open lane, its head moved, a forge gate holds it, or another hold stands without accepted. It never deletes the head branch, and it reports the merge only once the forge reads the pull request back as merged.
func Merge(e Env, number int, head, bodyFile string, accepted bool) (Merged, error) {
	m := Merged{Number: number, Head: head}
	text, err := os.ReadFile(bodyFile)
	if err != nil {
		return m, err
	}
	if strings.TrimSpace(string(text)) == "" {
		return m, fmt.Errorf("%w: %s is empty; the squash body carries the resulting behavior and assumptions", ErrMergeRefused, bodyFile)
	}
	checks, err := Run(e, []int{number}, false, true)
	if err != nil {
		return m, err
	}
	c := checks[0]
	refuse := func(format string, args ...any) (Merged, error) {
		return m, fmt.Errorf("%w: #%d "+format, append([]any{ErrMergeRefused, number}, args...)...)
	}
	switch {
	case c.Lane != "open":
		return refuse("is on the %s lane: %s", c.Lane, strings.Join(c.Holds, "; "))
	case c.Head != head:
		return refuse("has head %s, not the checked %s: check it again", c.Head, head)
	case len(c.gate.holds) > 0:
		return refuse("is held by the forge, which no acceptance clears: %s", strings.Join(c.gate.holds, "; "))
	case len(c.Holds) > 0 && !accepted:
		return refuse("has holds only the human can accept; pass --accepted once they have, by name: %s", strings.Join(c.Holds, "; "))
	}

	tmp, err := os.MkdirTemp("", "harness-merge-")
	if err != nil {
		return m, err
	}
	defer os.RemoveAll(tmp)
	file := filepath.Join(tmp, "body.md")
	if err := os.WriteFile(file, []byte(MergeBody(string(text), c.body)), 0o644); err != nil {
		return m, err
	}
	// --delete-branch is left out: its local checkout fails when another worktree holds the default branch, which reads as a failed merge.
	_, mergeErr := e.Forge("pr", "merge", strconv.Itoa(number), "--squash", "--match-head-commit", head, "--subject", c.Title, "--body-file", file)

	var back struct {
		State       string
		MergeCommit struct{ Oid string }
	}
	if err := e.read(&back, "pr", "view", strconv.Itoa(number), "--json", "state,mergeCommit"); err != nil {
		return m, errors.Join(mergeErr, err)
	}
	m.State, m.MergeCommit = back.State, back.MergeCommit.Oid
	if m.State != "MERGED" {
		return m, errors.Join(mergeErr, fmt.Errorf("#%d reads back as %s, not MERGED", number, m.State))
	}
	return m, nil
}
