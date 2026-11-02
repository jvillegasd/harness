package main

// Regression checks for frontier promotion in reconcile-agent-lifecycle.
//
// Planning-only tickets carry no lifecycle label and must stay off the ready
// frontier until explicit promotion labels them. Each case mirrors a real
// reconcile path with an in-memory fake of the issue API.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newIssue(number int, state string, labels ...string) *issue {
	return &issue{Number: number, State: state, Labels: labelList(labels)}
}

type fakeAPI struct {
	issues    map[int]*issue
	satisfied map[int]bool
	blockedBy map[int][]int
	blocking  map[int][]int
	parents   map[int][]int
	updates   []updateCall
	history   map[int][]event
	// foreign holds issues from other repositories, listed under a local issue's relation; their numbers may collide with local issues.
	foreign map[int]map[string][]issue
}

type updateCall struct {
	Number int
	Labels []string
	State  string
}

func (u updateCall) String() string {
	parts := []string{strconv.Itoa(u.Number)}
	if u.State != "" {
		parts = append(parts, "state="+u.State)
	}
	if u.Labels != nil {
		parts = append(parts, "labels="+strings.Join(u.Labels, ","))
	}
	return strings.Join(parts, " ")
}

func newFake(issues map[int]*issue, satisfied []int, blockedBy, blocking, parents map[int][]int) *fakeAPI {
	f := &fakeAPI{
		issues:    issues,
		satisfied: map[int]bool{},
		blockedBy: blockedBy,
		blocking:  blocking,
		parents:   parents,
		history:   map[int][]event{},
	}
	for _, number := range satisfied {
		f.satisfied[number] = true
	}
	for number, value := range issues {
		for _, name := range value.Labels {
			f.history[number] = append(f.history[number], event{Event: "labeled", Label: label{Name: name}})
		}
	}
	return f
}

func (f *fakeAPI) issue(number int) (issue, error) {
	return *f.issues[number], nil
}

func (f *fakeAPI) related(number int, relation string) ([]issue, error) {
	switch relation {
	case "sub_issues":
		var children []issue
		for n, parents := range f.parents {
			for _, parent := range parents {
				if parent == number {
					children = append(children, *f.issues[n])
				}
			}
		}
		return append(children, f.foreign[number][relation]...), nil
	case "blocked_by":
		return append(f.byIDs(f.blockedBy[number]), f.foreign[number][relation]...), nil
	case "blocking":
		return append(f.byIDs(f.blocking[number]), f.foreign[number][relation]...), nil
	case "parent":
		return append(f.byIDs(f.parents[number]), f.foreign[number][relation]...), nil
	}
	return nil, nil
}

func (f *fakeAPI) byIDs(numbers []int) []issue {
	result := make([]issue, 0, len(numbers))
	for _, number := range numbers {
		result = append(result, *f.issues[number])
	}
	return result
}

func (f *fakeAPI) events(number int) ([]event, error) {
	return f.history[number], nil
}

func (f *fakeAPI) blockerSatisfied(blocker issue) (bool, error) {
	if blocker.Foreign {
		// A foreign issue's evidence lives in its own repository; the fake has none, so it never counts as merged.
		return false, nil
	}
	return blocker.State == "closed" && f.satisfied[blocker.Number], nil
}

func (f *fakeAPI) openIssues(name string) ([]issue, error) {
	var result []issue
	for _, value := range f.issues {
		if value.State == "open" && labelNames(*value)[name] {
			result = append(result, *value)
		}
	}
	return result, nil
}

func foreignIssue(number int, state string, labels ...string) issue {
	return issue{Number: number, State: state, Labels: labelList(labels), RepositoryURL: "https://api.github.com/repos/o/other", Foreign: true}
}

