package reviewledger

import (
	"errors"
	"os"
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
		"/Users/me/work/app/.git":                  "Users_me_work_app",
		"/Users/me/b@c/app":                        "Users_me_b@c_app",
	} {
		if got := Key(source); got != want {
			t.Errorf("Key(%q) = %q, want %q", source, got, want)
		}
	}
	// Distinct sources that differ only in where a separator or underscore falls never share a ledger.
	for _, pair := range [][2]string{
		{"git@github.com:a-b/c.git", "git@github.com:a/b-c.git"},
		{"gitlab.com/a_b/c", "gitlab.com/a/b_c"},
		{"/x/a_b/c", "/x/a/b_c"},
	} {
		if Key(pair[0]) == Key(pair[1]) {
			t.Errorf("%s and %s share the key %q", pair[0], pair[1], Key(pair[0]))
		}
	}
}

func str(s string) *string { return &s }

func finding() Entry {
	return Entry{Branch: "feat", SHA: "abc", Round: 1, Depth: "standard", Path: str("a.go"), Lines: "40-52", Severity: "bug", Summary: "s", Disposition: "confirmed"}
}

func TestValidate(t *testing.T) {
	for name, e := range map[string]Entry{
		"a confirmed finding":   finding(),
		"a whole-review line":   {Branch: "feat", SHA: "abc", Round: 1, Depth: "none", Disposition: "no-review"},
		"a fixed finding":       func() Entry { e := finding(); e.Disposition, e.Evidence, e.Commit = "fixed", "test", "def"; return e }(),
		"a nit kept as a nit":   func() Entry { e := finding(); e.Severity, e.Disposition = "nit", "nit"; return e }(),
		"an old bug, recorded":  func() Entry { e := finding(); e.Severity, e.Disposition = "pre-existing", "pre-existing"; return e }(),
		"a refuted with reason": func() Entry { e := finding(); e.Disposition, e.Evidence = "refuted", "cut at b.go:5"; return e }(),
	} {
		if err := Validate(e); err != nil {
			t.Errorf("%s was rejected: %v", name, err)
		}
	}
	for name, mutate := range map[string]func(*Entry){
		"unknown disposition":    func(e *Entry) { e.Disposition = "dismissed" },
		"unknown depth":          func(e *Entry) { e.Depth = "huge" },
		"third round":            func(e *Entry) { e.Round = 3 },
		"no branch":              func(e *Entry) { e.Branch = "" },
		"no path":                func(e *Entry) { e.Path = nil },
		"bad severity":           func(e *Entry) { e.Severity = "major" },
		"bad lines":              func(e *Entry) { e.Lines = "40-x" },
		"no summary":             func(e *Entry) { e.Summary = "" },
		"whole with a path":      func(e *Entry) { e.Disposition = "clean" },
		"fixed without commit":   func(e *Entry) { e.Disposition, e.Evidence = "fixed", "test" },
		"refuted without reason": func(e *Entry) { e.Disposition = "refuted" },
		"human call unquoted":    func(e *Entry) { e.Disposition = "wont-matter" },
		"bug relabelled nit":     func(e *Entry) { e.Disposition = "nit" },
		"bug relabelled old":     func(e *Entry) { e.Disposition = "pre-existing" },
	} {
		e := finding()
		mutate(&e)
		if err := Validate(e); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}

func TestFindReturnsTheLatestLine(t *testing.T) {
	first, second := finding(), finding()
	first.ID, second.ID, second.Disposition = "A", "A", "unverified"
	other := finding()
	other.ID = "B"
	got, ok := Find([]Entry{first, second, other}, "A")
	if !ok || got.Disposition != "unverified" {
		t.Fatalf("Find returned %+v, %v; want the second line for A", got, ok)
	}
	if _, ok := Find([]Entry{first}, "missing"); ok {
		t.Error("Find found a missing id")
	}
}

func TestLatestState(t *testing.T) {
	inHead := func(sha string) bool { return sha == "abc" || sha == "landed" }
	a := finding()
	a.ID = "A"
	aFixed := a
	aFixed.Disposition, aFixed.Evidence, aFixed.Commit = "fixed", "t", "landed"
	b := finding()
	b.ID, b.SHA, b.Disposition = "B", "gone", "unverified"
	c := finding()
	c.ID, c.Disposition, c.Evidence, c.Commit = "C", "fixed", "t", "elsewhere"
	other := finding()
	other.ID, other.Branch = "D", "main"
	whole := Entry{ID: "E", Branch: "feat", SHA: "abc", Round: 1, Depth: "quick", Disposition: "clean"}

	got := Latest([]Entry{a, b, c, other, aFixed, whole}, "feat", inHead)
	want := []struct {
		id              string
		open, rewritten bool
	}{
		{"A", false, false}, // the latest line wins, and its fix is in the head
		{"B", true, true},   // rewritten history keeps an open finding open
		{"C", true, false},  // a fix whose commit the head lacks still holds
		{"E", false, false}, // a whole-review line is listed so a clean review is visible
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].ID != w.id || got[i].Open != w.open || got[i].Rewritten != w.rewritten {
			t.Errorf("line %d = %s open=%v rewritten=%v, want %+v", i, got[i].ID, got[i].Open, got[i].Rewritten, w)
		}
	}
}

func TestAppendReadRoundTrip(t *testing.T) {
	file := filepath.Join(t.TempDir(), "nested", "ledger.jsonl")
	if entries, err := Read(file); err != nil || entries != nil {
		t.Fatalf("a missing ledger must read as empty: %v %v", entries, err)
	}
	if err := Append(file, Entry{ID: "A", Branch: "feat"}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n  \n")
	f.Close()
	if err := Append(file, Entry{ID: "B", Path: str("a.go")}); err != nil {
		t.Fatal(err)
	}
	entries, err := Read(file)
	if err != nil || len(entries) != 2 {
		t.Fatalf("blank lines must be skipped: %v %v", entries, err)
	}
	if entries[0].Path != nil || entries[0].Digest != nil || *entries[1].Path != "a.go" {
		t.Errorf("nulls and values must survive: %+v", entries)
	}
}
