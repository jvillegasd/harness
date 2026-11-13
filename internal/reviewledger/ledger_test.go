package reviewledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func str(s string) *string { return &s }

func finding() Entry {
	return Entry{Branch: "feat", SHA: "abc", Depth: "high", Path: str("a.go"), Lines: "40-52", Severity: "bug", Summary: "s", Disposition: "confirmed"}
}

func TestValidate(t *testing.T) {
	for name, e := range map[string]Entry{
		"a confirmed finding":    finding(),
		"a whole-review line":    {Branch: "feat", SHA: "abc", Depth: "none", Disposition: "no-review"},
		"a fixed finding":        func() Entry { e := finding(); e.Disposition, e.Evidence, e.Commit = "fixed", "test", "def"; return e }(),
		"a nit kept as a nit":    func() Entry { e := finding(); e.Severity, e.Disposition = "nit", "nit"; return e }(),
		"an old bug, as a nit":   func() Entry { e := finding(); e.Severity, e.Disposition = "pre-existing", "nit"; return e }(),
		"a human's call, quoted": func() Entry { e := finding(); e.Disposition, e.Evidence = "accepted", "\"leave it\""; return e }(),
		"a dropped with its run": func() Entry { e := finding(); e.Disposition, e.Evidence = "dropped", "ran green"; return e }(),
	} {
		if err := Validate(e); err != nil {
			t.Errorf("%s was rejected: %v", name, err)
		}
	}
	for name, mutate := range map[string]func(*Entry){
		"unknown disposition":     func(e *Entry) { e.Disposition = "dismissed" },
		"unknown depth":           func(e *Entry) { e.Depth = "huge" },
		"no branch":               func(e *Entry) { e.Branch = "" },
		"no path":                 func(e *Entry) { e.Path = nil },
		"bad severity":            func(e *Entry) { e.Severity = "major" },
		"bad lines":               func(e *Entry) { e.Lines = "40-x" },
		"no summary":              func(e *Entry) { e.Summary = "" },
		"whole with a path":       func(e *Entry) { e.Disposition = "clean" },
		"fixed without commit":    func(e *Entry) { e.Disposition, e.Evidence = "fixed", "test" },
		"dropped without its run": func(e *Entry) { e.Disposition = "dropped" },
		"human call unquoted":     func(e *Entry) { e.Disposition = "accepted" },
		"bug relabelled nit":      func(e *Entry) { e.Disposition = "nit" },
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
	first.ID, second.ID, second.Disposition = "A", "A", "dropped"
	other := finding()
	other.ID = "B"
	got, ok := Find([]Entry{first, second, other}, "A")
	if !ok || got.Disposition != "dropped" {
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
	b.ID, b.SHA = "B", "gone"
	c := finding()
	c.ID, c.Disposition, c.Evidence, c.Commit = "C", "fixed", "t", "elsewhere"
	other := finding()
	other.ID, other.Branch = "D", "main"
	whole := Entry{ID: "E", Branch: "feat", SHA: "abc", Depth: "medium", Disposition: "clean"}

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
	if err := Append(file, Entry{ID: "A", Branch: "feat", Depth: "none", Disposition: "clean"}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString("\n  \n")
	f.Close()
	if err := Append(file, Entry{ID: "B", Path: str("a.go"), Depth: "high", Disposition: "confirmed"}); err != nil {
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

// branchDir writes a branch directory holding a ledger and a saved patch, as a review leaves it.
func branchDir(t *testing.T, branches, name string) string {
	t.Helper()
	dir := filepath.Join(branches, "branches", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"ledger.jsonl", "review.patch"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "ledger.jsonl")
}

func TestRemoveBranchDeletesOnlyItsDirectory(t *testing.T) {
	branches := t.TempDir()
	file := branchDir(t, branches, "feat%2Fexport")
	sibling := branchDir(t, branches, "feat%2Fexport-v2")
	removed, err := RemoveBranch(file)
	if err != nil || !removed {
		t.Fatalf("RemoveBranch = %v, %v; want removed", removed, err)
	}
	if _, err := os.Stat(filepath.Dir(file)); !os.IsNotExist(err) {
		t.Fatalf("the branch directory survived: %v", err)
	}
	if _, err := os.Stat(sibling); err != nil {
		t.Fatalf("a sibling branch's ledger was touched: %v", err)
	}
}

func TestRemoveBranchWithNoDirectoryIsDone(t *testing.T) {
	removed, err := RemoveBranch(filepath.Join(t.TempDir(), "branches", "never-reviewed", "ledger.jsonl"))
	if err != nil || removed {
		t.Fatalf("RemoveBranch = %v, %v; want nothing removed and no error", removed, err)
	}
}

// worktree.txt is review's record of an executor worktree it could not retire; deleting it would orphan that worktree and its saved patch.
func TestRemoveBranchRefusesARecordedWorktree(t *testing.T) {
	file := branchDir(t, t.TempDir(), "feat")
	dir := filepath.Dir(file)
	if err := os.WriteFile(filepath.Join(dir, "worktree.txt"), []byte("/tmp/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, err := RemoveBranch(file)
	if !errors.Is(err, ErrHeld) || removed {
		t.Fatalf("RemoveBranch = %v, %v; want ErrHeld and nothing removed", removed, err)
	}
	for _, f := range []string{"ledger.jsonl", "review.patch", "worktree.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s was deleted by a refused removal: %v", f, err)
		}
	}
}

// A dot segment is the one name path-escaping leaves alone, and it would point the ledger, and a removal, at a parent directory.
func TestFileRejectsDotSegments(t *testing.T) {
	for _, branch := range []string{".", ".."} {
		if _, err := File(t.TempDir(), branch); !errors.Is(err, ErrInvalid) {
			t.Errorf("File(%q) = %v, want ErrInvalid", branch, err)
		}
	}
}

func TestRemoveBranchRefusesOutsideTheBranchesDirectory(t *testing.T) {
	root := t.TempDir()
	file := branchDir(t, root, "feat")
	// The path a ".." branch would have resolved to: the directory holding every branch.
	removed, err := RemoveBranch(filepath.Join(root, "branches", "ledger.jsonl"))
	if !errors.Is(err, ErrInvalid) || removed {
		t.Fatalf("RemoveBranch = %v, %v; want ErrInvalid and nothing removed", removed, err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("a branch's ledger was deleted: %v", err)
	}
}

// Prune keeps every branch that still exists and every directory that records a review worktree, and a dry run deletes nothing.
func TestPruneRemovesOnlyGoneBranches(t *testing.T) {
	root := t.TempDir()
	live := filepath.Dir(branchDir(t, root, "feat%2Flive"))
	gone := filepath.Dir(branchDir(t, root, "feat%2Fgone"))
	held := filepath.Dir(branchDir(t, root, "held"))
	if err := os.WriteFile(filepath.Join(held, "worktree.txt"), []byte("/tmp/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	isLive := func(b string) bool { return b == "feat/live" }
	dir := filepath.Join(root, "branches")

	removed, kept, err := Prune(dir, isLive, true)
	if err != nil || len(removed) != 1 || removed[0] != gone || len(kept) != 1 || kept[0] != held {
		t.Fatalf("dry run = %v, %v, %v; want [%s], [%s]", removed, kept, err, gone, held)
	}
	if _, err := os.Stat(gone); err != nil {
		t.Fatalf("a dry run deleted %s: %v", gone, err)
	}

	if removed, _, err := Prune(dir, isLive, false); err != nil || len(removed) != 1 {
		t.Fatalf("Prune = %v, %v; want one directory removed", removed, err)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("a gone branch's directory survived: %v", err)
	}
	for _, d := range []string{live, held} {
		if _, err := os.Stat(filepath.Join(d, "ledger.jsonl")); err != nil {
			t.Errorf("%s was touched: %v", d, err)
		}
	}
	if removed, held, err := Prune(filepath.Join(root, "missing"), isLive, false); err != nil || removed != nil || held != nil {
		t.Errorf("Prune with no branches directory = %v, %v, %v; want nothing", removed, held, err)
	}
}
