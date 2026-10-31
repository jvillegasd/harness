package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// changeMapRepo is a repository on main with a base commit and one branch commit, plus an untracked and a deleted file.
func changeMapRepo(t *testing.T) string {
	t.Helper()
	dir := repo(t)
	write := func(name, body string) {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main\n")
	write("go.sum", "a\n")
	write("old.sql", "select 1;\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "branch", "-m", "main")
	git(t, dir, "switch", "-q", "-c", "feat")
	write("docs/agents/harness.json", `{"version": 1, "criticalPaths": ["secret/**"]}`)
	write("secret/key.go", "package secret\n")
	write("go.sum", "b\n")
	write("README.md", "hi\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "branch")
	write("main.go", "package main\n// edit\n")
	write("new_test.go", "package main\n")
	if err := os.Remove(filepath.Join(dir, "old.sql")); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestChangeMapText(t *testing.T) {
	dir := changeMapRepo(t)
	r := runBinary(t, dir, "change-map", "--base", "main")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	// Files are ordered by tier descending, then path; each line carries a reason, whose wording the package tests own.
	wantFiles := []string{
		"critical\told.sql",
		"critical\tsecret/key.go",
		"structural\tmain.go",
		"tests-docs\tREADME.md",
		"tests-docs\tdocs/agents/harness.json",
		"tests-docs\tnew_test.go",
		"mechanical\tgo.sum",
	}
	wantTotals := []string{"total\tcritical\t2", "total\tbehavioral\t0", "total\tstructural\t1", "total\ttests-docs\t3", "total\tmechanical\t1"}
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != len(wantFiles)+len(wantTotals) {
		t.Fatalf("got %d lines, want %d:\n%s", len(lines), len(wantFiles)+len(wantTotals), r.stdout)
	}
	for i, want := range wantFiles {
		cols := strings.Split(lines[i], "\t")
		if len(cols) != 3 || cols[0]+"\t"+cols[1] != want || cols[2] == "" {
			t.Errorf("line %d = %q, want tier and path %q with a reason", i, lines[i], want)
		}
	}
	for i, want := range wantTotals {
		if got := lines[len(wantFiles)+i]; got != want {
			t.Errorf("totals line %d = %q, want %q", i, got, want)
		}
	}
}

func TestChangeMapJSON(t *testing.T) {
	dir := changeMapRepo(t)
	r := runBinary(t, dir, "change-map", "--base", "main", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	var out struct {
		Base      string `json:"base"`
		MergeBase string `json:"mergeBase"`
		Files     []struct {
			Path   string `json:"path"`
			Tier   string `json:"tier"`
			Reason string `json:"reason"`
		} `json:"files"`
		Totals map[string]int `json:"totals"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, r.stdout)
	}
	if out.Base != "main" || out.MergeBase != git(t, dir, "merge-base", "HEAD", "main") {
		t.Errorf("base %q mergeBase %q", out.Base, out.MergeBase)
	}
	tiers := map[string]string{}
	for _, f := range out.Files {
		if f.Reason == "" {
			t.Errorf("%s has no reason", f.Path)
		}
		tiers[f.Path] = f.Tier
	}
	wantTiers := map[string]string{
		"old.sql": "critical", "secret/key.go": "critical", "go.sum": "mechanical",
		"README.md": "tests-docs", "docs/agents/harness.json": "tests-docs", "new_test.go": "tests-docs",
		"main.go": "structural",
	}
	for path, tier := range wantTiers {
		if tiers[path] != tier {
			t.Errorf("%s = %q, want %q", path, tiers[path], tier)
		}
	}
	if len(out.Files) != len(wantTiers) {
		t.Errorf("got %d files, want %d: %+v", len(out.Files), len(wantTiers), out.Files)
	}
	if out.Totals["critical"] != 2 || out.Totals["tests-docs"] != 3 || out.Totals["mechanical"] != 1 || out.Totals["structural"] != 1 || out.Totals["behavioral"] != 0 {
		t.Errorf("totals = %v", out.Totals)
	}
}

func TestChangeMapDefaultBaseFallsBackToMain(t *testing.T) {
	dir := changeMapRepo(t)
	r := runBinary(t, dir, "change-map", "--json")
	if r.code != 0 {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.Contains(r.stdout, `"base": "main"`) && !strings.Contains(r.stdout, `"base":"main"`) {
		t.Errorf("default base is not main: %s", r.stdout)
	}
}

func TestChangeMapOperationalErrorsExitTwo(t *testing.T) {
	dir := changeMapRepo(t)
	if r := runBinary(t, dir, "change-map", "--base", "no-such-ref"); r.code != 2 || !strings.Contains(r.stderr, "no-such-ref") {
		t.Errorf("unknown base exited %d with stderr %q", r.code, r.stderr)
	}
	if r := runBinary(t, t.TempDir(), "change-map"); r.code != 2 || !strings.Contains(r.stderr, "git repository") {
		t.Errorf("outside a repository exited %d with stderr %q", r.code, r.stderr)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs/agents/harness.json"), []byte(`{"criticalPaths": ["ok", "/abs"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runBinary(t, dir, "change-map", "--base", "main"); r.code != 2 || !strings.Contains(r.stderr, "/abs") || !strings.Contains(r.stderr, "criticalPaths") {
		t.Errorf("invalid criticalPaths exited %d with stderr %q", r.code, r.stderr)
	}
}

func TestChangeMapListedInHelp(t *testing.T) {
	if r := runBinary(t, t.TempDir(), "--help"); !strings.Contains(r.stdout, "  change-map ") {
		t.Errorf("help does not list change-map: %q", r.stdout)
	}
}
