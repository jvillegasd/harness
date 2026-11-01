// Package reviewledger keeps the local record of review findings and their dispositions. The ledger lives outside the repository and outside ~/.harness, so it never reaches a commit or pull request and survives `harness install --uninstall`.
package reviewledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is one ledger line. A finding's later dispositions are new lines with the same ID; the latest wins.
type Entry struct {
	ID          string  `json:"id"`
	TS          string  `json:"ts"`
	Branch      string  `json:"branch"`
	Base        string  `json:"base"`
	SHA         string  `json:"sha"`
	Digest      *string `json:"digest"`
	Round       int     `json:"round"`
	Depth       string  `json:"depth"`
	Axis        string  `json:"axis"`
	Reviewer    string  `json:"reviewer"`
	Path        *string `json:"path"`
	Lines       string  `json:"lines"`
	Severity    string  `json:"severity"`
	Class       string  `json:"class"`
	Summary     string  `json:"summary"`
	Disposition string  `json:"disposition"`
	Evidence    string  `json:"evidence"`
	// Rewritten and Open are computed by Latest, never stored.
	Rewritten bool `json:"rewritten,omitempty"`
	Open      bool `json:"open"`
}

var (
	dispositions = set("confirmed", "unverified", "fixed", "refuted", "intentional", "wont-matter", "pre-existing", "nit", "clean", "no-review")
	severities   = set("bug", "risk", "q", "pre-existing", "nit")
	depths       = set("none", "quick", "standard", "deep")
	linesRe      = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)
	separators   = regexp.MustCompile(`[:/]+`)
	// ErrInvalid marks a rejected entry, which the command reports as a usage error.
	ErrInvalid = errors.New("invalid entry")
)

func set(values ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range values {
		m[v] = true
	}
	return m
}

// Validate checks a new finding or review line before it is written.
func Validate(e Entry) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}
	switch {
	case !dispositions[e.Disposition]:
		return bad("disposition %q is not one of confirmed, unverified, fixed, refuted, intentional, wont-matter, pre-existing, nit, clean, no-review", e.Disposition)
	case !depths[e.Depth]:
		return bad("depth %q is not one of none, quick, standard, deep", e.Depth)
	case e.Round != 1 && e.Round != 2:
		return bad("round %d is not 1 or 2", e.Round)
	case e.Branch == "" || e.SHA == "":
		return bad("branch and sha are required")
	}
	whole := e.Disposition == "clean" || e.Disposition == "no-review"
	if whole {
		if e.Path != nil {
			return bad("a %s line describes the whole review and takes no path", e.Disposition)
		}
		return nil
	}
	switch {
	case e.Path == nil:
		return bad("a finding needs a path")
	case !severities[e.Severity]:
		return bad("severity %q is not one of bug, risk, q, pre-existing, nit", e.Severity)
	case e.Lines != "" && !linesRe.MatchString(e.Lines):
		return bad("lines %q is not a line or a range such as 40-52", e.Lines)
	case e.Summary == "":
		return bad("a finding needs a summary")
	}
	return nil
}

// Key turns a remote URL or a top-level path into a directory name that keeps the host, owner, and full repository path, so distinct remotes never share a ledger.
func Key(source string) string {
	s := strings.TrimSpace(source)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if at := strings.Index(s, "@"); at >= 0 && !strings.Contains(s[:at], "/") {
		s = s[at+1:]
	}
	s = strings.TrimSuffix(strings.TrimRight(s, "/"), ".git")
	s = separators.ReplaceAllString(s, "_")
	return strings.TrimPrefix(s, "_")
}

// Dir is the ledger directory for the repository at root: $XDG_DATA_HOME/harness/reviews/<key>, defaulting to ~/.local/share.
func Dir(root string) (string, error) {
	source, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		source = root
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "harness", "reviews", Key(source)), nil
}

// Append writes one line to the ledger file, creating its directory.
func Append(file string, e Entry) error {
	e.Rewritten, e.Open = false, false
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Read returns every line of the ledger in append order; a missing file is an empty ledger.
func Read(file string) ([]Entry, error) {
	f, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var entries []Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for n := 1; scanner.Scan(); n++ {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", file, n, err)
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

// Find returns the latest line for id.
func Find(entries []Entry, id string) (Entry, bool) {
	for i := len(entries) - 1; i >= 0; i-- {
		if entries[i].ID == id {
			return entries[i], true
		}
	}
	return Entry{}, false
}

// Latest returns the latest line of each finding on branch, in first-seen order. A finding is open when its latest disposition is confirmed or unverified. isAncestor reports whether a reviewed commit is still in the branch's history; a finding whose commit is not stays in the result, marked rewritten, because a rebase does not settle it.
func Latest(entries []Entry, branch string, isAncestor func(sha string) bool) []Entry {
	index := map[string]int{}
	var out []Entry
	for _, e := range entries {
		if e.Branch != branch || e.Path == nil {
			continue
		}
		if i, ok := index[e.ID]; ok {
			out[i] = e
			continue
		}
		index[e.ID] = len(out)
		out = append(out, e)
	}
	for i := range out {
		out[i].Open = out[i].Disposition == "confirmed" || out[i].Disposition == "unverified"
		out[i].Rewritten = !isAncestor(out[i].SHA)
	}
	return out
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
