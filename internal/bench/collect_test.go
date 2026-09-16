package bench

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// synthHome writes a Claude session store with one transcript per day starting
// 2026-01-01, so a since date selects a tail of the population. It backs both
// the tests here and the measured collection benchmark.
func synthHome(tb testing.TB, sessions, records int) string {
	tb.Helper()
	dir := filepath.Join(tb.TempDir(), ".claude", "projects", "-tmp-acme-app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tb.Fatal(err)
	}
	text := strings.Repeat("x", 160)
	for s := 0; s < sessions; s++ {
		stamp := time.Date(2026, 1, 1+s, 9, 0, 0, 0, time.UTC).Format(time.RFC3339)
		var b strings.Builder
		for r := 0; r < records; r++ {
			fmt.Fprintf(&b, `{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"text","text":%q}]}}`+"\n", stamp, text)
		}
		name := filepath.Join(dir, fmt.Sprintf("session-%04d.jsonl", s))
		if err := os.WriteFile(name, []byte(b.String()), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(dir)))
}

func writeTranscript(tb testing.TB, path, body string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tb.Fatal(err)
	}
}

func TestCollectDeduplicatesClients(t *testing.T) {
	home := synthHome(t, 3, 4)
	once, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := collect(home, []string{"claude", "claude", "claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(once) != 3 || len(repeated) != len(once) {
		t.Fatalf("one client gave %d sessions, repeated gave %d", len(once), len(repeated))
	}
}

func TestCollectExcludesBeforeAggregation(t *testing.T) {
	home := synthHome(t, 10, 4)
	since := time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	got, err := collect(home, []string{"claude"}, nil, &since)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("collected %d sessions, want the 3 on or after %s", len(got), since.Format("2006-01-02"))
	}
	for _, s := range got {
		if s.date.Before(since) {
			t.Fatalf("collected session dated %s, before %s", s.date.Format("2006-01-02"), since.Format("2006-01-02"))
		}
	}
}

func TestCollectSkipsExcludedParentSidecar(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".claude", "projects", "-tmp-acme-app")
	writeTranscript(t, filepath.Join(dir, "session-old.jsonl"),
		`{"type":"user","timestamp":"2026-01-01T09:00:00Z","message":{"content":"hi"}}`+"\n")
	// A sidecar that cannot be read stands in for a real one: if it is opened,
	// collection fails.
	if err := os.MkdirAll(filepath.Join(dir, "session-old", "subagents", "agent1.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}
	since := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	got, err := collect(home, []string{"claude"}, nil, &since)
	if err != nil {
		t.Fatalf("an excluded parent's sidecar was opened: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("collected %d sessions, want the excluded parent dropped", len(got))
	}
	if _, err := collect(home, []string{"claude"}, nil, nil); err == nil {
		t.Fatal("expected the sidecar read to fail once the parent is included")
	}
}

func TestStreamHandlesLongRecord(t *testing.T) {
	home := t.TempDir()
	long := strings.Repeat("y", 1<<20)
	writeTranscript(t, filepath.Join(home, ".claude", "projects", "-tmp-acme-app", "long.jsonl"),
		fmt.Sprintf(`{"type":"assistant","timestamp":"2026-01-01T09:00:00Z","message":{"content":[{"type":"text","text":%q},{"type":"tool_use"}]}}`, long)+"\n")
	got, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("collected %d sessions, want 1", len(got))
	}
	if v, ok := got[0].metric("tool_calls"); !ok || v != 1 {
		t.Fatalf("tool_calls = %v, want 1", v)
	}
}

func TestStreamSkipsDamagedLines(t *testing.T) {
	home := t.TempDir()
	body := strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-01T09:00:00Z","message":{"content":"hi"}}`,
		"not json at all",
		`{"type":"user","timestamp":"2026-01-01T09:00:01Z","message":{"content":`,
		`[1, 2, 3]`,
		`{"type":"assistant","timestamp":"2026-01-01T09:00:05Z","message":{"content":[{"type":"tool_use"}]}}`,
	}, "\n") + "\n"
	writeTranscript(t, filepath.Join(home, ".claude", "projects", "-tmp-acme-app", "damaged.jsonl"), body)
	got, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("collected %d sessions, want 1", len(got))
	}
	for metric, want := range map[string]float64{"turns": 1, "tool_calls": 1} {
		if v, ok := got[0].metric(metric); !ok || v != want {
			t.Fatalf("%s = %v, want %v", metric, v, want)
		}
	}
}

func TestReportCallableDirectly(t *testing.T) {
	home := synthHome(t, 3, 4)
	split := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	out, err := Report(Options{Home: home, Clients: []string{"claude"}, Work: []string{"acme"}, Split: &split}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "split 2026-01-03\n") {
		t.Fatalf("report does not start with its header:\n%s", out)
	}
	if !strings.Contains(out, "claude  work") {
		t.Fatalf("report did not split the configured cohort:\n%s", out)
	}
	rows, err := Report(Options{Home: home, Clients: []string{"claude"}, Work: []string{"acme"}, Rows: true}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(rows, "\n"); n != 3 {
		t.Fatalf("row export wrote %d lines, want 3", n)
	}
	if _, err := Report(Options{Home: home}, io.Discard); err == nil {
		t.Fatal("a report without a split date was accepted")
	}
}