func (f *fakeAPI) update(number int, changes map[string]any) error {
	call := updateCall{Number: number}
	if raw, ok := changes["labels"].([]string); ok {
		call.Labels = raw
	}
	if state, ok := changes["state"].(string); ok {
		call.State = state
	}
	f.updates = append(f.updates, call)

	value := f.issues[number]
	before := labelNames(*value)
	if raw, ok := changes["labels"].([]string); ok {
		value.Labels = labelList(raw)
	}
	if state, ok := changes["state"].(string); ok {
		value.State = state
	}
	after := labelNames(*value)
	for name := range after {
		if !before[name] {
			f.history[number] = append(f.history[number], event{Event: "labeled", Label: label{Name: name}})
		}
	}
	for name := range before {
		if !after[name] {
			f.history[number] = append(f.history[number], event{Event: "unlabeled", Label: label{Name: name}})
		}
	}
	return nil
}

func assertUpdates(t *testing.T, f *fakeAPI, want ...string) {
	t.Helper()
	if len(f.updates) != len(want) {
		t.Fatalf("updates = %v, want %v", f.updates, want)
	}
	for i, call := range f.updates {
		if got := call.String(); got != want[i] {
			t.Fatalf("update[%d] = %q, want %q (all: %v)", i, got, want[i], f.updates)
		}
	}
}

func assertLabels(t *testing.T, f *fakeAPI, number int, want ...string) {
	t.Helper()
	got := labelNames(*f.issues[number])
	expected := labelSet{}
	for _, name := range want {
		expected[name] = true
	}
	if !got.equals(expected) {
		t.Fatalf("issue %d labels = %v, want %v", number, got.sorted(), expected.sorted())
	}
}

func assertNoNewUpdates(t *testing.T, f *fakeAPI, run func() error) {
	t.Helper()
	before := len(f.updates)
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if len(f.updates) != before {
		t.Fatalf("unexpected updates: %v", f.updates[before:])
	}
}

func TestPlanningOnlyTicketNotPromoted(t *testing.T) {
	f := newFake(map[int]*issue{1: newIssue(1, "open")}, nil, nil, nil, nil)
	if err := reconcileFrontier(f, 1, defaultLabels(), false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)
}

func TestPlanningOnlyTicketNotPromotedOnReopen(t *testing.T) {
	f := newFake(map[int]*issue{1: newIssue(1, "open")}, nil, nil, nil, nil)
	if err := reconcileFrontier(f, 1, defaultLabels(), true); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)
}

func TestPlanningOnlyDependentNotPromotedWhenBlockerCloses(t *testing.T) {
	f := newFake(
		map[int]*issue{1: newIssue(1, "closed"), 2: newIssue(2, "open")},
		[]int{1}, map[int][]int{2: {1}}, map[int][]int{1: {2}}, nil,
	)
	if err := reconcile(f, 1, defaultLabels(), ""); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)
}

func TestBlockedTicketBecomesReadyWhenBlockerCloses(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "open", labels["blocked"]),
		},
		[]int{1}, map[int][]int{2: {1}}, map[int][]int{1: {2}}, nil,
	)
	if err := reconcile(f, 1, labels, ""); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "2 labels="+labels["ready_for_agent"])
}

func TestReadyTicketBecomesBlockedWhileBlockerOpen(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", labels["ready_for_agent"]),
			2: newIssue(2, "open"),
		},
		nil, map[int][]int{1: {2}}, nil, nil,
	)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["blocked"])
}

func TestReopenedInProgressTicketResetsToReady(t *testing.T) {
	labels := defaultLabels()
	f := newFake(map[int]*issue{1: newIssue(1, "open", labels["in_progress"])}, nil, nil, nil, nil)
	if err := reconcileFrontier(f, 1, labels, true); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["ready_for_agent"])
}

func TestActiveTicketWithNoBlockersUntouched(t *testing.T) {
	labels := defaultLabels()
	f := newFake(map[int]*issue{1: newIssue(1, "open", labels["in_progress"])}, nil, nil, nil, nil)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)
}

func TestReleasedPairKeepsReadyOverReadyForHuman(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{1: newIssue(1, "open", labels["ready_for_agent"], labels["ready_for_human"])},
		nil, nil, nil, nil,
	)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["ready_for_agent"])
}

