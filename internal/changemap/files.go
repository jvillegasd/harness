package changemap

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Sentinel errors let the command map a bad invocation to exit 2 and keep a failed git run as an operational failure.
var (
	ErrNotRepo     = errors.New("not inside a git repository")
	ErrUnknownBase = errors.New("unknown base ref")
)

// Changes is the set of files a branch changes against its base.
type Changes struct {
	Base      string
	MergeBase string
	Files     []string
}

// git runs one git command in dir and returns its stdout. A repository path is passed as the working directory, never parsed out of output, so names with spaces stay intact.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Root returns the top level of the repository containing dir. All other git calls run from it, since git reports some paths relative to the working directory and others to the root.
func Root(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepo
	}
	return strings.TrimSpace(out), nil
}

// DefaultBase is origin/HEAD when it resolves and main otherwise.
func DefaultBase(root string) string {
	if resolves(root, "origin/HEAD") {
		return "origin/HEAD"
	}
	return "main"
}

func resolves(root, ref string) bool {
	if strings.HasPrefix(ref, "-") {
		return false
	}
	_, err := git(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// Changed lists every file that differs between the merge base of HEAD and base and the working tree, so committed, staged, and unstaged work all count, plus untracked files git does not ignore. Deletions are included because removing a critical file is still a critical change. Renames are not followed: the old and new paths each appear, so a rename out of a critical path is not hidden.
func Changed(root, base string) (Changes, error) {
	if !resolves(root, base) {
		return Changes{}, fmt.Errorf("%w %q", ErrUnknownBase, base)
	}
	mergeBase, err := git(root, "merge-base", "HEAD", base)
	if err != nil {
		return Changes{}, err
	}
	mergeBase = strings.TrimSpace(mergeBase)
	tracked, err := git(root, "diff", "--name-only", "--no-renames", "-z", mergeBase)
	if err != nil {
		return Changes{}, err
	}
	untracked, err := git(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	seen := map[string]bool{}
	var files []string
	for _, listing := range []string{tracked, untracked} {
		for _, name := range strings.Split(listing, "\x00") {
			if name != "" && !seen[name] {
				seen[name] = true
				files = append(files, name)
			}
		}
	}
	sort.Strings(files)
	return Changes{Base: base, MergeBase: mergeBase, Files: files}, nil
}
