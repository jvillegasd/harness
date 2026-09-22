package state

import (
	"os/exec"
	"testing"
)

// Legacy values remain readable until a canonical value replaces them.
func TestSnapshotStatePrefersCanonicalThenLegacy(t *testing.T) {
	snap := Snapshot{
		"worktrunk.state.feat/x.vars.ladder":   "go test ./...",
		"harness.feat/x.reason":                "legacy reason",
		"worktrunk.state.feat/x.vars.attempts": "3",
		"harness.feat/x.escalations":           "a paragraph",
	}
	x := snap.State("feat/x")
	if x.Branch != "feat/x" {
		t.Fatalf("identity = %q", x.Branch)
	}
	if x.Ladder != "go test ./..." {
		t.Fatalf("ladder = %q, want the worktrunk key", x.Ladder)
	}
	if x.Reason != "legacy reason" {
		t.Fatalf("reason = %q, want the canonical value", x.Reason)
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

// legacyShapes are the counter values that must read as legacy: what agents wrote before the typed contract, plus integers in a non-canonical spelling.
var legacyShapes = map[string]string{
	"json object":    `{"review_round":2,"repair_passes":1}`,
	"empty array":    `[]`,
	"prose":          "round two raised new blocking findings in round-one code.\nStopped before round three.",
	"signed integer": "+5",
	"padded integer": "007",
	"negative":       "-1",
}

// A legacy counter leaves the typed field nil and carries its text verbatim, so an increment can name what it replaced.
func TestLegacyCounterShapesReadAsLegacy(t *testing.T) {
	for name, raw := range legacyShapes {
		if n, legacy := parseCounter(raw); legacy == nil || *legacy != raw || n != 0 {
			t.Errorf("%s: parseCounter(%q) = %d, %v; want 0 and the text verbatim", name, raw, n, legacy)
		}
		x := Snapshot{"harness.feat/x.attempts": raw}.State("feat/x")
		if x.Attempts != nil || x.Legacy["attempts"] != raw {
			t.Errorf("%s: attempts = %v, legacy = %q", name, x.Attempts, x.Legacy["attempts"])
		}
	}
	for raw, want := range map[string]int{"0": 0, "5": 5, "42": 42} {
		n, legacy := parseCounter(raw)
		if legacy != nil || n != want {
			t.Errorf("parseCounter(%q) = %d, %v; want %d and no legacy text", raw, n, legacy, want)
		}
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
