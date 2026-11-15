package land

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/review"
)

// advance puts a new commit on origin's main that no checkout has.
func (f *fixture) advance() string {
	f.t.Helper()
	f.git(f.dir, "fetch", "-q", "origin")
	sha := f.git(f.dir, "commit-tree", "origin/main^{tree}", "-p", "origin/main", "-m", "elsewhere")
	f.git(f.dir, "push", "-q", "origin", sha+":refs/heads/main")
	return sha
}

// TestSync covers fast-forwarding the default branch where a worktree holds it, where none does, and refusing a diverged one.
func TestSync(t *testing.T) {
	f := setup(t)
	env := f.env()

	sha := f.advance()
	if a, err := Sync(env); err != nil || !a.Done || f.git(f.dir, "rev-parse", "HEAD") != sha {
		t.Fatalf("held by this worktree: %v %v", a, err)
	}

	f.git(f.dir, "switch", "-q", "-c", "other")
	sha = f.advance()
	if a, err := Sync(env); err != nil || !a.Done || f.git(f.dir, "rev-parse", "main") != sha || f.git(f.dir, "branch", "--show-current") != "other" {
		t.Fatalf("held by no worktree: %v %v", a, err)
	}

	f.git(f.dir, "switch", "-q", "main")
	local := f.commit(map[string]string{"local.txt": "mine\n"})
	f.advance()
	if _, err := Sync(env); !errors.Is(err, ErrDiverged) || f.git(f.dir, "rev-parse", "main") != local {
		t.Fatalf("diverged: %v, main at %s", err, f.git(f.dir, "rev-parse", "main"))
	}
}

// landed opens a pull request on branch from main, squash-merges it on origin, and returns its review directory holding a saved patch, as a review run leaves it. The checkout is left on main.
func (f *fixture) landed(number int, branch string) (*pull, string) {
	f.t.Helper()
	f.git(f.dir, "switch", "-q", "main")
	f.git(f.dir, "pull", "-q", "--ff-only", "origin", "main")
	p := f.open(number, branch, "main", map[string]string{branch + ".txt": branch + "\n"})
	f.git(f.dir, "switch", "-q", "main")
	f.git(f.dir, "merge", "-q", "--squash", branch)
	f.git(f.dir, "commit", "-q", "-m", "squash "+branch)
	f.git(f.dir, "push", "-q", "origin", "main")
	p.State = "MERGED"
	return p, f.scratch(branch)
}

// scratch writes a saved patch into key's review directory and returns the directory.
func (f *fixture) scratch(key string) string {
	f.t.Helper()
	dir, err := review.Dir(f.dir, key)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review.patch"), []byte("x\n"), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return dir
}

// worktreeOn moves branch from the main checkout into a linked worktree and returns its path.
func (f *fixture) worktreeOn(branch string) string {
	f.t.Helper()
	path := filepath.Join(realTemp(f.t), branch)
	f.git(f.dir, "worktree", "add", "-q", path, branch)
	return path
}

