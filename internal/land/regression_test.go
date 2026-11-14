package land

import (
	"strings"
	"testing"
	"time"
)

// TestWatchWaitsWhileComputing covers a forge still computing mergeability right after a push: the watch waits instead of stopping.
func TestWatchWaitsWhileComputing(t *testing.T) {
	f := setup(t)
	f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	_, code, err, c := f.watch(time.Hour, func(pass int) {
		if pass == 1 {
			f.pulls[1].Mergeable, f.pulls[1].MergeStateStatus = "UNKNOWN", "UNKNOWN"
		} else {
			f.pulls[1].Mergeable, f.pulls[1].MergeStateStatus = "MERGEABLE", "CLEAN"
		}
	})
	if err != nil || code != ExitReady || c.sleeps != 1 {
		t.Errorf("code %d after %d sleeps: %v", code, c.sleeps, err)
	}
}

// TestFinishKeepsMovedRemote covers a remote branch that gained commits after its pull request merged, or heads another open pull request: it stays.
func TestFinishKeepsMovedRemote(t *testing.T) {
	f := setup(t)
	f.landed(1, "feat")
	moved := f.git(f.dir, "commit-tree", "feat^{tree}", "-p", "feat", "-m", "more")
	f.git(f.dir, "push", "-q", "origin", moved+":refs/heads/feat")
	actions, err := f.finish(f.dir, 1)
	if err != nil || !f.has("refs/remotes/origin/feat") || !said(actions, false, "remote branch feat") {
		t.Errorf("a moved remote branch: %v %v", err, actions)
	}

	p, _ := f.landed(2, "again")
	reopened := *p
	reopened.Number, reopened.State = 3, "OPEN"
	f.pulls[3] = &reopened
	actions, err = f.finish(f.dir, 2)
	if err != nil || !f.has("refs/remotes/origin/again") {
		t.Errorf("a branch heading an open pull request: %v %v", err, actions)
	}
}

// TestDirtyHoldText covers land check's hold for a DIRTY merge state on a mergeable pull request, unchanged from before.
func TestDirtyHoldText(t *testing.T) {
	f := setup(t)
	p := f.open(1, "docs", "main", map[string]string{"README.md": "more\n"})
	p.MergeStateStatus = "DIRTY"
	holds(t, f.one(1), "its merge state is DIRTY")
}

// TestRepairConflictRestoresBase covers a conflicted repair putting the dependent's base back, so finish keeps the landed branch it still sits on.
func TestRepairConflictRestoresBase(t *testing.T) {
	f := setup(t)
	feat, child := f.stack()
	f.squash(feat)
	f.commit(map[string]string{"child.txt": "main's own\n"})
	f.git(f.dir, "push", "-q", "origin", "main")
	if _, err := f.repair(2, 1); err == nil || !strings.Contains(err.Error(), "conflicted") {
		t.Fatalf("err %v", err)
	}
	if child.BaseRefName != "feat" {
		t.Errorf("the dependent's base is %s after a conflicted repair", child.BaseRefName)
	}
}