func TestReleasedPairKeepsReadyOverInProgress(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{1: newIssue(1, "open", labels["ready_for_agent"], labels["in_progress"])},
		nil, nil, nil, nil,
	)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["ready_for_agent"])
}

func TestReleasedPairKeepsBlockedOverReadyForHuman(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", labels["blocked"], labels["ready_for_human"]),
			2: newIssue(2, "open"),
		},
		nil, map[int][]int{1: {2}}, nil, nil,
	)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["blocked"])
}

func TestReleasedPairKeepsBlockedOverInProgress(t *testing.T) {
	labels := defaultLabels()
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", labels["blocked"], labels["in_progress"]),
			2: newIssue(2, "open"),
		},
		nil, map[int][]int{1: {2}}, nil, nil,
	)
	if err := reconcileFrontier(f, 1, labels, false); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f, "1 labels="+labels["blocked"])
}

func TestCompletedParentLosesLifecycleLabels(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "closed", "in-progress", "feature"),
		},
		[]int{1}, nil, nil, map[int][]int{1: {2}},
	)
	if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	if f.issues[2].State != "closed" {
		t.Fatalf("issue 2 state = %q, want closed", f.issues[2].State)
	}
	assertLabels(t, f, 2, "feature")
}

func TestAlreadyClosedParentRepairsLabelsOnce(t *testing.T) {
	labels := map[string]string{
		"blocked":         "waiting",
		"ready_for_agent": "queued",
		"in_progress":     "working",
		"ready_for_human": "review",
	}
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "closed", "waiting", "queued", "working", "review", "feature"),
		},
		[]int{1}, nil, nil, map[int][]int{1: {2}},
	)
	if err := reconcile(f, 2, labels, "closed"); err != nil {
		t.Fatal(err)
	}
	if f.issues[2].State != "closed" {
		t.Fatalf("issue 2 state = %q, want closed", f.issues[2].State)
	}
	assertLabels(t, f, 2, "feature")
	assertNoNewUpdates(t, f, func() error { return reconcile(f, 2, labels, "closed") })
}

func TestCleanedParentReopensWhenChildReopens(t *testing.T) {
	labels := []map[string]string{
		defaultLabels(),
		{
			"blocked":         "waiting",
			"ready_for_agent": "queued",
			"in_progress":     "working",
			"ready_for_human": "review",
		},
	}
	for _, label := range labels {
		f := newFake(
			map[int]*issue{
				1: newIssue(1, "closed", label["ready_for_human"]),
				2: newIssue(2, "open", label["in_progress"], "feature"),
			},
			[]int{1}, nil, nil, map[int][]int{1: {2}},
		)
		if err := reconcile(f, 1, label, "closed"); err != nil {
			t.Fatal(err)
		}
		assertLabels(t, f, 2, "feature")

		if err := f.update(1, map[string]any{"state": "open"}); err != nil {
			t.Fatal(err)
		}
		delete(f.satisfied, 1)
		if err := reconcile(f, 1, label, "reopened"); err != nil {
			t.Fatal(err)
		}
		if f.issues[2].State != "open" {
			t.Fatalf("issue 2 state = %q, want open", f.issues[2].State)
		}
		assertLabels(t, f, 2, label["in_progress"], "feature")
		assertNoNewUpdates(t, f, func() error { return reconcile(f, 1, label, "reopened") })
	}
}

func TestAlreadyReopenedParentRecoversLifecycleLabel(t *testing.T) {
	for _, eventNumber := range []int{1, 2} {
		f := newFake(
			map[int]*issue{
				1: newIssue(1, "closed", "ready-for-human"),
				2: newIssue(2, "open", "in-progress", "feature"),
			},
			[]int{1}, nil, nil, map[int][]int{1: {2}},
		)
		if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
			t.Fatal(err)
		}
		if err := f.update(2, map[string]any{"state": "open"}); err != nil {
			t.Fatal(err)
		}
		if err := f.update(1, map[string]any{"state": "open"}); err != nil {
			t.Fatal(err)
		}
		if err := reconcile(f, eventNumber, defaultLabels(), "reopened"); err != nil {
			t.Fatal(err)
		}
		if f.issues[2].State != "open" {
			t.Fatalf("event %d: issue 2 state = %q, want open", eventNumber, f.issues[2].State)
		}
		assertLabels(t, f, 2, "in-progress", "feature")
		assertNoNewUpdates(t, f, func() error { return reconcile(f, 1, defaultLabels(), "") })
	}
}

