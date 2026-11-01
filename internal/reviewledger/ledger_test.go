package reviewledger

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestKeyKeepsHostOwnerAndPath(t *testing.T) {
	for source, want := range map[string]string{
		"git@github.com:jvillegasd/harness.git":    "github.com_jvillegasd_harness",
		"https://github.com/jvillegasd/harness/":   "github.com_jvillegasd_harness",
		"https://user:token@github.com/o/r.git":    "github.com_o_r",
		"ssh://git@host:22/x/y.git":                "host_22_x_y",
		"https://gitlab.com/group/sub/project.git": "gitlab.com_group_sub_project",
		"/Users/me/work/app":                       "Users_me_work_app",
		"/Users/me/b@c/app":                        "Users_me_b@c_app",
	} {
		if got := Key(source); got != want {
			t.Errorf("Key(%q) = %q, want %q", source, got, want)
		}
	}
	// Owner and repository names that share a dash stay distinct, which an owner-repo join did not.
	if Key("git@github.com:a-b/c.git") == Key("git@github.com:a/b-c.git") {
		t.Error("a-b/c and a/b-c share a key")
	}
}

func str(s string) *string { return &s }

func finding() Entry {
	return Entry{Branch: "feat", SHA: "abc", Round: 1, Depth: "standard", Path: str("a.go"), Lines: "40-52", Severity: "bug", Summary: "s", Disposition: "confirmed"}
}

func TestValidate(t *testing.T) {
	if err := Validate(finding()); err != nil {
		t.Fatalf("a complete finding was rejected: %v", err)
	}
	whole := Entry{Branch: "feat", SHA: "abc", Round: 1, Depth: "none", Disposition: "no-review"}
	if err := Validate(whole); err != nil {
		t.Fatalf("a whole-review line was rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Entry){
		"unknown disposition": func(e *Entry) { e.Disposition = "dismissed" },
		"unknown depth":       func(e *Entry) { e.Depth = "huge" },
		"third round":         func(e *Entry) { e.Round = 3 },
		"no branch":           func(e *Entry) { e.Branch = "" },
		"no path":             func(e *Entry) { e.Path = nil },
		"bad severity":        func(e *Entry) { e.Severity = "major" },
		"bad lines":           func(e *Entry) { e.Lines = "40-x" },
		"no summary":          func(e *Entry) { e.Summary = "" },
		"whole with a path":   func(e *Entry) { e.Disposition = "clean" },
	} {
		e := finding()
		mutate(&e)
		if err := Validate(e); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}

func TestLatestKeepsRewrittenOpenFindings(t *testing.T) {
	a := finding()
	a.ID = "A"
	aFixed := a
	aFixed.Disposition = "fixed"
	b := finding()
	b.ID, b.SHA, b.Disposition = "B", "gone", "unverified"
	other := finding()
	other.ID, other.Branch = "C", "main"
	whole := Entry{ID: "D", Branch: "feat", SHA: "abc", Round: 1, Depth: "quick", Disposition: "clean"}

	got := Latest([]Entry{a, b, other, aFixed, whole}, "feat", func(sha string) bool { return sha != "gone" })
	if len(got) != 2 {
		t.Fatalf("got %d findings, want A and B only: %+v", len(got), got)
	}
	if got[0].ID != "A" || got[0].Disposition != "fixed" || got[0].Open || got[0].Rewritten {
		t.Errorf("A: the latest line must win and be closed: %+v", got[0])
	}
	if got[1].ID != "B" || !got[1].Open || !got[1].Rewritten {
		t.Errorf("B: a finding from rewritten history must stay open and be marked: %+v", got[1])
	}
}

func TestAppendReadRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested", "ledger.jsonl")
	if entries, err := Read(file); err != nil || entries != nil {
		t.Fatalf("a missing ledger must read as empty: %v %v", entries, err)
	}
	e := finding()
	e.ID, e.Open, e.Rewritten = "A", true, true
	if err := Append(file, e); err != nil {
		t.Fatal(err)
	}
	entries, err := Read(file)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read %v %v", entries, err)
	}
	if entries[0].Open || entries[0].Rewritten || entries[0].Digest != nil || *entries[0].Path != "a.go" {
		t.Errorf("computed fields must not be stored and nulls must survive: %+v", entries[0])
	}
}
