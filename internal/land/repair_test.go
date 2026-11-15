package land

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func (f *fixture) stack() (*pull, *pull) {
	f.t.Helper()
	feat := f.open(1, "feat", "main", map[string]string{"feat.txt": "feat\n"})
	child := f.open(2, "child", "feat", map[string]string{"child.txt": "child\n"})
	f.git(f.dir, "switch", "-q", "main")
	return feat, child
}

func (f *fixture) squash(p *pull) {
	f.t.Helper()
	f.git(f.dir, "switch", "-q", "main")
	f.git(f.dir, "fetch", "-q", "origin")
	f.git(f.dir, "reset", "-q", "--hard", "origin/main")
	f.git(f.dir, "merge", "-q", "--squash", p.HeadRefOid)
	f.git(f.dir, "commit", "-q", "-m", "squash")
	f.git(f.dir, "push", "-q", "origin", "main")
	p.State = "MERGED"
}

func (f *fixture) repair(dependent, landed int) (Repair, error) {
	f.t.Helper()
	return RepairDependent(f.env(), dependent, landed)
}

func (f *fixture) own(rev string) string {
	f.git(f.dir, "fetch", "-q", "origin")
	return f.git(f.dir, "log", "--format=%s", "origin/main.."+rev)
}

func (f *fixture) edits() int {
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, "pr edit") {
			n++
		}
	}
	return n
}

func TestRepair(t *testing.T) {
	f := setup(t)
	feat, child := f.stack()
	f.git(f.dir, "branch", "-q", "-D", "child")
	f.squash(feat)
	lease := child.HeadRefOid

	r, err := f.repair(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Boundary != feat.HeadRefOid || r.Lease != lease || !r.Temporary || child.BaseRefName != "main" {
		t.Fatalf("repair %+v, base %s", r, child.BaseRefName)
	}
	if got := f.own(r.Rebased); got != "change" {
		t.Errorf("rebased head holds %q above main", got)
	}
	if remote := f.git(f.dir, "ls-remote", "origin", "refs/heads/child"); !strings.HasPrefix(remote, lease) {
		t.Errorf("the first step pushed: %s", remote)
	}

	if _, err := PushRepair(f.env(), 2); err != nil {
		t.Fatal(err)
	}
	if remote := f.git(f.dir, "ls-remote", "origin", "refs/heads/child"); !strings.HasPrefix(remote, r.Rebased) {
		t.Errorf("origin's child is %s, want %s", remote, r.Rebased)
	}
	if _, err := os.Stat(r.Worktree); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the temporary worktree %s is still there", r.Worktree)
	}
}

func TestRepairRewritten(t *testing.T) {
	f := setup(t)
	feat, child := f.stack()

	f.advance()
	f.git(f.dir, "switch", "-q", "feat")
	f.git(f.dir, "rebase", "-q", "origin/main")
	f.push(feat)
	f.squash(feat)
	wt := f.worktreeOn("child")

	r, err := f.repair(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.Temporary || r.Boundary == feat.HeadRefOid {
		t.Errorf("repair %+v ran in the wrong place or from the landed head", r)
	}
	if got := f.own(r.Rebased); got != "change" {
		t.Errorf("rebased head holds %q above main", got)
	}
	if head := f.git(wt, "rev-parse", "HEAD"); head != r.Rebased || r.Worktree != wt || child.HeadRefOid == r.Rebased {
		t.Errorf("the checkout is at %s, want %s", head, r.Rebased)
	}
}

func TestRepairRefusals(t *testing.T) {
	t.Run("merge commit", func(t *testing.T) {
		f := setup(t)
		feat, child := f.stack()
		f.squash(feat)
		f.git(f.dir, "switch", "-q", "child")
		f.git(f.dir, "merge", "-q", "--no-ff", "-m", "update branch", "origin/main")
		f.push(child)
		if _, err := f.repair(2, 1); !errors.Is(err, ErrRepairRefused) || !strings.Contains(err.Error(), "merge commit") || f.edits() != 0 {
			t.Errorf("err %v, %d edits", err, f.edits())
		}
	})
	t.Run("someone else's", func(t *testing.T) {
		f := setup(t)
		feat, child := f.stack()
		f.squash(feat)
		child.Author.Login = "someone"
		if _, err := f.repair(2, 1); !errors.Is(err, ErrRepairRefused) || f.edits() != 0 {
			t.Errorf("err %v, %d edits", err, f.edits())
		}
	})
	t.Run("own commit matches a landed one", func(t *testing.T) {
		f := setup(t)
		feat := f.open(1, "feat", "main", map[string]string{"feat.txt": "feat\n"})
		child := f.open(2, "child", "feat", map[string]string{"own.txt": "own\n"})
		f.commit(map[string]string{"shared.txt": "shared\n"})
		f.push(child)

		f.git(f.dir, "switch", "-q", "feat")
		f.git(f.dir, "commit", "-q", "--amend", "-m", "feat again")
		f.commit(map[string]string{"shared.txt": "shared\n"})
		f.push(feat)
		f.squash(feat)
		before := f.git(f.dir, "rev-parse", "child")
		if _, err := f.repair(2, 1); !errors.Is(err, ErrRepairRefused) || !strings.Contains(err.Error(), "drop its own work") || f.edits() != 0 || f.git(f.dir, "rev-parse", "child") != before {
			t.Errorf("err %v, %d edits", err, f.edits())
		}
	})
	t.Run("conflict", func(t *testing.T) {
		f := setup(t)
		feat, child := f.stack()
		f.squash(feat)
		f.commit(map[string]string{"child.txt": "main's own\n"})
		f.git(f.dir, "push", "-q", "origin", "main")
		wt := f.worktreeOn("child")
		_, err := f.repair(2, 1)
		if err == nil || !strings.Contains(err.Error(), "conflicted") {
			t.Fatalf("err %v", err)
		}
		if head := f.git(wt, "rev-parse", "HEAD"); head != child.HeadRefOid || f.git(wt, "status", "--porcelain") != "" {
			t.Errorf("the checkout is at %s with %q", head, f.git(wt, "status", "--porcelain"))
		}

		if child.BaseRefName != "feat" {
			t.Errorf("the dependent's base is %s after a conflicted repair", child.BaseRefName)
		}
	})
}

func TestRepairPushLease(t *testing.T) {
	f := setup(t)
	feat, _ := f.stack()
	f.squash(feat)
	if _, err := f.repair(2, 1); err != nil {
		t.Fatal(err)
	}
	moved := f.git(f.dir, "commit-tree", "child^{tree}", "-p", "child", "-m", "someone pushed")
	f.git(f.dir, "push", "-q", "origin", moved+":refs/heads/child")

	if _, err := PushRepair(f.env(), 2); err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err %v", err)
	}
	if remote := f.git(f.dir, "ls-remote", "origin", "refs/heads/child"); !strings.HasPrefix(remote, moved) {
		t.Errorf("origin's child is %s, want %s", remote, moved)
	}
}
