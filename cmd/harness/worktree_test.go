package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorktreeCommand proves the CLI is wired to the retirement rules: the audit names each state, a refusal exits 1 and removes nothing, and a retirement removes the worktree and its landed branch.
func TestWorktreeCommand(t *testing.T) {
	dir := repo(t)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	parent := filepath.Dir(dir)
	landed, open := filepath.Join(parent, "landed"), filepath.Join(parent, "open")
	git(t, dir, "worktree", "add", "-q", "-b", "landed", landed)
	git(t, dir, "worktree", "add", "-q", "-b", "open", open)
	// The landed proof is by content, so the open branch needs a real change.
	if err := os.WriteFile(filepath.Join(open, "work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, open, "add", ".")
	git(t, open, "commit", "-q", "-m", "work")
	if err := os.WriteFile(filepath.Join(landed, "scratch.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	audit := ok(t, harness(t, dir, "worktree", "audit", "--into", branch), "audit").stdout
	for path, state := range map[string]string{landed: "dirty", open: "unlanded"} {
		found := false
		for _, line := range strings.Split(audit, "\n") {
			found = found || strings.HasPrefix(line, state) && strings.HasSuffix(line, path)
		}
		if !found {
			t.Errorf("audit lacks a %s row for %s:\n%s", state, path, audit)
		}
	}
	if strings.Contains(audit, dir+"\n") {
		t.Errorf("audit listed the main worktree:\n%s", audit)
	}

	for _, refused := range [][]string{
		{"worktree", "retire", landed, "--into", branch},
		{"worktree", "retire", open, "--into", branch},
		{"worktree", "retire", "--branch", "open", "--into", branch},
		{"worktree", "retire", dir, "--into", branch},
	} {
		if r := harness(t, dir, refused...); r.code != 1 || r.stderr == "" || r.stdout != "" {
			t.Errorf("%v exited %d with stdout %q, stderr %q", refused, r.code, r.stdout, r.stderr)
		}
	}
	for _, kept := range []string{landed, open} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("a refused retirement removed %s", kept)
		}
	}

	if err := os.Remove(filepath.Join(landed, "scratch.txt")); err != nil {
		t.Fatal(err)
	}
	// Run from inside the worktree being retired, as land does.
	r := ok(t, harness(t, landed, "worktree", "retire", landed, "--into", branch), "retire")
	if _, err := os.Stat(landed); err == nil || git(t, dir, "branch", "--list", "landed") != "" {
		t.Errorf("worktree or branch survived: %s", r.stdout)
	}
	if !strings.Contains(r.stdout, "removed worktree") || !strings.Contains(r.stdout, "deleted branch landed") {
		t.Errorf("retire printed %q", r.stdout)
	}
}