func TestReopenedParentStillRespectsItsBlockers(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "open", "in-progress", "feature"),
			3: newIssue(3, "open"),
		},
		[]int{1}, map[int][]int{2: {3}}, nil, map[int][]int{1: {2}},
	)
	if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	if err := f.update(2, map[string]any{"state": "open"}); err != nil {
		t.Fatal(err)
	}
	if err := f.update(1, map[string]any{"state": "open"}); err != nil {
		t.Fatal(err)
	}
	if err := reconcile(f, 2, defaultLabels(), "reopened"); err != nil {
		t.Fatal(err)
	}
	if f.issues[2].State != "open" {
		t.Fatalf("issue 2 state = %q, want open", f.issues[2].State)
	}
	assertLabels(t, f, 2, "blocked", "feature")
}

func TestPlanningOnlyParentStaysUnmanaged(t *testing.T) {
	for _, state := range []string{"open", "closed"} {
		f := newFake(
			map[int]*issue{
				1: newIssue(1, "closed", "ready-for-human"),
				2: newIssue(2, state, "feature"),
			},
			[]int{1}, nil, nil, map[int][]int{1: {2}},
		)
		if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
			t.Fatal(err)
		}
		assertUpdates(t, f)

		if err := f.update(1, map[string]any{"state": "open"}); err != nil {
			t.Fatal(err)
		}
		assertNoNewUpdates(t, f, func() error { return reconcile(f, 1, defaultLabels(), "") })
		if f.issues[2].State != state {
			t.Fatalf("issue 2 state = %q, want %q", f.issues[2].State, state)
		}
		assertLabels(t, f, 2, "feature")
	}
}

func TestParentWaitsForEveryChildMerge(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "open", "in-progress", "feature"),
			3: newIssue(3, "closed"),
		},
		[]int{1}, nil, nil, map[int][]int{1: {2}, 3: {2}},
	)
	if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)

	f.satisfied[3] = true
	if err := reconcile(f, 3, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	if f.issues[2].State != "closed" {
		t.Fatalf("issue 2 state = %q, want closed", f.issues[2].State)
	}
	assertLabels(t, f, 2, "feature")
}

func TestClosedStandaloneTicketKeepsLabels(t *testing.T) {
	f := newFake(
		map[int]*issue{1: newIssue(1, "closed", "ready-for-human", "feature")},
		nil, nil, nil, nil,
	)
	if err := reconcile(f, 1, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertUpdates(t, f)
}

// TestReopenedParentIsNotReadyForAgent replays a path the statechart pilot found: a parent closed by hand and reopened before the workflow ran keeps in-progress, and the reopened run must not reset it to ready-for-agent like a ticket.
func TestReopenedParentIsNotReadyForAgent(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", "ready-for-agent"),
			2: newIssue(2, "open", "in-progress", "feature"),
		},
		nil, nil, nil, map[int][]int{1: {2}},
	)
	if err := reconcile(f, 2, defaultLabels(), "reopened"); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 2, "in-progress", "feature")
}

// TestParentRunHealsAChildsFrontier replays the other path: the workflow's concurrency group keeps one pending run, so a blocker's close event can be replaced by a later event in the same tree. The later run must still unblock the dependent.
func TestParentRunHealsAChildsFrontier(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed", "ready-for-human"),
			3: newIssue(3, "open", "blocked"),
			2: newIssue(2, "open", "in-progress"),
		},
		[]int{1}, map[int][]int{3: {1}}, map[int][]int{1: {3}}, map[int][]int{1: {2}, 3: {2}},
	)
	if err := reconcile(f, 2, defaultLabels(), "reopened"); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 3, "ready-for-agent")
}

