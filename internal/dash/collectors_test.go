package dash

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/skills"
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
		skills: func(checkout.Checkout) ([]skills.LockResult, error) {
			add("skills")
			return []skills.LockResult{{Name: "tdd", Status: "match"}}, nil
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
	if got := count(*calls, "skills"); got != 0 {
		t.Fatalf("a local refresh verified skills: %v", *calls)
	}
	if len(*calls) != 2 {
		t.Fatalf("local refresh did %v, want worktrees then states", *calls)
	}
	if m.snap.Worktrees[1].State.Ladder != "vet" {
		t.Fatalf("the snapshot did not reach the displayed branch: %+v", m.snap.Worktrees[1])
	}
}

// TestTicksReloadLocallyWithoutVerifyingSkills pins that a routine tick runs the
// local reload, which verifies no skills.
func TestTicksReloadLocallyWithoutVerifyingSkills(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	m = feed(t, m, m.refreshSkills())
	before := count(*calls, "skills")
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
	if got := count(*calls, "skills"); got != before {
		t.Fatalf("a tick's local reload verified skills: %v", *calls)
	}
	if got := count(*calls, "states"); got != 1 {
		t.Fatalf("the tick's local reload read branch state %d times: %v", got, *calls)
	}
	if after.snap.SkillsAt.IsZero() {
		t.Fatal("the tick dropped the startup verification")
	}
}

// TestInitStartsEveryStartupLoad pins that Init verifies skills, loads the
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
	for _, want := range []string{"worktrees", "states", "skills", "tracker"} {
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

// TestExplicitSkillsRefreshVerifiesAndStampsTime pins that startup verifies, the
// explicit action verifies again, and the displayed time advances.
func TestExplicitSkillsRefreshVerifiesAndStampsTime(t *testing.T) {
	cols, calls := recording()
	m := newModel("", Snapshot{}, cols)
	if !m.skillsRunning {
		t.Fatal("startup did not begin a skills verification")
	}
	m = feed(t, m, m.refreshSkills())
	if m.snap.SkillsAt.IsZero() || len(m.snap.Skills) != 1 {
		t.Fatalf("startup verification = %v bytes at %v", m.snap.Skills, m.snap.SkillsAt)
	}
	first := m.snap.SkillsAt
	time.Sleep(2 * time.Millisecond)
	next, cmd := m.press("s")
	if cmd == nil {
		t.Fatal("s did not verify")
	}
	m = feed(t, next, cmd)
	if !m.snap.SkillsAt.After(first) {
		t.Fatalf("explicit refresh did not advance the verification time: %v then %v", first, m.snap.SkillsAt)
	}
	if got := count(*calls, "skills"); got != 2 {
		t.Fatalf("skills verified %d times, want 2: %v", got, *calls)
	}
	running := m
	running.skillsRunning = true
	if _, again := running.press("s"); again != nil {
		t.Fatal("an overlapping skills refresh ran")
	}
	if _, old := m.press("l"); old != nil {
		t.Fatal("the retired l key still refreshes skills")
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
