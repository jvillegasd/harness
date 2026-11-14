package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repo creates a repository on main with one commit that ignores *.log, isolated from the user's git config.
func repo(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "gitconfig"))
	dir := filepath.Join(t.TempDir(), "main")
	run(t, filepath.Dir(dir), "init", "-q", "-b", "main", dir)
	write(t, filepath.Join(dir, ".gitignore"), "*.log\n")
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	run(t, dir, "add", ".")
	run(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// branchTree adds a sibling worktree on a new branch holding one commit that edits a.txt.
func branchTree(t *testing.T, dir, branch string) string {
	t.Helper()
	path := filepath.Join(filepath.Dir(dir), strings.ReplaceAll(branch, "/", "-"))
	run(t, dir, "worktree", "add", "-q", "-b", branch, path)
	write(t, filepath.Join(path, "a.txt"), "one\n"+branch+"\n")
	run(t, path, "commit", "-q", "-am", branch)
	return path
}

// squash lands a branch on main as one new commit, the way a squash merge does.
func squash(t *testing.T, dir, branch string) {
	t.Helper()
	run(t, dir, "merge", "-q", "--squash", branch)
	run(t, dir, "commit", "-q", "-m", "squash "+branch)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func hasBranch(t *testing.T, dir, branch string) bool {
	t.Helper()
	return run(t, dir, "branch", "--list", branch) != ""
}

// refused asserts a retirement was refused and left the worktree in place.
func refused(t *testing.T, path string, err error, want string) {
	t.Helper()
	if !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want a refusal naming %q", err, want)
	}
	if !exists(path) {
		t.Fatalf("a refused retirement removed %s", path)
	}
}

// TestRetireRemovesALandedWorktreeFromInsideIt covers the path land takes: a squash-merged branch, retired from the worktree being removed.
func TestRetireRemovesALandedWorktreeFromInsideIt(t *testing.T) {
	dir := repo(t)
	path := branchTree(t, dir, "feat/x")
	squash(t, dir, "feat/x")

	done, err := Retire(path, path, Options{Into: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if exists(path) || hasBranch(t, dir, "feat/x") {
		t.Fatalf("worktree or branch survived: %v", done)
	}
	if got := strings.Join(done, "; "); got != "removed worktree "+path+"; deleted branch feat/x" {
		t.Errorf("reported %q", got)
	}
}

// TestRetireRefusesToLoseBranchWork covers each thing a branch worktree can hold that removal would lose, and the option that covers it.
func TestRetireRefusesToLoseBranchWork(t *testing.T) {
	dir := repo(t)
	path := branchTree(t, dir, "task")

	_, err := Retire(dir, path, Options{Into: "main"})
	refused(t, path, err, "not proven landed on main")
	if s, _ := Inspect(tree(t, dir, path), "main"); s.State != "unlanded" {
		t.Errorf("state %q, want unlanded", s.State)
	}
	_, err = Retire(dir, path, Options{})
	refused(t, path, err, "pass --into")

	squash(t, dir, "task")
	write(t, filepath.Join(path, "build.log"), "x")
	_, err = Retire(dir, path, Options{Into: "main"})
	refused(t, path, err, "build.log")
	if s, _ := Inspect(tree(t, dir, path), "main"); s.State != "ignored" {
		t.Errorf("state %q, want ignored", s.State)
	}

	write(t, filepath.Join(path, "new.txt"), "wip\n")
	_, err = Retire(dir, path, Options{Into: "main", AllowIgnored: true})
	refused(t, path, err, "new.txt")
	if s, _ := Inspect(tree(t, dir, path), "main"); s.State != "dirty" {
		t.Errorf("state %q, want dirty", s.State)
	}

	run(t, dir, "worktree", "lock", path)
	_, err = Retire(dir, path, Options{Into: "main", AllowIgnored: true, SavePatch: filepath.Join(t.TempDir(), "p")})
	refused(t, path, err, "locked")
	run(t, dir, "worktree", "unlock", path)

	patch := filepath.Join(t.TempDir(), "work.patch")
	if _, err := Retire(dir, path, Options{Into: "main", AllowIgnored: true, SavePatch: patch}); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(patch); !strings.Contains(string(raw), "new.txt") {
		t.Errorf("patch lacks the uncommitted file: %q", raw)
	}
	if exists(path) || hasBranch(t, dir, "task") {
		t.Error("worktree or landed branch survived")
	}

	_, err = Retire(dir, dir, Options{Into: "main"})
	refused(t, dir, err, "main worktree")
	if _, err := Retire(dir, t.TempDir(), Options{Into: "main"}); !errors.Is(err, ErrRefused) {
		t.Errorf("a directory outside the repository returned %v", err)
	}
}

// TestRetireKeepsAnUnlandedBranch removes only the worktree when asked to.
func TestRetireKeepsAnUnlandedBranch(t *testing.T) {
	dir := repo(t)
	path := branchTree(t, dir, "task")
	if _, err := Retire(dir, path, Options{KeepBranch: true}); err != nil {
		t.Fatal(err)
	}
	if exists(path) || !hasBranch(t, dir, "task") {
		t.Error("want the worktree gone and the branch kept")
	}
}

// TestRetireDetachedWorktree covers a head on a ref, a loose head still where the caller created it, and one that moved.
func TestRetireDetachedWorktree(t *testing.T) {
	dir := repo(t)
	parent := filepath.Dir(dir)
	add := func(name string) string {
		path := filepath.Join(parent, name)
		run(t, dir, "worktree", "add", "-q", "--detach", path, "main")
		return path
	}

	onRef := add("on-ref")
	if _, err := Retire(dir, onRef, Options{}); err != nil || exists(onRef) {
		t.Fatalf("a detached head on a branch was not retired: %v", err)
	}

	loose := add("loose")
	base := run(t, loose, "rev-parse", "HEAD")
	write(t, filepath.Join(loose, "b.txt"), "committed\n")
	run(t, loose, "add", ".")
	run(t, loose, "commit", "-q", "-m", "loose")
	head := run(t, loose, "rev-parse", "HEAD")
	if s, _ := Inspect(tree(t, dir, loose), "main"); s.State != "loose" {
		t.Errorf("state %q, want loose", s.State)
	}
	_, err := Retire(dir, loose, Options{})
	refused(t, loose, err, "pass --at")
	_, err = Retire(dir, loose, Options{At: base})
	refused(t, loose, err, "pass --save-patch")
	patch := filepath.Join(t.TempDir(), "loose.patch")
	if _, err := Retire(dir, loose, Options{At: base, SavePatch: patch}); err != nil || exists(loose) {
		t.Fatalf("a moved head was not retired with its patch: %v", err)
	}
	if raw, _ := os.ReadFile(patch); !strings.Contains(string(raw), "b.txt") {
		t.Errorf("patch lacks the loose commit: %q", raw)
	}

	// The same loose commit, named as where the worktree was created, needs no patch.
	created := filepath.Join(parent, "created")
	run(t, dir, "worktree", "add", "-q", "--detach", created, head)
	if _, err := Retire(dir, created, Options{At: head}); err != nil || exists(created) {
		t.Fatalf("a loose head at its creation commit was not retired: %v", err)
	}
}

// TestLanded separates an ancestor, a squash, an unmerged branch, and a squash that into later edited, which stays unproven.
func TestLanded(t *testing.T) {
	dir := repo(t)
	branchTree(t, dir, "merged")
	if Landed(dir, "refs/heads/merged", "main") {
		t.Error("an unmerged branch read as landed")
	}
	squash(t, dir, "merged")
	if !Landed(dir, "refs/heads/merged", "main") {
		t.Error("a squash-merged branch read as unlanded")
	}
	run(t, dir, "branch", "-q", "behind", "main~1")
	if !Landed(dir, "refs/heads/behind", "main") {
		t.Error("an ancestor read as unlanded")
	}
	write(t, filepath.Join(dir, "a.txt"), "one\nrewritten\n")
	run(t, dir, "commit", "-q", "-am", "edit the same line")
	if Landed(dir, "refs/heads/merged", "main") {
		t.Error("a branch whose lines main rewrote read as landed")
	}
}

// TestRetireBranch deletes only a landed branch no worktree holds, and a missing one is a no-op.
func TestRetireBranch(t *testing.T) {
	dir := repo(t)
	path := branchTree(t, dir, "task")
	if _, err := RetireBranch(dir, "task", "main"); !errors.Is(err, ErrRefused) || !strings.Contains(err.Error(), path) {
		t.Fatalf("a held branch returned %v", err)
	}
	if _, err := Retire(dir, path, Options{KeepBranch: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := RetireBranch(dir, "task", "main"); !errors.Is(err, ErrRefused) || !hasBranch(t, dir, "task") {
		t.Fatalf("an unlanded branch returned %v", err)
	}
	squash(t, dir, "task")
	if done, err := RetireBranch(dir, "task", "main"); err != nil || len(done) != 1 || hasBranch(t, dir, "task") {
		t.Fatalf("a landed branch was not deleted: %v %v", done, err)
	}
	if done, err := RetireBranch(dir, "task", "main"); err != nil || done != nil {
		t.Errorf("a missing branch returned %v %v", done, err)
	}
}

// TestRetireNeverDeletesTheIntoBranch keeps the branch a landed check compares against, which is trivially its own ancestor.
func TestRetireNeverDeletesTheIntoBranch(t *testing.T) {
	dir := repo(t)
	run(t, dir, "switch", "-q", "-c", "elsewhere")
	path := filepath.Join(filepath.Dir(dir), "holds-main")
	run(t, dir, "worktree", "add", "-q", path, "main")
	if _, err := Retire(dir, path, Options{Into: "main"}); err != nil || exists(path) {
		t.Fatalf("the worktree holding main was not removed: %v", err)
	}
	if _, err := RetireBranch(dir, "main", "origin/main"); !errors.Is(err, ErrRefused) || !hasBranch(t, dir, "main") {
		t.Fatalf("the into branch was deleted or returned %v", err)
	}
}

func tree(t *testing.T, dir, path string) Tree {
	t.Helper()
	trees, err := List(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range trees {
		if samePath(tr.Path, path) {
			return tr
		}
	}
	t.Fatalf("no worktree at %s", path)
	return Tree{}
}