// TestReleasedParentGoesBackInProgress covers the release path: a parent that was blocked by its own blocker returns to in-progress when that blocker merges, never ready-for-agent, because it is a container rather than work.
func TestReleasedParentGoesBackInProgress(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", "in-progress"),
			2: newIssue(2, "open", "blocked", "feature"),
			3: newIssue(3, "closed", "ready-for-human"),
		},
		[]int{3}, map[int][]int{2: {3}}, map[int][]int{3: {2}}, map[int][]int{1: {2}},
	)
	if err := reconcile(f, 3, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 2, "in-progress", "feature")
}

// TestSiblingEventHealsADependent is the common case of a dropped event: the next event is a sibling's close, and the run through the shared parent unblocks the dependent whose blocker already merged.
func TestSiblingEventHealsADependent(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed", "ready-for-human"),
			3: newIssue(3, "open", "blocked"),
			4: newIssue(4, "closed", "ready-for-human"),
			2: newIssue(2, "open", "in-progress"),
		},
		[]int{1, 4}, map[int][]int{3: {1}}, map[int][]int{1: {3}}, map[int][]int{1: {2}, 3: {2}, 4: {2}},
	)
	if err := reconcile(f, 4, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 3, "ready-for-agent")
}

func TestDeployedCopyMatchesHarnessCopy(t *testing.T) {
	harnessCopy, err := os.ReadFile("reconcile-agent-lifecycle.go")
	if err != nil {
		t.Fatal(err)
	}
	deployed := filepath.Join("..", "..", "..", "..", ".github", "scripts", "reconcile-agent-lifecycle.go")
	deployedCopy, err := os.ReadFile(deployed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(harnessCopy, deployedCopy) {
		t.Fatalf("harness and deployed reconciler copies differ: %s", deployed)
	}
}

// TestGoRunWithoutModule proves the file is standard-library only: it compiles
// and runs through `go run` in a directory with no Go module.
func TestGoRunWithoutModule(t *testing.T) {
	source, err := os.ReadFile("reconcile-agent-lifecycle.go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "reconcile-agent-lifecycle.go"), source, 0o644); err != nil {
		t.Fatal(err)
	}
	eventPath := filepath.Join(dir, "event.json")
	if err := os.WriteFile(eventPath, []byte(`{"issue":{"number":1},"action":"closed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	gh := filepath.Join(dir, "gh")
	script := "#!/bin/sh\ncase \"$*\" in\n  *sub_issues*|*blocking*|*blocked_by*|*events*) printf '[]\\n' ;;\n  *) printf '{}\\n' ;;\nesac\n"
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	command := exec.Command("go", "run", "reconcile-agent-lifecycle.go")
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GITHUB_REPOSITORY=owner/repo",
		"GITHUB_EVENT_PATH="+eventPath,
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("go run failed with no module: %v\n%s", err, out)
	}
}

// TestSweepHealsAnUnrelatedTree replays a dropped event no later event reaches: the blocker merged, nothing in its tree moved since, and the scheduled sweep releases the dependent while a planning-only ticket stays off the frontier.
func TestSweepHealsAnUnrelatedTree(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed", "ready-for-human"),
			3: newIssue(3, "open", "blocked"),
			4: newIssue(4, "open", "feature"),
		},
		[]int{1}, map[int][]int{3: {1}, 4: {1}}, map[int][]int{1: {3, 4}}, nil,
	)
	if err := sweep(f, defaultLabels()); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 3, "ready-for-agent")
	assertLabels(t, f, 4, "feature")
	assertNoNewUpdates(t, f, func() error { return sweep(f, defaultLabels()) })
}

// TestSweepClosesAParentWhoseChildrenMerged covers the parent half of a dropped event: the last child merged and the sweep closes the parent.
func TestSweepClosesAParentWhoseChildrenMerged(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", "in-progress"),
			2: newIssue(2, "closed", "ready-for-human"),
		},
		[]int{2}, nil, nil, map[int][]int{2: {1}},
	)
	if err := sweep(f, defaultLabels()); err != nil {
		t.Fatal(err)
	}
	if f.issues[1].State != "closed" {
		t.Fatalf("parent state = %s, want closed; updates %v", f.issues[1].State, f.updates)
	}
}

// TestForeignIssuesAreNeverRelabeled gives a parent a sub-issue and a dependent from another repository whose numbers collide with local issues. The local issues must stay untouched, and the open foreign child keeps the parent open.
func TestForeignIssuesAreNeverRelabeled(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", "in-progress"),
			5: newIssue(5, "open", "ready-for-agent"),
			6: newIssue(6, "open"),
			7: newIssue(7, "closed", "ready-for-human"),
		},
		[]int{7}, map[int][]int{5: {6}}, nil, map[int][]int{7: {1}},
	)
	f.foreign = map[int]map[string][]issue{
		1: {"sub_issues": {foreignIssue(5, "open", "blocked")}},
		7: {"blocking": {foreignIssue(5, "open", "blocked")}},
	}
	if err := reconcile(f, 7, defaultLabels(), "closed"); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.updates {
		if call.Number == 5 {
			t.Fatalf("local #5 was relabeled for a foreign #5: %v", f.updates)
		}
	}
	if f.issues[1].State != "open" {
		t.Fatalf("parent closed while a foreign child is open: %v", f.updates)
	}
}

// TestMarkFlagsOnlyOtherRepositories pins the foreign check on the API's repository_url, case-insensitively, with a missing URL counting as this repository.
func TestMarkFlagsOnlyOtherRepositories(t *testing.T) {
	g := &gitHubAPI{repository: "Owner/Repo"}
	values := g.mark([]issue{
		{Number: 1, RepositoryURL: "https://api.github.com/repos/owner/repo"},
		{Number: 2, RepositoryURL: "https://api.github.com/repos/owner/other"},
		{Number: 3},
	})
	if values[0].Foreign || !values[1].Foreign || values[2].Foreign {
		t.Fatalf("foreign flags = %v %v %v, want false true false", values[0].Foreign, values[1].Foreign, values[2].Foreign)
	}
	if got := repositoryOf(values[1].RepositoryURL); got != "owner/other" {
		t.Fatalf("repositoryOf = %q, want owner/other", got)
	}
}

// TestSteadyReadyTicketKeepsItsLabel guards the no-change shortcut: re-reconciling a ready ticket whose blockers merged, as a sibling heal or the sweep does, must leave its label alone rather than strip it off the frontier.
func TestSteadyReadyTicketKeepsItsLabel(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed", "ready-for-human"),
			3: newIssue(3, "open", "ready-for-agent", "feature"),
			4: newIssue(4, "open", "ready-for-agent"),
		},
		[]int{1}, map[int][]int{3: {1}}, nil, nil,
	)
	for _, number := range []int{3, 4} {
		assertNoNewUpdates(t, f, func() error { return reconcileFrontier(f, number, defaultLabels(), false) })
	}
	assertLabels(t, f, 3, "ready-for-agent", "feature")
}

// TestReadyTicketThatGainsAChildGoesInProgress keeps "a parent is never ready-for-agent" when a ready ticket later gains a sub-issue: the frontier shortcut leaves its labels alone, so the parent pass must move it to in-progress.
func TestReadyTicketThatGainsAChildGoesInProgress(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "open", "ready-for-agent"),
			2: newIssue(2, "open", "ready-for-agent"),
		},
		nil, nil, nil, map[int][]int{2: {1}},
	)
	if err := sweep(f, defaultLabels()); err != nil {
		t.Fatal(err)
	}
	assertLabels(t, f, 1, "in-progress")
	assertLabels(t, f, 2, "ready-for-agent")
}

// TestSweepReopensAParentWhoseChildReopened heals a dropped reopen: the child is open again, its parent stayed closed, and the sweep reaches the parent through the child.
func TestSweepReopensAParentWhoseChildReopened(t *testing.T) {
	f := newFake(
		map[int]*issue{
			1: newIssue(1, "closed"),
			2: newIssue(2, "open", "ready-for-agent"),
		},
		nil, nil, nil, map[int][]int{2: {1}},
	)
	f.history[1] = []event{{Event: "labeled", Label: label{Name: "in-progress"}}}
	if err := sweep(f, defaultLabels()); err != nil {
		t.Fatal(err)
	}
	if f.issues[1].State != "open" {
		t.Fatalf("parent state = %s, want open; updates %v", f.issues[1].State, f.updates)
	}
	assertLabels(t, f, 1, "in-progress")
}

// TestBlockerSatisfiedReadsForeignEvidenceInItsRepository drives the gh-backed check with a stand-in gh: a foreign issue is looked up in its own repository, an unreadable one counts as unmerged without failing the run, and a failed lookup is not cached as unmerged.
func TestBlockerSatisfiedReadsForeignEvidenceInItsRepository(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	gh := filepath.Join(dir, "gh")
	script := `#!/bin/sh
echo "$*" >> "` + calls + `"
case "$*" in
  *"--repo o/private"*) echo "no access" >&2; exit 1 ;;
  *"issue view 7 --repo o/repo"*) echo '{"closedByPullRequestsReferences":[{"url":"u7"}]}' ;;
  *"pr view u7"*) echo "timeout" >&2; exit 1 ;;
  *"issue view"*) echo '{"closedByPullRequestsReferences":[{"url":"u"}]}' ;;
  *"pr view"*) echo '{"state":"MERGED","mergedAt":"2026-01-01T00:00:00Z"}' ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	g := &gitHubAPI{repository: "o/repo", binary: gh}
	foreign := issue{Number: 5, State: "closed", RepositoryURL: "https://api.github.com/repos/o/other", Foreign: true}
	if ok, err := g.blockerSatisfied(foreign); err != nil || !ok {
		t.Fatalf("foreign merged issue = %v, %v; want true, nil", ok, err)
	}
	private := issue{Number: 6, State: "closed", RepositoryURL: "https://api.github.com/repos/o/private", Foreign: true}
	if ok, err := g.blockerSatisfied(private); err != nil || ok {
		t.Fatalf("unreadable foreign issue = %v, %v; want false, nil", ok, err)
	}
	if _, err := g.blockerSatisfied(issue{Number: 7, State: "closed"}); err == nil {
		t.Fatal("a failed pull request lookup did not fail the check")
	}
	if cached, ok := g.merged["o/repo#7"]; ok {
		t.Fatalf("a failed lookup was cached as %v, which would mislead the next issue in a sweep", cached)
	}
	log, _ := os.ReadFile(calls)
	if !strings.Contains(string(log), "issue view 5 --repo o/other") {
		t.Fatalf("foreign issue was not looked up in its own repository:\n%s", log)
	}
}

