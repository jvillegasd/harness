package state

import (
	"os/exec"
	"testing"
)

// Legacy values remain readable until a canonical value replaces them.
func TestSnapshotStateResolvesStoreThenFallback(t *testing.T) {
	snap := Snapshot{
		"worktrunk.state.feat/x.vars.ladder":   "go test ./...",
		"harness.feat/x.reason":                "legacy reason",
		"worktrunk.state.feat/x.vars.attempts": "3",
		"harness.feat/x.escalations":           "a paragraph",
	}
	x := snap.State("feat/x")
	if x.Branch != "feat/x" || x.Store != "harness" {
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

	none := snap.State("none")
	if none.Attempts == nil || *none.Attempts != 0 || none.Escalations == nil || *none.Escalations != 0 {
		t.Fatalf("an absent counter should read 0, got %v/%v", none.Attempts, none.Escalations)
	}
}

// Canonical state wins even when both stores contain a value.
func TestCanonicalStateTakesPrecedence(t *testing.T) {
	snap := Snapshot{
		"worktrunk.state.main.vars.ladder": "w",
		"harness.main.ladder":              "h",
	}
	if got := snap.State("main").Ladder; got != "h" {
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
