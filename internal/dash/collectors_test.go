package dash

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/locks"
	"github.com/jvillegasd/harness/internal/state"
)

// recording returns a collectors set that records each read instead of doing it,
// so the model's work is asserted without touching git, wt, gh, or a store.
func recording() (collectors, *[]string) {
	var calls []string
	add := func(name string) { calls = append(calls, name) }
	return collectors{
		worktrees: func(checkout.Checkout) ([]Worktree, string) {
			add("worktrees")
			return []Worktree{{Branch: "main"}, {Branch: "next"}}, ""
		},
		states: func(_ checkout.Checkout, names []string) (map[string]state.State, error) {
			add("states")
			out := make(map[string]state.State, len(names))
			for _, name := range names {
				out[name] = state.State{Branch: name, Ladder: "vet"}
			}
			return out, nil
		},
		locks: func(checkout.Checkout) ([]locks.LockResult, error) {
			add("locks")
			return []locks.LockResult{{Name: "tdd", Status: "match"}}, nil
		},
		bench: func(checkout.Checkout) (string, error) {
			add("bench")
			return "split 2026-09-15", nil
		},
		tracker: func(checkout.Checkout) Tracker {
			add("tracker")
			return Tracker{Labels: []LabelCount{{Label: "in-progress", Count: 1}}}
		},
	}, &calls
}

func count(calls []string, name string) int {
	n := 0
	for _, call := range calls {
		if call == name {
			n++
		}
	}
	return n
}

// feed runs one command and applies its message to the model.
func feed(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil command")
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

// TestLocalRefreshReadsBranchStateOnce is the work-reduction seam: one refresh
// lists worktrees once and resolves every displayed branch from one snapshot.
func TestLocalRefreshReadsBranchStateOnce(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	m = feed(t, m, m.refreshLocal())
	if got := count(*calls, "states"); got != 1 {
		t.Fatalf("local refresh read branch state %d times, want 1: %v", got, *calls)
	}
	if got := count(*calls, "locks"); got != 0 {
		t.Fatalf("a local refresh verified locks: %v", *calls)
	}
	if len(*calls) != 2 {
		t.Fatalf("local refresh did %v, want worktrees then states", *calls)
	}
	if m.snap.Worktrees[1].State.Ladder != "vet" {
		t.Fatalf("the snapshot did not reach the displayed branch: %+v", m.snap.Worktrees[1])
	}
}

// TestTicksReloadLocallyWithoutVerifyingLocks pins that a routine tick runs the
// local reload, which verifies no locks.
func TestTicksReloadLocallyWithoutVerifyingLocks(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	m = feed(t, m, m.refreshLocks())
	before := count(*calls, "locks")
	m.localRunning = false
	next, cmd := m.Update(tickMsg(time.Now()))
	m2 := next.(Model)
	if cmd == nil || !m2.localRunning {
		t.Fatalf("a tick did not start a guarded local reload: cmd=%v running=%v", cmd, m2.localRunning)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("a tick must reload locally and re-arm, got %T", cmd())
	}
	after := feed(t, m2, batch[0]) // batch[1] is the periodic re-arm
	if got := count(*calls, "locks"); got != before {
		t.Fatalf("a tick's local reload verified locks: %v", *calls)
	}
	if got := count(*calls, "states"); got != 1 {
		t.Fatalf("the tick's local reload read branch state %d times: %v", got, *calls)
	}
	if after.snap.LocksAt.IsZero() {
		t.Fatal("the tick dropped the startup verification")
	}
}

// TestInitStartsEveryStartupLoad pins that Init verifies locks, loads the
// tracker, and reloads locally. The periodic tick is the batch's last command.
func TestInitStartsEveryStartupLoad(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	msg := m.Init()()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) != 4 {
		t.Fatalf("Init must start three loads and arm the tick, got %T with %d commands", msg, len(batch))
	}
	for _, cmd := range batch[:3] {
		m = feed(t, m, cmd)
	}
	for _, want := range []string{"worktrees", "states", "locks", "tracker"} {
		if count(*calls, want) != 1 {
			t.Fatalf("Init did not start %s exactly once: %v", want, *calls)
		}
	}
}

// TestStateReadFailureKeepsTheRowsRendered pins that a branch-state read
// failure degrades to a per-row error instead of blanking the worktrees panel.
func TestStateReadFailureKeepsTheRowsRendered(t *testing.T) {
	cols, _ := recording()
	cols.states = func(checkout.Checkout, []string) (map[string]state.State, error) {
		return nil, errors.New("git config failed")
	}
	m := newModel("", Snapshot{}, cols)
	m = feed(t, m, m.refreshLocal())
	view := m.render()
	if !strings.Contains(view, "main") || !strings.Contains(view, "state error: git config failed") {
		t.Fatalf("a state read failure blanked the worktrees panel\n%s", view)
	}
}

// TestExplicitLockRefreshVerifiesAndStampsTime pins that startup verifies, the
// explicit action verifies again, and the displayed time advances.
func TestExplicitLockRefreshVerifiesAndStampsTime(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	if !m.locksRunning {
		t.Fatal("startup did not begin a lock verification")
	}
	m = feed(t, m, m.refreshLocks())
	if m.snap.LocksAt.IsZero() || len(m.snap.Locks) != 1 {
		t.Fatalf("startup verification = %v bytes at %v", m.snap.Locks, m.snap.LocksAt)
	}
	first := m.snap.LocksAt
	time.Sleep(2 * time.Millisecond)
	next, cmd := m.press("l")
	if cmd == nil {
		t.Fatal("l did not verify")
	}
	m = feed(t, next, cmd)
	if !m.snap.LocksAt.After(first) {
		t.Fatalf("explicit refresh did not advance the verification time: %v then %v", first, m.snap.LocksAt)
	}
	if got := count(*calls, "locks"); got != 2 {
		t.Fatalf("locks verified %d times, want 2: %v", got, *calls)
	}
	running := m
	running.locksRunning = true
	if _, again := running.press("l"); again != nil {
		t.Fatal("an overlapping lock refresh ran")
	}
}

// TestStartupTrackerBlocksManualRefresh pins that a manual tracker refresh
// cannot overlap the startup request.
func TestStartupTrackerBlocksManualRefresh(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	if !m.trackerRunning {
		t.Fatal("startup did not begin a tracker load")
	}
	if _, cmd := m.press("t"); cmd != nil {
		t.Fatal("t overlapped the startup tracker request")
	}
	if got := count(*calls, "tracker"); got != 0 {
		t.Fatalf("an overlapping tracker load ran: %v", *calls)
	}
	m = feed(t, m, m.refreshTracker())
	if m.trackerRunning {
		t.Fatal("a tracker result did not clear the guard")
	}
	if _, cmd := m.press("t"); cmd == nil {
		t.Fatal("t did not query after startup finished")
	}
}

// TestBenchRefreshUsesTheModelCheckout pins that reporting uses the dashboard's
// resolved checkout rather than re-discovering one.
func TestBenchRefreshUsesTheModelCheckout(t *testing.T) {
	var got checkout.Checkout
	cols, _ := recording()
	cols.bench = func(c checkout.Checkout) (string, error) {
		got = c
		return "split 2026-09-15", nil
	}
	m := newModel("/tmp/here", Snapshot{}, cols)
	m = feed(t, m, m.refreshBench())
	if got.Root != "/tmp/here" {
		t.Fatalf("bench reported for %q, want the model's checkout", got.Root)
	}
	if m.snap.Bench == nil || m.snap.Bench.Report != "split 2026-09-15" {
		t.Fatalf("bench result did not reach the snapshot: %+v", m.snap.Bench)
	}
}
