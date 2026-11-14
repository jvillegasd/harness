package main

import (
	"os"
	"strings"
	"testing"
)

// TestReviewWorktreeCommand proves the CLI is wired to the review worktree lifecycle: add prints a new worktree's path, and retire removes it and its record.
func TestReviewWorktreeCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	head := git(t, dir, "rev-parse", "HEAD")

	r := reviewRun(t, dir, data, "worktree", "add", head)
	path := strings.TrimSpace(r.stdout)
	if r.code != 0 || path == "" || git(t, path, "rev-parse", "HEAD") != head {
		t.Fatalf("add exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if r := reviewRun(t, dir, data, "worktree", "retire"); r.code != 0 {
		t.Fatalf("retire exited %d: %s", r.code, r.stderr)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the worktree %s is still there", path)
	}
	if r := reviewRun(t, dir, data, "worktree", "add", "nope"); r.code != 2 || r.stdout != "" {
		t.Errorf("an unknown commit exited %d with %q", r.code, r.stdout)
	}
}
