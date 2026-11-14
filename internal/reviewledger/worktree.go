package reviewledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	wt "github.com/jvillegasd/harness/internal/worktree"
)

// recordName is the file in a key's review directory that records the one worktree a review may create. Its first line is the worktree's path and its second the commit it was created at. While it exists, `remove` and `prune` leave the directory alone.
const recordName = "worktree.txt"

func readRecord(dir string) (path, sha string, ok bool, err error) {
	raw, err := os.ReadFile(filepath.Join(dir, recordName))
	if errors.Is(err, os.ErrNotExist) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	path = strings.TrimSpace(lines[0])
	if len(lines) > 1 {
		sha = strings.TrimSpace(lines[1])
	}
	return path, sha, true, nil
}

// findTree returns the worktree of the repository at path, if any.
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

// AddWorktree creates the review worktree for key: a detached checkout of sha in a fresh sibling directory outside the checkout, with Git hooks disabled, and the saved patch applied when one is given. The record is written before the worktree exists, so a failed creation still names where it was meant to be. Run again while that worktree exists at the same commit, it returns the same path and changes nothing.
func AddWorktree(root, key, sha, patch string) (string, error) {
	file, err := File(root, key)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(file)
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
		// A patch cannot be told apart from what the worktree already holds, so it goes only into a fresh one.
		case tree != nil && patch != "":
			return "", fmt.Errorf("%w: the review worktree %s already exists, so the patch was not applied; retire it first", ErrHeld, path)
		case tree != nil && tree.Head == full:
			return path, nil
		case tree != nil:
			return "", fmt.Errorf("%w: the review worktree %s is at %s, not %s; retire it first", ErrHeld, path, tree.Head, full)
		case recorded != full:
			return "", fmt.Errorf("%w: %s records %s at %s, which is not %s; retire it first", ErrHeld, filepath.Join(dir, recordName), path, recorded, full)
		}
		// The recorded worktree was never created, so creation resumes at the recorded path.
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

// RetireWorktree retires the review worktree recorded for key without losing work: whatever it holds is saved as a patch in the review directory first, and the record is deleted only once the worktree is gone. A refusal leaves both. It returns one line per thing it did; with no record it does nothing.
func RetireWorktree(root, key string, allowIgnored bool) ([]string, error) {
	file, err := File(root, key)
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(file)
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
