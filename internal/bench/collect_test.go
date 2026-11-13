package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// TestCollectSinceIsDateGranular pins the window to the date, so a consumer
// that passes a Since with a time of day still selects whole sessions.
func TestCollectSinceIsDateGranular(t *testing.T) {
	home := synthHome(t, 10, 4)
	noon := time.Date(2026, 1, 8, 12, 0, 0, 0, time.UTC)
	got, err := collect(home, []string{"claude"}, nil, &noon)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("collected %d sessions, want the 3 dated on or after %s", len(got), noon.Format("2006-01-02"))
	}
}

func TestCollectRejectsUnknownClient(t *testing.T) {
	if _, err := collect(t.TempDir(), []string{"cursor"}, nil, nil); err == nil {
		t.Fatal("an unknown client was accepted")
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
	out, err := Report(Options{Home: home, Clients: []string{"claude"}, Work: []string{"acme"}, Split: &split})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "split 2026-01-03\n") {
		t.Fatalf("report does not start with its header:\n%s", out)
	}
	if !strings.Contains(out, "claude  work") {
		t.Fatalf("report did not split the configured cohort:\n%s", out)
	}
	noon := time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)
	noonOut, err := Report(Options{Home: home, Clients: []string{"claude"}, Work: []string{"acme"}, Split: &noon})
	if err != nil {
		t.Fatal(err)
	}
	if noonOut != out {
		t.Fatalf("a split with a time of day changed the report:\n--- midnight\n%s\n--- noon\n%s", out, noonOut)
	}
	rows, err := Report(Options{Home: home, Clients: []string{"claude"}, Work: []string{"acme"}, Rows: true})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(rows, "\n"); n != 3 {
		t.Fatalf("row export wrote %d lines, want 3", n)
	}
	if _, err := Report(Options{Home: home}); err == nil {
		t.Fatal("a report without a split date was accepted")
	}
}

// codexLine is one rollout record in Codex's wire shape.
func codexLine(stamp, kind, payload string) string {
	return fmt.Sprintf(`{"timestamp":"2026-03-01T%s.000Z","type":%q,"payload":%s}`, stamp, kind, payload) + "\n"
}

