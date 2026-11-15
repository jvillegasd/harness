// Package review holds what a review keeps on this machine and what it posts to the forge: the review level, the target a review reads, the one worktree it may create, snapshots of uncommitted work, the verdict comment it posts on a pull request, and the review evals. Findings and verdicts live on the pull request, never in a local record.
package review

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// ErrInvalid marks a rejected argument; tests use it to tell a rejection from an I/O failure.
	ErrInvalid = errors.New("invalid argument")
	// ErrHeld marks a review directory that still records a review worktree, so removing it is refused.
	ErrHeld = errors.New("a review worktree is still recorded")
)

// Dir is the review directory for key in the repository at root: <git-common-dir>/harness-review/branches/<key>. It holds only scratch a review run makes, the record of its worktree and saved patches, never findings. Git never tracks the common directory, every worktree of the repository shares it, and the key is path-escaped so a "/" stays inside one directory name.
func Dir(root, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: no branch is checked out; pass --branch", ErrInvalid)
	}
	// Escaping leaves a dot segment as it is, and it would resolve to a parent of the branch directories.
	if key == "." || key == ".." {
		return "", fmt.Errorf("%w: %q is not a branch name", ErrInvalid, key)
	}
	dir, err := BranchesDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, url.PathEscape(key)), nil
}

// BranchesDir is the directory holding every branch's review directory in the repository at root.
func BranchesDir(root string) (string, error) {
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "harness-review", "branches"), nil
}

// Prune removes each branch directory under dir whose branch live does not report, as after a pull request merged without `land` or was closed. It returns the directories removed, or that would be with dryRun, and those held because worktree.txt records a review worktree that still exists. A name that does not unescape is left alone.
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
		if live, err := recordLive(path); err != nil {
			return removed, held, err
		} else if live {
			held = append(held, path)
			continue
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

// RemoveBranch deletes a key's review directory, with its patches, and reports whether there was one. It refuses while worktree.txt records a worktree that still exists: that file is the only record of an executor worktree review could not retire, and of where its work was saved.
func RemoveBranch(dir string) (bool, error) {
	// Only a directory directly under branches/ is one branch's; anything else holds more than this removal owns.
	if filepath.Base(filepath.Dir(dir)) != "branches" {
		return false, fmt.Errorf("%w: %s is not a branch's review directory", ErrInvalid, dir)
	}
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if live, err := recordLive(dir); err != nil {
		return false, err
	} else if live {
		return false, fmt.Errorf("%w in %s; retire it with harness review worktree retire, then remove again", ErrHeld, filepath.Join(dir, "worktree.txt"))
	}
	return true, os.RemoveAll(dir)
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
