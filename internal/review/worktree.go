package review

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wt "github.com/jvillegasd/harness/internal/worktree"
)

const recordName = "worktree.txt"

func readRecord(dir string) (path, sha string, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, recordName))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	first, rest, _ := strings.Cut(strings.TrimSpace(string(raw)), "\n")
	second, _, _ := strings.Cut(rest, "\n")
	path, sha = strings.TrimSpace(first), strings.TrimSpace(second)
	return path, sha, true, nil
}

func recordLive(dir string) (bool, error) {
	path, _, ok, err := readRecord(dir)
	if err != nil || !ok {
		return false, err
	}
	return path != "" && exists(path), nil
}

func findTree(root, path string) (*wt.Tree, error) {
	trees, err := wt.List(root)
	if err != nil {
		return nil, err
	}
	real := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	for i := range trees {
		if real(trees[i].Path) == real(path) {
			return &trees[i], nil
		}
	}
	return nil, nil
}

func AddWorktree(root, key, sha, patch string) (string, error) {
	dir, err := Dir(root, key)
	if err != nil {
		return "", err
	}
	full, err := commit(root, sha)
	if err != nil {
		return "", err
	}
	path, recorded, ok, err := readRecord(dir)
	if err != nil {
		return "", err
	}
	if ok {
		tree, err := findTree(root, path)
		if err != nil {
			return "", err
		}
		switch {

		case tree != nil && patch != "":
			return "", fmt.Errorf("%w: the review worktree %s already exists, so the patch was not applied; retire it first", ErrHeld, path)
		case tree != nil && tree.Head == full:
			return path, nil
		case tree != nil:
			return "", fmt.Errorf("%w: the review worktree %s is at %s, not %s; retire it first", ErrHeld, path, tree.Head, full)
		case recorded != full:
			return "", fmt.Errorf("%w: %s records %s at %s, which is not %s; retire it first", ErrHeld, filepath.Join(dir, recordName), path, recorded, full)
		}

	} else {
		parent, base := filepath.Dir(root), filepath.Base(root)+"-review-"+full[:12]
		path = filepath.Join(parent, base)
		for n := 2; exists(path); n++ {
			path = filepath.Join(parent, base+"-"+strconv.Itoa(n))
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, recordName), []byte(path+"\n"+full+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	if _, err := git(root, "-c", "core.hooksPath=/dev/null", "worktree", "add", "--quiet", "--detach", path, full); err != nil {
		return "", err
	}
	if patch != "" {
		abs, err := filepath.Abs(patch)
		if err != nil {
			return "", err
		}
		if _, err := git(path, "apply", "--binary", abs); err != nil {
			return "", fmt.Errorf("the worktree %s was created, but the patch did not apply: %w", path, err)
		}
	}
	return path, nil
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func RetireWorktree(root, key string, allowIgnored bool) ([]string, error) {
	dir, err := Dir(root, key)
	if err != nil {
		return nil, err
	}
	path, sha, ok, err := readRecord(dir)
	if err != nil || !ok {
		return nil, err
	}
	record := filepath.Join(dir, recordName)
	tree, err := findTree(root, path)
	if err != nil {
		return nil, err
	}
	var done []string
	switch {
	case tree == nil && exists(path):
		return nil, fmt.Errorf("%w: %s is recorded as the review worktree but is not a worktree of this repository; check it and delete %s by hand", wt.ErrRefused, path, record)
	case tree == nil:
		done = append(done, "no worktree at "+path+": it was never created or is already gone")
	case tree.Prunable:
		if _, err := git(root, "worktree", "prune"); err != nil {
			return nil, err
		}
		done = append(done, "pruned "+path+", whose directory was already gone")
	default:
		save := filepath.Join(dir, filepath.Base(path)+".patch")
		retired, err := wt.Retire(root, path, wt.Options{At: sha, SavePatch: save, AllowIgnored: allowIgnored, KeepBranch: true})
		done = append(done, retired...)
		if err != nil {
			return done, err
		}
	}
	if err := os.Remove(record); err != nil {
		return done, err
	}
	return append(done, "deleted "+record), nil
}
