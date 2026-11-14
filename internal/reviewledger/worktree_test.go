package reviewledger

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (r *coverRepo) reviewDir(key string) string {
	r.t.Helper()
	file, err := File(r.dir, key)
	if err != nil {
		r.t.Fatal(err)
	}
	return filepath.Dir(file)
}

func (r *coverRepo) worktrees() int {
	return len(strings.Split(r.git("worktree", "list", "--porcelain"), "\n\n"))
}

// TestAddWorktree covers creating the review worktree outside the checkout at the commit, recording it, running no hook, applying a patch, and a second add changing nothing.
func TestAddWorktree(t *testing.T) {
	r := newCoverRepo(t)
	head := r.git("rev-parse", "HEAD")
	marker := filepath.Join(t.TempDir(), "hook ran")
	hook := filepath.Join(r.dir, ".git", "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := AddWorktree(r.dir, "feat", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(path, r.dir+string(filepath.Separator)) || filepath.Dir(path) != filepath.Dir(r.dir) {
		t.Errorf("%s is not a sibling outside the checkout %s", path, r.dir)
	}
	if got := strings.TrimSpace(r.gitIn(path, "rev-parse", "HEAD")); got != head || r.gitIn(path, "branch", "--show-current") != "" {
		t.Errorf("the worktree is at %s on a branch", got)
	}
	if raw, _ := os.ReadFile(filepath.Join(r.reviewDir("feat"), "worktree.txt")); !strings.HasPrefix(string(raw), path+"\n") {
		t.Errorf("the record holds %q", raw)
	}
	if exists(marker) {
		t.Error("a checkout hook ran")
	}

	count := r.worktrees()
	if again, err := AddWorktree(r.dir, "feat", head, ""); err != nil || again != path || r.worktrees() != count {
		t.Errorf("a second add gave %s, %v, %d worktrees", again, err, r.worktrees())
	}
	if _, err := RemoveBranch(filepath.Join(r.reviewDir("feat"), "ledger.jsonl")); !errors.Is(err, ErrHeld) {
		t.Errorf("remove did not refuse a recorded worktree: %v", err)
	}
	removed, held, err := Prune(filepath.Dir(r.reviewDir("feat")), func(string) bool { return false }, false)
	if err != nil || len(removed) != 0 || len(held) != 1 {
		t.Errorf("prune removed %v, held %v: %v", removed, held, err)
	}
}

func (r *coverRepo) gitIn(dir string, args ...string) string {
	r.t.Helper()
	saved := r.dir
	defer func() { r.dir = saved }()
	r.dir = dir
	return r.git(args...)
}

// TestAddWorktreePatch covers applying a saved patch, binary content included, in the new worktree.
func TestAddWorktreePatch(t *testing.T) {
	r := newCoverRepo(t)
	if err := os.WriteFile(filepath.Join(r.dir, "a.go"), []byte("patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(t.TempDir(), "work.patch")
	if err := os.WriteFile(patch, []byte(r.git("diff", "--binary")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.git("checkout", "--", "a.go")
	path, err := AddWorktree(r.dir, "feat", "HEAD", patch)
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(filepath.Join(path, "a.go")); string(raw) != "patched\n" {
		t.Errorf("a.go holds %q", raw)
	}
}

// TestAddWorktreeRecordFirst covers the record naming the intended path when creation fails.
func TestAddWorktreeRecordFirst(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the read-only directory this test relies on")
	}
	r := newCoverRepo(t)
	parent := filepath.Dir(r.dir)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o755)
	if _, err := AddWorktree(r.dir, "feat", "HEAD", ""); err == nil {
		t.Fatal("creation in a read-only directory succeeded")
	}
	raw, err := os.ReadFile(filepath.Join(r.reviewDir("feat"), "worktree.txt"))
	if err != nil || !strings.HasPrefix(string(raw), filepath.Join(parent, filepath.Base(r.dir)+"-review-")) {
		t.Errorf("the record holds %q: %v", raw, err)
	}
}

// TestRetireWorktree covers retiring a clean worktree, saving a dirty one's work first, and refusing over ignored paths until allowed.
func TestRetireWorktree(t *testing.T) {
	r := newCoverRepo(t)
	r.commit(map[string]string{".gitignore": "build/\n"})
	record := filepath.Join(r.reviewDir("feat"), "worktree.txt")

	path, err := AddWorktree(r.dir, "feat", "HEAD", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RetireWorktree(r.dir, "feat", false); err != nil || exists(path) || exists(record) {
		t.Fatalf("a clean worktree: %v, worktree %v, record %v", err, exists(path), exists(record))
	}

	path, _ = AddWorktree(r.dir, "feat", "HEAD", "")
	if err := os.WriteFile(filepath.Join(path, "a.go"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireWorktree(r.dir, "feat", false); err != nil || exists(path) || exists(record) {
		t.Fatalf("a dirty worktree: %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(r.reviewDir("feat"), filepath.Base(path)+".patch"))
	if err != nil || !strings.Contains(string(saved), "+work") {
		t.Errorf("the saved patch holds %q: %v", saved, err)
	}

	path, _ = AddWorktree(r.dir, "feat", "HEAD", "")
	if err := os.MkdirAll(filepath.Join(path, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "build", "out"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireWorktree(r.dir, "feat", false); err == nil || !exists(path) || !exists(record) {
		t.Fatalf("ignored paths were not refused: %v", err)
	}
	if _, err := RetireWorktree(r.dir, "feat", true); err != nil || exists(path) || exists(record) {
		t.Errorf("--allow-ignored: %v", err)
	}
	if done, err := RetireWorktree(r.dir, "feat", false); err != nil || len(done) != 0 {
		t.Errorf("with no record: %v %v", done, err)
	}
}
