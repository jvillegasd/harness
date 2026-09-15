package dash

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvillegasd/harness/internal/locks"
	"github.com/jvillegasd/harness/internal/state"
)

func ptr(n int) *int { return &n }

// TestWorktreesSortAndMarkLegacy walks the seam-3 snapshot: worktrees sort by escalations then attempts, highest first, and a legacy value is visibly not a number.
func TestWorktreesSortAndMarkLegacy(t *testing.T) {
	m := New("", Snapshot{Worktrees: []Worktree{
		{Branch: "light", State: state.State{Attempts: ptr(4), Escalations: ptr(0), Ladder: "vet"}},
		{Branch: "hot", State: state.State{Attempts: ptr(5), Escalations: ptr(2)}},
		{Branch: "old", Dirty: true, State: state.State{Escalations: ptr(1), Legacy: map[string]string{"attempts": "a paragraph"}}},
	}})
	view := m.render()
	if i, j, k := strings.Index(view, "hot"), strings.Index(view, "old"), strings.Index(view, "light"); !(i >= 0 && i < j && j < k) {
		t.Fatalf("worktrees not sorted escalations-then-attempts: hot=%d old=%d light=%d\n%s", i, j, k, view)
	}
	if !strings.Contains(view, "legacy:a parag~") {
		t.Fatalf("legacy attempts value not marked\n%s", view)
	}
	if !strings.Contains(view, "yes") {
		t.Fatalf("dirty worktree not shown\n%s", view)
	}
}

func TestLocksPanelShowsEveryVerifyResult(t *testing.T) {
	m := New("", Snapshot{Locks: []locks.LockResult{
		{Name: "impeccable", Status: "match"},
		{Name: "tdd", Status: "drift", Detail: "recorded a, computed b"},
		{Name: "handoff", Status: "unverifiable"},
	}})
	view := m.render()
	for _, want := range []string{"impeccable: match", "tdd: drift (recorded a, computed b)", "handoff: unverifiable"} {
		if !strings.Contains(view, want) {
			t.Fatalf("locks panel missing %q\n%s", want, view)
		}
	}
}

func TestBenchPanelShowsCacheAgeOrHowToRefresh(t *testing.T) {
	empty := New("", Snapshot{}).render()
	if !strings.Contains(empty, "no cached result") || !strings.Contains(empty, "press r") {
		t.Fatalf("empty bench panel does not point at the refresh key\n%s", empty)
	}
	cached := New("", Snapshot{Bench: &BenchCache{At: time.Now().Add(-90 * time.Second), Report: "split 2026-09-15\n  turns 3.0"}}).render()
	if !strings.Contains(cached, "age 1m30s") || !strings.Contains(cached, "split 2026-09-15") {
		t.Fatalf("cached bench panel missing age or report\n%s", cached)
	}
}

func TestTrackerPanelCountsAndDegradesToOneLine(t *testing.T) {
	counts := New("", Snapshot{Tracker: Tracker{Labels: []LabelCount{
		{Label: "in-progress", Count: 1},
		{Label: "ready-for-agent", Count: 2},
	}}}).render()
	if !strings.Contains(counts, "in-progress: 1") || !strings.Contains(counts, "ready-for-agent: 2") {
		t.Fatalf("tracker panel missing counts\n%s", counts)
	}
	failed := New("", Snapshot{
		Worktrees: []Worktree{{Branch: "main", State: state.State{Attempts: ptr(0), Escalations: ptr(0)}}},
		Tracker:   Tracker{Err: "gh unavailable: not logged in"},
	}).render()
	if !strings.Contains(failed, "gh unavailable: not logged in") {
		t.Fatalf("tracker failure not shown\n%s", failed)
	}
	if !strings.Contains(failed, "main") {
		t.Fatalf("a local panel stopped rendering when the tracker failed\n%s", failed)
	}
}

func TestWorktreesPanelShowsLoadFailureNotAnEmptyList(t *testing.T) {
	view := New("", Snapshot{WorktreesErr: "wt: executable file not found"}).render()
	if !strings.Contains(view, "wt: executable file not found") {
		t.Fatalf("worktrees load failure not shown\n%s", view)
	}
}

