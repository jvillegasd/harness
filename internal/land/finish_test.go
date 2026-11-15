package land

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/review"
)

func (f *fixture) advance() string {
	f.t.Helper()
	f.git(f.dir, "fetch", "-q", "origin")
	sha := f.git(f.dir, "commit-tree", "origin/main^{tree}", "-p", "origin/main", "-m", "elsewhere")
	f.git(f.dir, "push", "-q", "origin", sha+":refs/heads/main")
	return sha
}

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

func (f *fixture) worktreeOn(branch string) string {
	f.t.Helper()
	path := filepath.Join(realTemp(f.t), branch)
	f.git(f.dir, "worktree", "add", "-q", path, branch)
	return path
}

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

	f.landed(6, "moved")
	moved := f.git(f.dir, "commit-tree", "moved^{tree}", "-p", "moved", "-m", "more")
	f.git(f.dir, "push", "-q", "origin", moved+":refs/heads/moved")
	if actions, err := f.finish(f.dir, 6); err != nil || !f.has("refs/remotes/origin/moved") || !said(actions, false, "remote branch moved") {
		t.Errorf("a moved remote branch: %v %v", err, actions)
	}

	p, _ = f.landed(7, "again")
	reopened := *p
	reopened.Number, reopened.State = 8, "OPEN"
	f.pulls[8] = &reopened
	if actions, err := f.finish(f.dir, 7); err != nil || !f.has("refs/remotes/origin/again") {
		t.Errorf("a branch heading an open pull request: %v %v", err, actions)
	}
}

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
