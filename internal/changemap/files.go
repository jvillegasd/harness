package changemap

import (
	"errors"
	"fmt"
	"github.com/jvillegasd/harness/internal/run"
	"slices"
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

func Root(dir string) (string, error) {
	out, err := run.Git(dir, "rev-parse", "--show-toplevel")
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
	_, err := run.Git(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

func Changed(root, base string) (Changes, error) {
	if !resolves(root, base) {
		return Changes{}, fmt.Errorf("%w %q", ErrUnknownBase, base)
	}
	mergeBase, err := run.Git(root, "merge-base", "HEAD", base)
	if err != nil {
		return Changes{}, err
	}
	mergeBase = strings.TrimSpace(mergeBase)
	tracked, err := run.GitRaw(root, "diff", "--name-only", "--no-renames", "-z", mergeBase)
	if err != nil {
		return Changes{}, err
	}
	untracked, err := run.GitRaw(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return Changes{}, err
	}
	var files []string
	for _, listing := range [][]byte{tracked, untracked} {
		for _, name := range strings.Split(string(listing), "\x00") {
			if name != "" {
				files = append(files, name)
			}
		}
	}
	slices.Sort(files)
	files = slices.Compact(files)
	return Changes{Base: base, MergeBase: mergeBase, Files: files}, nil
}
