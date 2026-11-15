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
	ErrInvalid = errors.New("invalid argument")

	ErrHeld = errors.New("a review worktree is still recorded")
)

func Dir(root, key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%w: no branch is checked out; pass --branch", ErrInvalid)
	}

	if key == "." || key == ".." {
		return "", fmt.Errorf("%w: %q is not a branch name", ErrInvalid, key)
	}
	dir, err := BranchesDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, url.PathEscape(key)), nil
}

func BranchesDir(root string) (string, error) {
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "harness-review", "branches"), nil
}

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

func RemoveBranch(dir string) (bool, error) {

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