// realTemp is a temporary directory by its real path, as git worktree list prints it.
func realTemp(t *testing.T) string {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (f *fixture) has(ref string) bool {
	_, err := git(f.dir, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}

func (f *fixture) finish(dir string, numbers ...int) ([]Action, error) {
	f.t.Helper()
	return Finish(Env{Dir: dir, Forge: f.forge, State: f.state}, numbers, FinishOptions{})
}

func said(actions []Action, done bool, want string) bool {
	for _, a := range actions {
		if a.Done == done && strings.Contains(a.Text, want) {
			return true
		}
	}
	return false
}

// TestFinish covers the full cleanup of the forge user's merged pull request from its own checkout, the refusal of an open one, and a second run changing nothing.
func TestFinish(t *testing.T) {
	f := setup(t)
	f.open(2, "open", "main", map[string]string{"o.txt": "o\n"})
	_, scratch := f.landed(1, "feat")
	wt := f.worktreeOn("feat")

	if _, err := f.finish(wt, 1, 2); err == nil || !exists(scratch) || !exists(wt) {
		t.Fatalf("an open pull request was not refused before any removal: %v", err)
	}

	actions, err := f.finish(wt, 1)
	if err != nil {
		t.Fatalf("finish: %v %v", err, actions)
	}
	switch {
	case exists(scratch):
		t.Error("the review directory stayed")
	case f.has("refs/remotes/origin/feat") || strings.Contains(f.git(f.dir, "ls-remote", "--heads", "origin"), "refs/heads/feat"):
		t.Error("the remote branch stayed")
	case exists(wt):
		t.Error("the worktree stayed")
	case f.has("refs/heads/feat"):
		t.Error("the local branch stayed")
	}
	if a := actions[len(actions)-1]; !strings.Contains(a.Text, "deleted branch feat") {
		t.Errorf("retiring the checkout was not last: %v", actions)
	}

	again, err := f.finish(f.dir, 1)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for _, a := range again {
		if a.Done {
			t.Errorf("a second run did %s", a)
		}
	}
}

// TestFinishKeeps covers what stays: a remote branch with a dependent, someone else's branch, a fork's branches, and a review directory that records a worktree.
func TestFinishKeeps(t *testing.T) {
	f := setup(t)

	f.landed(1, "base")
	f.open(2, "child", "base", map[string]string{"c.txt": "c\n"})
	f.git(f.dir, "switch", "-q", "main")
	actions, err := f.finish(f.dir, 1)
	if err != nil || !said(actions, false, "#2 still based on it") || !f.has("refs/remotes/origin/base") {
		t.Errorf("a dependent's base: %v %v", err, actions)
	}

	p, _ := f.landed(3, "theirs")
	p.Author.Login = "someone"
	if actions, err := f.finish(f.dir, 3); err != nil || !said(actions, false, "it is someone's") || !f.has("refs/remotes/origin/theirs") {
		t.Errorf("someone else's branch: %v %v", err, actions)
	}

	p, _ = f.landed(4, "forked")
	p.IsCrossRepository = true
	forkScratch := f.scratch("pr-4")
	if actions, err := f.finish(f.dir, 4); err != nil || exists(forkScratch) || !f.has("refs/remotes/origin/forked") || !f.has("refs/heads/forked") {
		t.Errorf("a fork: %v %v", err, actions)
	}

	_, held := f.landed(5, "held")
	if err := os.WriteFile(filepath.Join(held, "worktree.txt"), []byte(f.t.TempDir()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions, err = f.finish(f.dir, 5)
	if err != nil || !exists(held) || !said(actions, false, "review directory") || f.has("refs/remotes/origin/held") || f.has("refs/heads/held") {
		t.Errorf("a held review directory: %v %v", err, actions)
	}

	// A remote branch that gained commits after its pull request merged stays.
	f.landed(6, "moved")
	moved := f.git(f.dir, "commit-tree", "moved^{tree}", "-p", "moved", "-m", "more")
	f.git(f.dir, "push", "-q", "origin", moved+":refs/heads/moved")
	if actions, err := f.finish(f.dir, 6); err != nil || !f.has("refs/remotes/origin/moved") || !said(actions, false, "remote branch moved") {
		t.Errorf("a moved remote branch: %v %v", err, actions)
	}

	// A remote branch that heads another open pull request stays.
	p, _ = f.landed(7, "again")
	reopened := *p
	reopened.Number, reopened.State = 8, "OPEN"
	f.pulls[8] = &reopened
	if actions, err := f.finish(f.dir, 7); err != nil || !f.has("refs/remotes/origin/again") {
		t.Errorf("a branch heading an open pull request: %v %v", err, actions)
	}
}

// TestFinishOrder covers a refused retirement of this checkout, and that with two pull requests this checkout goes last.
func TestFinishOrder(t *testing.T) {
	f := setup(t)
	f.landed(1, "one")
	f.landed(2, "two")
	wt := f.worktreeOn("one")
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	actions, err := f.finish(wt, 1, 2)
	if err == nil || !exists(wt) || !said(actions, true, "#2 deleted branch two") || !said(actions, true, "#1 deleted remote branch one") {
		t.Fatalf("a dirty checkout: %v %v", err, actions)
	}
	if a := actions[len(actions)-1]; a.Done || !strings.Contains(a.Text, "this checkout") {
		t.Errorf("the refusal was not last: %v", actions)
	}

	os.Remove(filepath.Join(wt, "dirty.txt"))
	actions, err = f.finish(wt, 2, 1)
	if err != nil || exists(wt) || !strings.Contains(actions[len(actions)-1].Text, "#1") {
		t.Errorf("this checkout was not retired last: %v %v", err, actions)
	}
}