func TestCodexRolloutMetrics(t *testing.T) {
	home := t.TempDir()
	day := filepath.Join(home, ".codex", "sessions", "2026", "03", "01")
	writeTranscript(t, filepath.Join(day, "rollout-2026-03-01T09-00-00-parent.jsonl"),
		codexLine("09:00:00", "session_meta", `{"id":"parent","cwd":"/tmp/acme/app","source":"cli"}`)+
			codexLine("09:00:00", "response_item", `{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>"}]}`)+
			codexLine("09:00:01", "event_msg", `{"type":"user_message","message":"fix it"}`)+
			codexLine("09:00:02", "response_item", `{"type":"function_call","name":"exec_command","arguments":"{}"}`)+
			codexLine("09:00:03", "response_item", `{"type":"function_call_output","output":"exec command rejected by user"}`)+
			codexLine("09:00:04", "response_item", `{"type":"custom_tool_call","name":"apply_patch","input":""}`)+
			codexLine("09:00:05", "response_item", `{"type":"custom_tool_call_output","output":"grep: rejected by user"}`)+
			codexLine("09:00:06", "event_msg", `{"type":"turn_aborted","reason":"interrupted"}`)+
			codexLine("09:00:07", "event_msg", `{"type":"turn_aborted","reason":"replaced"}`)+
			codexLine("09:03:00", "event_msg", `{"type":"user_message","message":"again"}`))
	// A spawned subagent, its own subagent, and a guardian review are not sessions.
	writeTranscript(t, filepath.Join(day, "rollout-2026-03-01T09-01-00-child.jsonl"),
		codexLine("09:01:00", "session_meta", `{"id":"child","cwd":"/tmp/acme/app","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent","depth":1}}}}`)+
			codexLine("09:01:00", "event_msg", `{"type":"user_message","message":"task"}`)+
			codexLine("09:01:01", "response_item", `{"type":"message","role":"developer","content":[]}`)+
			codexLine("09:01:01", "response_item", `{"type":"message","role":"user","content":[]}`)+
			codexLine("09:01:02", "response_item", `{"type":"reasoning","summary":[]}`)+
			codexLine("09:01:02", "response_item", `{"type":"function_call","name":"exec_command"}`)+
			codexLine("09:01:03", "response_item", `{"type":"function_call_output","output":"ok"}`)+
			codexLine("09:01:04", "response_item", `{"type":"message","role":"assistant","content":[]}`))
	writeTranscript(t, filepath.Join(day, "rollout-2026-03-01T09-02-00-grandchild.jsonl"),
		codexLine("09:02:00", "session_meta", `{"id":"grandchild","parent_thread_id":"child","cwd":"/tmp/acme/app","source":{"subagent":{"thread_spawn":{"parent_thread_id":"child","depth":2}}}}`)+
			codexLine("09:02:01", "response_item", `{"type":"message","role":"assistant","content":[]}`))
	writeTranscript(t, filepath.Join(day, "rollout-2026-03-01T09-02-30-guardian.jsonl"),
		codexLine("09:02:30", "session_meta", `{"id":"guardian","cwd":"/tmp/acme/app","source":{"internal":"guardian"}}`)+
			codexLine("09:02:31", "event_msg", `{"type":"user_message","message":"review"}`))
	// An archived session is still a session, and a fork's copied parent
	// metadata does not replace its own.
	writeTranscript(t, filepath.Join(home, ".codex", "archived_sessions", "rollout-2026-03-01T10-00-00-fork.jsonl"),
		codexLine("10:00:00", "session_meta", `{"id":"fork","forked_from_id":"parent","cwd":"/tmp/home/lab","source":"cli"}`)+
			codexLine("10:00:00", "session_meta", `{"id":"parent","cwd":"/tmp/acme/app","source":"cli"}`))

	got, err := collect(home, []string{"codex"}, []string{"acme"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("collected %d sessions, want the parent and the archived fork", len(got))
	}
	parent, fork := got[1], got[0]
	if *parent.project != "/tmp/acme/app" || parent.cohort != "work" || *fork.project != "/tmp/home/lab" {
		t.Fatalf("projects = %q (%s), %q", *parent.project, parent.cohort, *fork.project)
	}
	for metric, want := range map[string]float64{
		"turns": 2, "tool_calls": 2, "minutes": 3, "delegation": 5, "interrupts": 1, "denials": 1,
	} {
		if v, ok := parent.metric(metric); !ok || v != want {
			t.Errorf("%s = %v, want %v", metric, v, want)
		}
	}
	for _, metric := range []string{"commands", "errors"} {
		if _, ok := parent.metric(metric); ok {
			t.Errorf("%s is recorded, want absent", metric)
		}
	}
}

func TestCodexSinceSkipsOldRollout(t *testing.T) {
	home := t.TempDir()
	writeTranscript(t, filepath.Join(home, ".codex", "sessions", "2026", "03", "01", "rollout-2026-03-01T09-00-00-old.jsonl"),
		codexLine("09:00:00", "session_meta", `{"id":"old","cwd":"/tmp/x","source":"cli"}`))
	since := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	got, err := collect(home, []string{"codex"}, nil, &since)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("collected %d sessions, want the old rollout dropped", len(got))
	}
}

// A compaction writes its summary into the transcript as a user entry that is
// neither meta nor prefixed, so only its own flag tells it from a typed prompt.
func TestCompactionSummaryIsNotATurn(t *testing.T) {
	home := t.TempDir()
	body := strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-01T09:00:00Z","message":{"content":"fix the login test"}}`,
		`{"type":"user","timestamp":"2026-01-01T09:30:00Z","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"message":{"content":"This session is being continued from a previous conversation."}}`,
		`{"type":"user","timestamp":"2026-01-01T09:31:00Z","message":{"content":[{"type":"text","text":"keep going"}]}}`,
	}, "\n") + "\n"
	writeTranscript(t, filepath.Join(home, ".claude", "projects", "-tmp-acme-app", "compacted.jsonl"), body)
	got, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("collected %d sessions, want 1", len(got))
	}
	if v, ok := got[0].metric("turns"); !ok || v != 2 {
		t.Fatalf("turns = %v, want the 2 typed prompts and not the compaction summary", v)
	}
}

