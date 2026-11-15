package changemap

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

var (
	ErrNotRepo     = errors.New("not inside a git repository")
	ErrUnknownBase = errors.New("unknown base ref")
)

type Changes struct {
	Base      string
	MergeBase string
	Files     []string
}

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

func Root(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", ErrNotRepo
	}
	return strings.TrimSpace(out), nil
}

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
