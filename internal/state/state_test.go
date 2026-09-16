package state

import (
	"os/exec"
	"testing"
)

// TestSnapshotStateResolvesStoreThenFallback pins the precedence the dashboard
// depends on: this branch's store first, then the other store, with legacy text
// marked rather than parsed.
func TestSnapshotStateResolvesStoreThenFallback(t *testing.T) {
	snap := Snapshot{
		"worktrunk.state.feat/x.vars.ladder":   "go test ./...",
		"harness.feat/x.reason":                "legacy reason",
		"worktrunk.state.feat/x.vars.attempts": "3",
		"harness.feat/x.escalations":           "a paragraph",
	}
	x := snap.State("worktrunk", "feat/x")
	if x.Branch != "feat/x" || x.Store != "worktrunk" {
		t.Fatalf("identity = %q/%q", x.Branch, x.Store)
	}
	if x.Ladder != "go test ./..." {
		t.Fatalf("ladder = %q, want the worktrunk key", x.Ladder)
	}
	if x.Reason != "legacy reason" {
		t.Fatalf("reason = %q, want the harness fallback", x.Reason)
	}
	if x.Attempts == nil || *x.Attempts != 3 {
		t.Fatalf("attempts = %v, want 3", x.Attempts)
	}
	if x.Escalations != nil {
		t.Fatalf("a legacy counter should leave the typed field nil, got %d", *x.Escalations)
	}
	if x.Legacy["escalations"] != "a paragraph" {
		t.Fatalf("legacy escalations = %q", x.Legacy["escalations"])
	}

	none := snap.State("worktrunk", "none")
	if none.Attempts == nil || *none.Attempts != 0 || none.Escalations == nil || *none.Escalations != 0 {
		t.Fatalf("an absent counter should read 0, got %v/%v", none.Attempts, none.Escalations)
	}
}

// TestSnapshotStateFollowsTheBranchStore pins that store selection, not a fixed
// key, decides which store wins.
func TestSnapshotStateFollowsTheBranchStore(t *testing.T) {
	snap := Snapshot{
		"worktrunk.state.main.vars.ladder": "w",
		"harness.main.ladder":              "h",
	}
	if got := snap.State("worktrunk", "main").Ladder; got != "w" {
		t.Fatalf("worktrunk store read %q", got)
	}
	if got := snap.State("harness", "main").Ladder; got != "h" {
		t.Fatalf("harness store read %q", got)
	}
}

func TestReadSnapshotReadsLocalState(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "--local", "worktrunk.state.a.vars.ladder", "vet"},
		{"config", "--local", "harness.a.reason", "legacy"},
		{"config", "--local", "harness.b.attempts", "2"},
		{"config", "--local", "unrelated.key", "ignored"},
	} {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	snap, err := ReadSnapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap["worktrunk.state.a.vars.ladder"] != "vet" {
		t.Fatalf("worktrunk key = %q", snap["worktrunk.state.a.vars.ladder"])
	}
	if snap["harness.a.reason"] != "legacy" {
		t.Fatalf("harness key = %q", snap["harness.a.reason"])
	}
	if _, ok := snap["unrelated.key"]; ok {
		t.Fatal("a non-state key was read into the snapshot")
	}

	empty := t.TempDir()
	cmd := exec.Command(gitPath, "init", "-q")
	cmd.Dir = empty
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSnapshot(empty)
	if err != nil || len(got) != 0 {
		t.Fatalf("an empty repository = %v, %v", got, err)
	}
}
