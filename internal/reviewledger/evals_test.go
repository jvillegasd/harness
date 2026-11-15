package reviewledger

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func bugCase(id, path, lines string) Case {
	return Case{ID: id, Kind: "bug", Expected: []Expected{{Path: path, Lines: lines}}, ConfirmedBy: "human"}
}

// TestScore covers the scoring rules: a confirmed bug or risk overlapping an expected range catches a bug case, one on a clean case is a false positive, a dropped one inside an expected range is a dropped catch, and a nit or a miss counts for nothing.
func TestScore(t *testing.T) {
	cases := []Case{bugCase("b1", "a.go", "40-52"), bugCase("b2", "b.go", "10"), {ID: "c1", Kind: "clean", ConfirmedBy: "human"}}
	s, err := Score(cases, []RunFinding{
		{Case: "b1", Path: "a.go", Lines: "50-60", Severity: "risk", Disposition: "confirmed"}, // overlaps: caught
		{Case: "b2", Path: "b.go", Lines: "11", Severity: "bug", Disposition: "confirmed"},     // misses by a line
		{Case: "b2", Path: "b.go", Lines: "10", Severity: "nit", Disposition: "confirmed"},     // a nit catches nothing
		{Case: "b2", Path: "b.go", Lines: "9-10", Severity: "bug", Disposition: "dropped"},     // a dropped catch
		{Case: "c1", Path: "x.go", Lines: "1", Severity: "bug", Disposition: "confirmed"},      // a false positive
		{Case: "c1", Path: "x.go", Lines: "2", Severity: "q", Disposition: "confirmed"},        // a question is not one
	})
	if err != nil {
		t.Fatal(err)
	}
	want := Summary{Summary: true, Cases: []string{"b1", "b2", "c1"}, BugCases: 2, Caught: 1, Recall: 0.5, FalsePositives: 1, Dropped: 1, DroppedCatches: 1}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("got %+v\nwant %+v", s, want)
	}
	if _, err := Score(cases, []RunFinding{{Case: "nope", Path: "a.go", Severity: "bug", Disposition: "confirmed"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a finding on an unknown case returned %v", err)
	}
}

func writeLines(t *testing.T, path string, values ...any) {
	t.Helper()
	var b strings.Builder
	for _, v := range values {
		line, _ := json.Marshal(v)
		b.Write(append(line, '\n'))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScoreRun covers a run's scoring: only confirmed cases count, the summary is appended once, and it is compared with the last run on the same cases.
func TestScoreRun(t *testing.T) {
	dir := t.TempDir()
	casesFile := filepath.Join(dir, "evals.jsonl")
	unconfirmed := bugCase("u1", "z.go", "1")
	unconfirmed.ConfirmedBy = ""
	writeLines(t, casesFile, bugCase("b1", "a.go", "1-5"), unconfirmed)
	runs := filepath.Join(dir, "runs")
	hit := RunFinding{Case: "b1", Path: "a.go", Lines: "3", Severity: "bug", Disposition: "confirmed"}

	writeLines(t, filepath.Join(runs, "2026-11-01.jsonl"), hit)
	first, err := ScoreRun(casesFile, filepath.Join(runs, "2026-11-01.jsonl"), Summary{Date: "2026-11-01"})
	if err != nil || first.Previous != nil || first.Regression || first.Summary.Recall != 1 || !reflect.DeepEqual(first.Summary.Cases, []string{"b1"}) {
		t.Fatalf("first run got %+v, %v", first, err)
	}
	if _, err := ScoreRun(casesFile, filepath.Join(runs, "2026-11-01.jsonl"), Summary{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("rescoring a run returned %v", err)
	}

	// A later run that misses the case is a regression against the first.
	writeLines(t, filepath.Join(runs, "2026-11-02.jsonl"), RunFinding{Case: "b1", Path: "a.go", Lines: "9", Severity: "bug", Disposition: "confirmed"})
	second, err := ScoreRun(casesFile, filepath.Join(runs, "2026-11-02.jsonl"), Summary{Date: "2026-11-02", Depth: "high", Models: map[string]string{"review-bugs": "opus"}})
	if err != nil || second.Previous == nil || second.Previous.Date != "2026-11-01" || !second.Regression || second.Summary.Recall != 0 {
		t.Fatalf("second run got %+v, %v", second, err)
	}
	_, summaries, err := readRun(filepath.Join(runs, "2026-11-02.jsonl"))
	if err != nil || len(summaries) != 1 || summaries[0].Depth != "high" || summaries[0].Models["review-bugs"] != "opus" {
		t.Errorf("appended summaries %+v, %v", summaries, err)
	}
}

func (r *coverRepo) commitMsg(msg string, files map[string]string) string {
	r.t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte(body), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

// TestSeed covers seeding: each fix's changed lines are traced to the commit that introduced them, at that commit's line numbers; a pure insertion is placed by its neighbors; a line from the root commit makes no case; other commits are not read.
func TestSeed(t *testing.T) {
	r := newCoverRepo(t)
	c := func(lines ...string) string { return strings.Join(lines, "\n") + "\n" }
	intro := r.commitMsg("feat: add c", map[string]string{"c.go": c("c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8", "c9", "c10")})
	fix1 := r.commitMsg("fix: c4 and c5", map[string]string{"c.go": c("c1", "c2", "c3", "C4", "C5", "c6", "c7", "c8", "c9", "c10")})
	r.commitMsg("fix(b): root line", map[string]string{"b.go": "B\n"})
	r.commitMsg("refactor: not a fix", map[string]string{"c.go": c("c1", "c2", "c3", "C4", "C5", "c6", "c7", "c8", "c9", "c10x")})
	fix3 := r.commitMsg("fix: guard after c7", map[string]string{"c.go": c("c1", "c2", "c3", "C4", "C5", "c6", "c7", "guard", "c8", "c9", "c10x")})

	cases, err := Seed(r.dir, "HEAD", 20)
	if err != nil {
		t.Fatal(err)
	}
	base := r.git("rev-parse", intro+"^")
	want := []Case{
		{ID: "seed-" + intro[:12] + "-" + fix3[:12], Kind: "bug", Base: base, Head: intro, Expected: []Expected{{Path: "c.go", Lines: "7-8"}}, Source: fix3},
		{ID: "seed-" + intro[:12] + "-" + fix1[:12], Kind: "bug", Base: base, Head: intro, Expected: []Expected{{Path: "c.go", Lines: "4-5"}}, Source: fix1},
	}
	if !reflect.DeepEqual(cases, want) {
		t.Fatalf("got %+v\nwant %+v", cases, want)
	}
	if few, err := Seed(r.dir, "HEAD", 1); err != nil || len(few) != 1 || few[0].Source != fix3 {
		t.Errorf("--max 1 got %+v, %v", few, err)
	}
}
