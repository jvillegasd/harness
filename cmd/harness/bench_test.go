package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// benchDir returns a throwaway checkout whose cohorts decide the split, so the
// fixture's report does not depend on this machine's gitignored cohorts.json.
func benchDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "bench"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "POLICY.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/jvillegasd/harness\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cohorts, err := os.ReadFile(filepath.Join("testdata", "bench", "cohorts.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bench", "cohorts.json"), cohorts, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func benchHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.Abs(filepath.Join("testdata", "bench", "home"))
	if err != nil {
		t.Fatal(err)
	}
	return home
}

// sameReport compares line by line, ignoring the trailing pad the fixed-width
// sessions row leaves behind, which the repository's whitespace rule strips.
func sameReport(got, want string) bool {
	a := strings.Split(strings.TrimRight(got, "\n"), "\n")
	b := strings.Split(strings.TrimRight(want, "\n"), "\n")
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimRight(a[i], " \t") != strings.TrimRight(b[i], " \t") {
			return false
		}
	}
	return true
}

func readGolden(t *testing.T, name string) string {
	t.Helper()
	golden, err := os.ReadFile(filepath.Join("testdata", "bench", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(golden)
}

// TestBenchMatchesRecordedReport holds the port to the numbers the pre-port
// bench printed on the same fixture, recorded before it was deleted.
func TestBenchMatchesRecordedReport(t *testing.T) {
	r := ok(t, harness(t, benchDir(t), "bench", "--split", "2026-09-05", "--since", "2026-09-01", "--home", benchHome(t)), "bench")
	if want := readGolden(t, "report.golden"); !sameReport(r.stdout, want) {
		t.Fatalf("report differs from the recorded baseline:\n--- want\n%s\n--- got\n%s", want, r.stdout)
	}
}

func TestBenchRowsMatchRecordedRows(t *testing.T) {
	r := ok(t, harness(t, benchDir(t), "bench", "--split", "2026-09-05", "--since", "2026-09-01", "--home", benchHome(t), "--rows"), "bench --rows")
	if want := readGolden(t, "rows.golden"); r.stdout != want {
		t.Fatalf("rows differ from the recorded baseline:\n--- want\n%s\n--- got\n%s", want, r.stdout)
	}
}

// TestBenchRowsWithoutSplit pins the split date to the report: a row export
// needs no split, and adding one changes nothing.
func TestBenchRowsWithoutSplit(t *testing.T) {
	dir := benchDir(t)
	without := ok(t, harness(t, dir, "bench", "--rows", "--since", "2026-09-01", "--home", benchHome(t)), "bench --rows")
	with := ok(t, harness(t, dir, "bench", "--split", "2026-09-05", "--rows", "--since", "2026-09-01", "--home", benchHome(t)), "bench --split --rows")
	if without.stdout != with.stdout {
		t.Fatalf("adding --split changed the rows:\n--- without\n%s\n--- with\n%s", without.stdout, with.stdout)
	}
}

// TestBenchSinceFilteredMatchesRecorded compares a windowed run, before and
// after the change, byte for byte against the baseline recorded on the same
// fixture population.
func TestBenchSinceFilteredMatchesRecorded(t *testing.T) {
	dir := benchDir(t)
	report := ok(t, harness(t, dir, "bench", "--split", "2026-09-05", "--since", "2026-09-02", "--home", benchHome(t)), "bench --since report")
	if want := readGolden(t, "report-since.golden"); !sameReport(report.stdout, want) {
		t.Fatalf("windowed report differs from the recorded baseline:\n--- want\n%s\n--- got\n%s", want, report.stdout)
	}
	rows := ok(t, harness(t, dir, "bench", "--split", "2026-09-05", "--rows", "--since", "2026-09-02", "--home", benchHome(t)), "bench --since rows")
	if want := readGolden(t, "rows-since.golden"); rows.stdout != want {
		t.Fatalf("windowed rows differ from the recorded baseline:\n--- want\n%s\n--- got\n%s", want, rows.stdout)
	}
}

// TestBenchClientFlagNarrowsThePopulation is the command wiring for --client; that repeating a client collects it once is settled in internal/bench.
func TestBenchClientFlagNarrowsThePopulation(t *testing.T) {
	r := ok(t, harness(t, benchDir(t), "bench", "--split", "2026-09-05", "--since", "2026-09-01", "--client", "claude", "--rows", "--home", benchHome(t)), "bench one client")
	if !strings.Contains(r.stdout, `"client": "claude"`) {
		t.Fatalf("the row export carried no claude session: %q", r.stdout)
	}
	if strings.Contains(r.stdout, `"client": "opencode"`) {
		t.Fatalf("--client claude still exported another client: %q", r.stdout)
	}
}

// TestBenchReadsCodex is the command wiring for the Codex reader; its counting
// rules are settled in internal/bench.
func TestBenchReadsCodex(t *testing.T) {
	home := t.TempDir()
	rollout := filepath.Join(home, ".codex", "sessions", "2026", "09", "03", "rollout-2026-09-03T10-00-00-a.jsonl")
	if err := os.MkdirAll(filepath.Dir(rollout), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"timestamp":"2026-09-03T10:00:00.000Z","type":"session_meta","payload":{"id":"a","cwd":"/tmp/acme/app","source":"cli"}}` + "\n"
	if err := os.WriteFile(rollout, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ok(t, harness(t, benchDir(t), "bench", "--rows", "--client", "codex", "--home", home), "bench codex")
	if !strings.Contains(r.stdout, `"client": "codex"`) {
		t.Fatalf("the row export carried no codex session: %q", r.stdout)
	}
}

func TestBenchEmptyHomeReportsNoSessions(t *testing.T) {
	r := ok(t, harness(t, benchDir(t), "bench", "--split", "2026-01-01", "--since", "2026-01-01", "--home", t.TempDir()), "bench on an empty home")
	if !strings.Contains(r.stdout, "no sessions found") {
		t.Fatalf("bench on an empty home printed %q", r.stdout)
	}
}

func TestBenchReportsUnreadableTranscript(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, ".claude", "projects", "-tmp-x")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory named like a transcript stands in for an unreadable file.
	if err := os.Mkdir(filepath.Join(project, "broken.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := harness(t, benchDir(t), "bench", "--split", "2026-09-05", "--home", home)
	if r.code == 0 || r.stderr == "" {
		t.Fatalf("an unreadable transcript exited %d with stderr %q, want a reported failure", r.code, r.stderr)
	}
}

func TestBenchRefusesOutsideCheckout(t *testing.T) {
	r := harness(t, t.TempDir(), "bench", "--split", "2026-09-05", "--home", benchHome(t))
	if r.code == 0 || r.stderr == "" {
		t.Fatalf("outside a checkout exited %d with stderr %q, want a reported failure", r.code, r.stderr)
	}
}

func TestBenchUsageErrors(t *testing.T) {
	dir := benchDir(t)
	cases := [][]string{
		{"bench"},
		{"bench", "--home", benchHome(t)},
		{"bench", "--split", "not-a-date", "--home", benchHome(t)},
		{"bench", "--split", "2026-09-05", "--client", "cursor", "--home", benchHome(t)},
		{"bench", "--split", "2026-09-05", "--bogus"},
	}
	for _, args := range cases {
		r := harness(t, dir, args...)
		if r.code != 2 {
			t.Errorf("%v exited %d, want a usage error", args, r.code)
		}
		// A report with no --split has to name the flag it is missing.
		if !slices.Contains(args, "--split") && !strings.Contains(r.stderr, "--split") {
			t.Errorf("%v did not name --split: %q", args, r.stderr)
		}
	}
}
