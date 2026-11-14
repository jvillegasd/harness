// Package reviewledger keeps the local record of review findings and their dispositions, one file per branch inside the repository's Git directory, so it never reaches a commit or pull request and goes away with the branch.
package reviewledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	Branch      string  `json:"branch"`
	Base        string  `json:"base"`
	SHA         string  `json:"sha"`
	Digest      *string `json:"digest"`
	Depth       string  `json:"depth"`
	Path        *string `json:"path"`
	Lines       string  `json:"lines"`
	Severity    string  `json:"severity"`
	Summary     string  `json:"summary"`
	Disposition string  `json:"disposition"`
	Evidence    string  `json:"evidence"`
	// Commit is the fixing commit of a fixed finding.
	Commit string `json:"commit,omitempty"`
}

// Finding is a finding's latest line as `list` reports it, with the state computed against the head.
type Finding struct {
	Entry
	// Open holds the merge: confirmed, or fixed by a commit the head does not contain.
	Open bool `json:"open"`
	// Rewritten marks a reviewed commit that is no longer in the head's history, as after a rebase.
	Rewritten bool `json:"rewritten,omitempty"`
}

var (
	dispositions = []string{"confirmed", "fixed", "dropped", "accepted", "nit", "clean", "no-review"}
	severities   = []string{"bug", "risk", "q", "pre-existing", "nit"}
	depths       = []string{"none", "medium", "high"}
	// needEvidence are the dispositions that close a finding on someone's word, so the word must be recorded.
	needEvidence = []string{"fixed", "dropped", "accepted"}
	linesRe      = regexp.MustCompile(`^[0-9]+(-[0-9]+)?$`)
	// ErrInvalid marks a rejected entry; tests use it to tell a rejection from an I/O failure.
	ErrInvalid = errors.New("invalid entry")
	// ErrHeld marks a branch directory that still records a review worktree, so removing it is refused.
	ErrHeld = errors.New("a review worktree is still recorded")
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
	case e.Disposition == "nit" && e.Severity != "nit" && e.Severity != "pre-existing":
		return bad("only a nit or pre-existing finding can take the nit disposition")
	}
	return nil
}

// File is the ledger for branch in the repository at root: <git-common-dir>/harness-review/branches/<branch>/ledger.jsonl. The branch directory also holds the branch's review patches, so `land` removes that one directory and nothing beside it. Git never tracks the common directory, every worktree of the repository shares it, and the branch name is path-escaped so a "/" stays inside one directory name.
func File(root, branch string) (string, error) {
	if branch == "" {
		return "", fmt.Errorf("%w: no branch is checked out; pass --branch", ErrInvalid)
	}
	// Escaping leaves a dot segment as it is, and it would resolve to a parent of the branch directories.
	if branch == "." || branch == ".." {
		return "", fmt.Errorf("%w: %q is not a branch name", ErrInvalid, branch)
	}
	dir, err := BranchesDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, url.PathEscape(branch), "ledger.jsonl"), nil
}

// BranchesDir is the directory holding every branch's review directory in the repository at root.
func BranchesDir(root string) (string, error) {
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "harness-review", "branches"), nil
}

// Prune removes each branch directory under dir whose branch live does not report, as after a pull request merged without `land` or was closed. It returns the directories removed, or that would be with dryRun, and those held because worktree.txt still records a review worktree. A name that does not unescape is left alone.
func Prune(dir string, live func(branch string) bool, dryRun bool) (removed, held []string, err error) {
	names, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	for _, n := range names {
		branch, err := url.PathUnescape(n.Name())
		if !n.IsDir() || err != nil || live(branch) {
			continue
		}
		path := filepath.Join(dir, n.Name())
		if _, err := os.Stat(filepath.Join(path, "worktree.txt")); err == nil {
			held = append(held, path)
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return removed, held, err
		}
		if !dryRun {
			if err := os.RemoveAll(path); err != nil {
				return removed, held, err
			}
		}
		removed = append(removed, path)
	}
	return removed, held, nil
}

// RemoveBranch deletes the branch directory that holds the ledger file, with its patches, and reports whether there was one. It refuses while worktree.txt is there: that file is the only record of an executor worktree review could not retire, and of where its work was saved.
func RemoveBranch(file string) (bool, error) {
	dir := filepath.Dir(file)
	// Only a directory directly under branches/ is one branch's; anything else holds more than this removal owns.
	if filepath.Base(filepath.Dir(dir)) != "branches" {
		return false, fmt.Errorf("%w: %s is not a branch's review directory", ErrInvalid, dir)
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if _, err := os.Stat(filepath.Join(dir, "worktree.txt")); err == nil {
		return false, fmt.Errorf("%w in %s; retire it with harness review worktree retire, then remove again", ErrHeld, filepath.Join(dir, "worktree.txt"))
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	return true, os.RemoveAll(dir)
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
		case "confirmed":
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