// TestIssueMemoRefetchesAfterUpdate pins the memo: a second read in one run is served from memory, a write drops it, and the next read fetches fresh labels, so a PATCH never starts from a label set this run did not read after its own last write.
func TestIssueMemoRefetchesAfterUpdate(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	gh := filepath.Join(dir, "gh")
	script := `#!/bin/sh
echo "$*" >> "` + calls + `"
case "$*" in
  *"--method PATCH"*) cat >/dev/null ;;
  *"issues/5 "*) echo '{"number":5,"state":"open","labels":[{"name":"blocked"}]}' ;;
esac
`
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	g := &gitHubAPI{repository: "o/repo", binary: gh}
	count := func() int {
		log, _ := os.ReadFile(calls)
		return strings.Count(string(log), "--method GET repos/o/repo/issues/5 -H")
	}
	for i := 0; i < 2; i++ {
		if _, err := g.issue(5); err != nil {
			t.Fatal(err)
		}
	}
	if got := count(); got != 1 {
		t.Fatalf("two reads fetched %d times, want 1", got)
	}
	if err := g.update(5, map[string]any{"labels": []string{"ready-for-agent"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.issue(5); err != nil {
		t.Fatal(err)
	}
	if got := count(); got != 2 {
		t.Fatalf("a read after an update fetched %d times in total, want 2", got)
	}
}