// `/work fix the flaky test` carries the request in the command's arguments,
// so it is a command and a turn, in both the message-first form a skill writes
// and the name-first form a built-in writes. A bare command stays no turn:
// counted otherwise, moving a request behind `/work` would lower turns by itself.
func TestSlashCommandWithArgumentsIsAlsoATurn(t *testing.T) {
	home := t.TempDir()
	body := strings.Join([]string{
		`{"type":"user","timestamp":"2026-01-01T09:00:00Z","message":{"content":"<command-message>work</command-message>\n<command-name>/work</command-name>\n<command-args>fix the flaky login test</command-args>"}}`,
		`{"type":"user","timestamp":"2026-01-01T09:00:01Z","isMeta":true,"message":{"content":[{"type":"text","text":"Base directory for this skill: /tmp/skills/work"}]}}`,
		`{"type":"user","timestamp":"2026-01-01T09:05:00Z","message":{"content":"<command-name>/compact</command-name>\n<command-message>compact</command-message>\n<command-args>keep the plan</command-args>"}}`,
		`{"type":"user","timestamp":"2026-01-01T09:06:00Z","message":{"content":"<command-name>/clear</command-name>\n<command-message>clear</command-message>\n<command-args></command-args>"}}`,
		`{"type":"user","timestamp":"2026-01-01T09:07:00Z","message":{"content":"<command-message>land</command-message>\n<command-name>/land</command-name>"}}`,
	}, "\n") + "\n"
	writeTranscript(t, filepath.Join(home, ".claude", "projects", "-tmp-acme-app", "work.jsonl"), body)
	got, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("collected %d sessions, want 1", len(got))
	}
	for metric, want := range map[string]float64{"turns": 2, "commands": 4} {
		if v, ok := got[0].metric(metric); !ok || v != want {
			t.Fatalf("%s = %v, want %v", metric, v, want)
		}
	}
}

// Only a slash-command entry carries the person's arguments. The same tag
// quoted inside text the client injected is not a typed request.
func TestQuotedCommandArgsInInjectedTextIsNotATurn(t *testing.T) {
	home := t.TempDir()
	body := `{"type":"user","timestamp":"2026-01-01T09:00:00Z","message":{"content":"<system-reminder>earlier: <command-args>fix it</command-args></system-reminder>"}}` + "\n" +
		`{"type":"user","timestamp":"2026-01-01T09:00:01Z","message":{"content":"<local-command-stdout><command-args>x</command-args></local-command-stdout>"}}` + "\n"
	writeTranscript(t, filepath.Join(home, ".claude", "projects", "-tmp-acme-app", "quoted.jsonl"), body)
	got, err := collect(home, []string{"claude"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("collected %d sessions, want 1", len(got))
	}
	if v, ok := got[0].metric("turns"); !ok || v != 0 {
		t.Fatalf("turns = %v, want 0", v)
	}
}

// TestCollectDropsScratchProjects keeps synthetic sessions, such as an eval's
// headless runs, out of the population: a session whose project lies under the
// scratch directory is dropped for each client, while a sibling directory whose
// name only shares the prefix stays where the client records a real path.
func TestCollectDropsScratchProjects(t *testing.T) {
	home := t.TempDir()
	scratch := filepath.Join(home, scratchDir)
	stamp := `{"type":"user","timestamp":"2026-03-01T09:00:00Z","message":{"content":"go"}}` + "\n"
	for _, project := range []string{
		claudeSlug(filepath.Join(scratch, "acme-api")),
		claudeSlug(filepath.Join(home, "acme-api")),
	} {
		writeTranscript(t, filepath.Join(home, ".claude", "projects", project, "s.jsonl"), stamp)
	}
	day := filepath.Join(home, ".codex", "sessions", "2026", "03", "01")
	for i, cwd := range []string{filepath.Join(scratch, "shop"), filepath.Join(home, scratchDir+"-old", "shop"), filepath.Join(home, "shop")} {
		body := fmt.Sprintf(`{"timestamp":"2026-03-01T09:00:00Z","type":"session_meta","payload":{"id":"t%d","cwd":%q}}`+"\n", i, cwd)
		writeTranscript(t, filepath.Join(day, fmt.Sprintf("rollout-%d.jsonl", i)), body)
	}

	got, err := collect(home, []string{"claude", "codex"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	for _, s := range got {
		kept = append(kept, s.client+" "+filepath.Base(*s.project))
	}
	slices.Sort(kept)
	want := []string{"claude " + claudeSlug(filepath.Join(home, "acme-api")), "codex shop", "codex shop"}
	slices.Sort(want)
	if !slices.Equal(kept, want) {
		t.Fatalf("kept %q, want %q", kept, want)
	}
}
