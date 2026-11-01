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
	"slices"
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
	// Commit is the fixing commit of a fixed finding.
	Commit string `json:"commit,omitempty"`
}

// Finding is a finding's latest line as `list` reports it, with the state computed against the head.
type Finding struct {
	Entry
	// Open holds the merge: confirmed, unverified, or fixed by a commit the head does not contain.
	Open bool `json:"open"`
	// Rewritten marks a reviewed commit that is no longer in the head's history, as after a rebase.
	Rewritten bool `json:"rewritten,omitempty"`
}

var (
	dispositions = []string{"confirmed", "unverified", "fixed", "refuted", "intentional", "wont-matter", "pre-existing", "nit", "clean", "no-review"}
	severities   = []string{"bug", "risk", "q", "pre-existing", "nit"}
	depths       = []string{"none", "quick", "standard", "deep"}
	// needEvidence are the dispositions that close a finding on someone's word, so the word must be recorded.
	needEvidence = []string{"fixed", "refuted", "intentional", "wont-matter"}
	linesRe      = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)
	separators   = regexp.MustCompile(`[:/]+`)
	// ErrInvalid marks a rejected entry; tests use it to tell a rejection from an I/O failure.
	ErrInvalid = errors.New("invalid entry")
)

func oneOf(field, value string, allowed []string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return fmt.Errorf("%w: %s %q is not one of %s", ErrInvalid, field, value, strings.Join(allowed, ", "))
}

// Validate checks a line before it is written.
func Validate(e Entry) error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}
	if err := oneOf("disposition", e.Disposition, dispositions); err != nil {
		return err
	}
	if err := oneOf("depth", e.Depth, depths); err != nil {
		return err
	}
	switch {
	case e.Round != 1 && e.Round != 2:
		return bad("round %d is not 1 or 2", e.Round)
	case e.Branch == "" || e.SHA == "":
		return bad("branch and sha are required")
	case slices.Contains(needEvidence, e.Disposition) && e.Evidence == "":
		return bad("a %s disposition needs --evidence", e.Disposition)
	case e.Disposition == "fixed" && e.Commit == "":
		return bad("a fixed disposition needs --commit, the fixing commit")
	}
	if e.Disposition == "clean" || e.Disposition == "no-review" {
		if e.Path != nil {
			return bad("a %s line describes the whole review and takes no path", e.Disposition)
		}
		return nil
	}
	if err := oneOf("severity", e.Severity, severities); err != nil {
		return err
	}
	switch {
	case e.Path == nil:
		return bad("a finding needs a path")
	case e.Lines != "" && !linesRe.MatchString(e.Lines):
		return bad("lines %q is not a line or a range such as 40-52", e.Lines)
	case e.Summary == "":
		return bad("a finding needs a summary")
	case (e.Disposition == "nit" || e.Disposition == "pre-existing") && e.Severity != e.Disposition:
		return bad("only a %s finding can take the %s disposition", e.Disposition, e.Disposition)
	}
	return nil
}

// Key turns a remote URL or a repository path into a directory name that keeps the host, owner, and full repository path. Underscores are doubled before separators become single underscores, so distinct sources never share a key.
func Key(source string) string {
	s := strings.TrimSpace(source)
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if at := strings.Index(s, "@"); at >= 0 && !strings.Contains(s[:at], "/") {
		s = s[at+1:]
	}
	s = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(s, "/"), ".git"), "/")
	s = strings.ReplaceAll(s, "_", "__")
	s = separators.ReplaceAllString(s, "_")
	return strings.TrimPrefix(s, "_")
}

// Dir is the ledger directory for the repository at root: $XDG_DATA_HOME/harness/reviews/<key>, defaulting to ~/.local/share when XDG_DATA_HOME is unset or relative. Without an origin remote, the key is the repository's common Git directory, so every worktree of one repository shares a ledger.
func Dir(root string) (string, error) {
	source, err := git(root, "remote", "get-url", "origin")
	if err != nil {
		if source, err = git(root, "rev-parse", "--path-format=absolute", "--git-common-dir"); err != nil {
			return "", err
		}
	}
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "harness", "reviews", Key(source)), nil
}

// Append writes one line to the ledger file, creating its directory. A single O_APPEND write keeps concurrent records from interleaving.
func Append(file string, e Entry) error {
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

// Read returns every line of the ledger in append order. A missing file is an empty ledger, and blank lines are skipped.
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
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
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

// Latest returns the latest line of each finding and whole-review line on branch, in first-seen order. inHead reports whether a commit is in the head's history: a finding whose reviewed commit is not stays listed, marked rewritten, and a fixed finding stays open until its fixing commit is in the head.
func Latest(entries []Entry, branch string, inHead func(sha string) bool) []Finding {
	index := map[string]int{}
	var out []Finding
	for _, e := range entries {
		if e.Branch != branch {
			continue
		}
		if i, ok := index[e.ID]; ok {
			out[i].Entry = e
			continue
		}
		index[e.ID] = len(out)
		out = append(out, Finding{Entry: e})
	}
	for i := range out {
		f := &out[i]
		switch f.Disposition {
		case "confirmed", "unverified":
			f.Open = true
		case "fixed":
			f.Open = !inHead(f.Commit)
		}
		f.Rewritten = !inHead(f.SHA)
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