// TestParseWorktreesReadsTheDirtyFlags pins the wt list schema: unknown field names would silently mark every worktree clean.
func TestParseWorktreesReadsTheDirtyFlags(t *testing.T) {
	listing := `{"schema":2,"items":[
	  {"branch":"main","worktree":{"path":"/r","changes":{"staged":false,"modified":false,"untracked":false,"renamed":false,"deleted":false,"conflicted":false,"diff":{"added":0,"deleted":0}}}},
	  {"branch":"feat/x","worktree":{"path":"/w","changes":{"staged":true,"modified":false,"untracked":true,"renamed":false,"deleted":false,"conflicted":false,"diff":{"added":3,"deleted":0}}}}
	]}`
	trees, err := parseWorktrees([]byte(listing))
	if err != nil {
		t.Fatal(err)
	}
	if len(trees) != 2 {
		t.Fatalf("parsed %d worktrees, want 2", len(trees))
	}
	if trees[0].Branch != "main" || trees[0].Dirty {
		t.Fatalf("clean worktree parsed wrong: %+v", trees[0])
	}
	if !trees[1].Dirty {
		t.Fatalf("dirty worktree not marked: %+v", trees[1])
	}
}

func TestTickerClearsItsInFlightGuard(t *testing.T) {
	m := New("", Snapshot{})
	next, first := m.Update(tickMsg(time.Now()))
	if first == nil || !next.(Model).localRunning {
		t.Fatalf("a tick did not start a guarded local reload: %v", first)
	}
	cleared, _ := next.(Model).Update(localMsg{})
	if cleared.(Model).localRunning {
		t.Fatal("a local result did not clear the in-flight guard")
	}
}

func TestTrackerRetryClearsStaleError(t *testing.T) {
	m := New("", Snapshot{Tracker: Tracker{Err: "gh unavailable: old"}})
	next, cmd := m.press("t")
	if cmd == nil {
		t.Fatal("t did not start a tracker query")
	}
	if strings.Contains(next.render(), "gh unavailable: old") {
		t.Fatalf("a tracker retry kept the stale error\n%s", next.render())
	}
}

func TestBenchPanelShowsRefreshErrors(t *testing.T) {
	m, _ := New("", Snapshot{}).press("r")
	model, _ := m.Update(benchMsg{err: "bench: --split is required"})
	if !strings.Contains(model.(Model).render(), "bench: --split is required") {
		t.Fatalf("bench refresh error not shown\n%s", model.(Model).render())
	}
}

func TestKeysRefreshInBackgroundAndQuit(t *testing.T) {
	m := New("", Snapshot{})
	next, cmd := m.press("r")
	if !next.benchRunning || cmd == nil {
		t.Fatalf("r did not start the bench refresh: running=%v cmd=%v", next.benchRunning, cmd)
	}
	if _, again := next.press("r"); again != nil {
		t.Fatal("a second r started a second bench refresh")
	}
	if still, _ := next.press("t"); !still.trackerRunning {
		t.Fatal("the tracker key stopped working while the bench was refreshing")
	}
	_, quit := next.press("q")
	if quit == nil {
		t.Fatal("q did not quit while a refresh was running")
	}
	if _, ok := quit().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want tea.QuitMsg", quit())
	}
	if _, none := m.press("x"); none != nil {
		t.Fatal("an unhandled key returned a command")
	}
}

func TestMessagesReachTheViewAndTickReloads(t *testing.T) {
	m, _ := New("", Snapshot{}).press("r")
	running, _ := m.press("r")
	model, _ := running.Update(benchMsg{cache: &BenchCache{At: time.Now(), Report: "split 2026-09-15"}})
	done := model.(Model)
	if done.benchRunning {
		t.Fatal("a bench result did not clear the running flag")
	}
	if !strings.Contains(done.render(), "split 2026-09-15") {
		t.Fatal("a bench result did not reach the view")
	}
	_, tickCmd := done.Update(tickMsg(time.Now()))
	if tickCmd == nil {
		t.Fatal("a tick did not request a local reload")
	}
	batch, ok := tickCmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("a tick must reload locally and re-arm the periodic tick, got %T with %d commands", tickCmd(), len(batch))
	}
	tracked, _ := done.Update(trackerMsg{tracker: Tracker{Labels: []LabelCount{{Label: "in-progress", Count: 3}}}})
	if !strings.Contains(tracked.(Model).render(), "in-progress: 3") {
		t.Fatal("a tracker result did not reach the view")
	}
}
