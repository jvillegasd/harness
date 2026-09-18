package scenarios

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func testScenario(overrides map[string]any) map[string]any {
	base := map[string]any{
		"id":        "s",
		"command":   "c",
		"prompt":    "p",
		"artifacts": map[string]any{"tree_unchanged": true},
	}
	for key, value := range overrides {
		base[key] = value
	}
	return base
}

func writeAttempt(t *testing.T, path, digest string, ok, review bool, metrics map[string]any) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(path, "prompts.txt"), "prompt\n")
	writeJSON(t, filepath.Join(path, "provenance.json"), map[string]any{"input_digest": digest, "recorded_at": "now"})
	writeFile(t, filepath.Join(path, "events.jsonl"), "")
	writeJSON(t, filepath.Join(path, "artifacts.json"), map[string]any{"ok": ok, "results": []any{map[string]any{"name": "x", "ok": ok}}})
	writeFile(t, filepath.Join(path, "transcript.jsonl"), "")
	if metrics == nil {
		metrics = map[string]any{
			"scenario": "s", "model": nil, "input_tokens": nil, "output_tokens": nil,
			"cached_input": nil, "tool_calls": 0, "command_output_bytes": 0,
			"wall_seconds": 0.0, "terminal_status": "ok",
		}
	}
	writeJSON(t, filepath.Join(path, "metrics.json"), metrics)
	if review {
		writeJSON(t, filepath.Join(path, "review.json"), map[string]any{"reviewed": true, "by": "test"})
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	writeFile(t, path, indentJSON(value))
}

func TestEvaluateStates(t *testing.T) {
	declaration := testScenario(nil)
	cases := []struct {
		name   string
		setup  func(t *testing.T, attempt string)
		digest string
		want   string
	}{
		{"incomplete when evidence missing", func(t *testing.T, attempt string) {
			writeAttempt(t, attempt, "abc", true, false, nil)
			_ = os.Remove(filepath.Join(attempt, "transcript.jsonl"))
		}, "abc", "incomplete"},
		{"stale when inputs changed", func(t *testing.T, attempt string) {
			writeAttempt(t, attempt, "old", true, false, nil)
		}, "new", "stale"},
		{"fail when artifact checks fail", func(t *testing.T, attempt string) {
			writeAttempt(t, attempt, "abc", false, false, nil)
		}, "abc", "fail"},
		{"unreviewed when no review record", func(t *testing.T, attempt string) {
			writeAttempt(t, attempt, "abc", true, false, nil)
		}, "abc", "unreviewed"},
		{"pass when artifacts and review agree", func(t *testing.T, attempt string) {
			writeAttempt(t, attempt, "abc", true, true, nil)
		}, "abc", "pass"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			attempt := filepath.Join(t.TempDir(), "attempt")
			c.setup(t, attempt)
			if got, _ := evaluate(attempt, declaration, c.digest); got != c.want {
				t.Fatalf("evaluate = %q, want %q", got, c.want)
			}
		})
	}
}

func TestInterruptedBeatsAFailingArtifactCheck(t *testing.T) {
	declaration := testScenario(nil)
	attempt := filepath.Join(t.TempDir(), "attempt")
	writeAttempt(t, attempt, "abc", false, false, map[string]any{"terminal_status": "interrupted"})
	if got, _ := evaluate(attempt, declaration, "abc"); got != "interrupted" {
		t.Fatalf("evaluate = %q, want interrupted", got)
	}
}

func TestMissingMetricsIsIncomplete(t *testing.T) {
	declaration := testScenario(nil)
	attempt := filepath.Join(t.TempDir(), "attempt")
	writeAttempt(t, attempt, "abc", true, true, nil)
	_ = os.Remove(filepath.Join(attempt, "metrics.json"))
	if got, _ := evaluate(attempt, declaration, "abc"); got != "incomplete" {
		t.Fatalf("evaluate = %q, want incomplete", got)
	}
}

func TestShippedMatrixIsValid(t *testing.T) {
	matrix := loadTestMatrix(t)
	if errors := validateMatrix(matrix); len(errors) != 0 {
		t.Fatalf("shipped matrix errors: %v", errors)
	}
}

func TestDeclarationRejections(t *testing.T) {
	cases := map[string]map[string]any{
		"artifacts": {"scenarios": []any{map[string]any{"id": "s", "command": "c", "prompt": "p", "artifacts": map[string]any{}}}},
		"check":     {"scenarios": []any{testScenario(map[string]any{"mode": "offline"})}},
		"duplicate": {"scenarios": []any{testScenario(nil), testScenario(nil)}},
		"contains":  {"scenarios": []any{testScenario(map[string]any{"artifacts": map[string]any{"contains": []any{map[string]any{"path": "x"}}}})}},
	}
	for keyword, matrix := range cases {
		errors := validateMatrix(matrix)
		found := false
		for _, problem := range errors {
			if containsString([]string{problem}, problem) && contains(problem, keyword) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: validateMatrix = %v, want an error naming %q", keyword, errors, keyword)
		}
	}
}

func TestNonMappingArtifactsIsRejected(t *testing.T) {
	errors := validateMatrix(map[string]any{"scenarios": []any{testScenario(map[string]any{"artifacts": []any{"x"}})}})
	if len(errors) == 0 || !contains(errors[0], "artifacts") {
		t.Fatalf("validateMatrix = %v, want an artifacts error", errors)
	}
}

func TestContainsCannotEscapeTheRepository(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(filepath.Dir(repo), "outside.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	declaration := testScenario(map[string]any{
		"artifacts": map[string]any{"contains": []any{map[string]any{"path": "../outside.txt", "text": "secret"}}},
	})
	entries := evaluateArtifacts(declaration, map[string]string{}, map[string]string{}, repo, nil, true, nil)
	if entries[0]["ok"] != false {
		t.Fatalf("a path outside the repository was accepted: %v", entries[0])
	}
}

func TestDigestChangesWithInputsAndDeclaration(t *testing.T) {
	root := t.TempDir()
	fixtures := filepath.Join(root, "fixtures")
	writeFile(t, filepath.Join(root, "input.txt"), "one")
	declaration := testScenario(map[string]any{"harness_inputs": []any{"input.txt"}})
	first := scenarioDigest(declaration, root, fixtures)
	writeFile(t, filepath.Join(root, "input.txt"), "two")
	if scenarioDigest(declaration, root, fixtures) == first {
		t.Fatal("digest did not change when a harness input changed")
	}
	if scenarioDigest(testScenario(nil), root, fixtures) == scenarioDigest(testScenario(map[string]any{"prompt": "other"}), root, fixtures) {
		t.Fatal("digest did not change when the declaration changed")
	}
}

func TestForgeGuardInterceptsARealCall(t *testing.T) {
	workspace := t.TempDir()
	shim, log, err := installForgeGuard(workspace, false)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(shim, "gh"), "issue", "list")
	cmd.Env = forgeEnv(shim, false)
	_ = cmd.Run()
	if cmd.ProcessState.ExitCode() == 0 {
		t.Fatal("the forge shim did not refuse")
	}
	raw, err := os.ReadFile(log)
	if err != nil || !contains(string(raw), "issue list") {
		t.Fatalf("forge log = %q, %v", raw, err)
	}
}

func TestMatrixRunsOfflineAndReviewPromotesToPass(t *testing.T) {
	out := t.TempDir()
	root := testRepoRoot()
	matrix := loadTestMatrixAt(t, filepath.Join(root, "scenarios", "matrix.json"))
	fixtures := filepath.Join(root, "scenarios", "fixtures")
	for _, raw := range list(matrix, "scenarios") {
		declaration, _ := raw.(map[string]any)
		state, where, err := runOne(declaration, root, out, fixtures, false, nil)
		if err != nil {
			t.Fatalf("%s: %v", getString(declaration, "id"), err)
		}
		if state != "unreviewed" {
			t.Fatalf("%s = %q at %s, want unreviewed", getString(declaration, "id"), state, where)
		}
		if err := os.WriteFile(filepath.Join(where, "review.json"), []byte(`{"reviewed": true}`), 0o644); err != nil {
			t.Fatal(err)
		}
		digest := scenarioDigest(declaration, root, fixtures)
		if got, _ := evaluate(where, declaration, digest); got != "pass" {
			t.Fatalf("%s after review = %q, want pass", getString(declaration, "id"), got)
		}
	}
}

func TestFailingCheckFailsTheAttempt(t *testing.T) {
	out := t.TempDir()
	declaration := testScenario(map[string]any{"id": "bad-check", "mode": "offline", "check": "exit 3"})
	state, where, err := runOne(declaration, testRepoRoot(), out, filepath.Join(testRepoRoot(), "scenarios", "fixtures"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "fail" {
		t.Fatalf("state = %q at %s, want fail", state, where)
	}
}

func TestReportSeparatesLocalTooling(t *testing.T) {
	root := testRepoRoot()
	text := report(loadTestMatrixAt(t, filepath.Join(root, "scenarios", "matrix.json")), t.TempDir(), root)
	for _, want := range []string{"local-tooling: pass", "behavioral-acceptance:", "incomplete (no attempt)"} {
		if !contains(text, want) {
			t.Fatalf("report missing %q:\n%s", want, text)
		}
	}
}

func TestReadOnlyOracleChecks(t *testing.T) {
	declaration := testScenario(map[string]any{"artifacts": map[string]any{"fixtures_unchanged": true}})
	entries := evaluateArtifacts(declaration, map[string]string{}, map[string]string{}, t.TempDir(), nil, false, nil)
	if entries[0]["ok"] != false {
		t.Fatal("a changed fixture set passed a fixtures_unchanged check")
	}
	declaration = testScenario(map[string]any{"artifacts": map[string]any{"tree_unchanged": true}})
	entries = evaluateArtifacts(declaration, map[string]string{"a": "b"}, map[string]string{"a": "b"}, t.TempDir(), []string{"issue list"}, true, nil)
	forge := findEntry(entries, "no-external-forge")
	if forge == nil || forge["ok"] != false {
		t.Fatalf("a forge call passed the read-only oracle: %v", forge)
	}
}

func TestShippedReadOnlyScenariosDeclareNoWriteChecks(t *testing.T) {
	matrix := loadTestMatrix(t)
	for _, id := range []string{"ask-read-only", "read-only-execution"} {
		declaration := findScenario(matrix, id)
		if declaration == nil || !truthy(dict(declaration, "artifacts")["tree_unchanged"]) || !truthy(dict(declaration, "artifacts")["fixtures_unchanged"]) {
			t.Fatalf("%s does not declare both read-only checks", id)
		}
	}
	declaration := findScenario(matrix, "read-only-execution")
	if mode(declaration) != "offline" || !truthy(declaration["check"]) {
		t.Fatalf("read-only-execution is not an offline command scenario")
	}
}

func TestCompare(t *testing.T) {
	tmp := t.TempDir()
	declaration := testScenario(map[string]any{"id": "ask-read-only"})
	matrix := map[string]any{"scenarios": []any{declaration}}
	fixtures := filepath.Join(tmp, "scenarios", "fixtures")
	digest := scenarioDigest(declaration, tmp, fixtures)

	batch := func(side string, specs []map[string]any) {
		for index, spec := range specs {
			path := filepath.Join(tmp, side, "ask-read-only", "a"+formatNumber(float64(index))+"-"+digest[:8])
			metrics := map[string]any{
				"scenario": "ask-read-only", "model": spec["model"], "input_tokens": spec["input_tokens"],
				"output_tokens": spec["output_tokens"], "cached_input": spec["cached_input"],
				"tool_calls": numberOr(spec["tool_calls"], 0), "command_output_bytes": numberOr(spec["command_output_bytes"], 0),
				"wall_seconds": numberOr(spec["wall_seconds"], 1.0), "terminal_status": stringOr(spec["terminal"], "ok"),
			}
			writeAttempt(t, path, stringOr(spec["digest"], digest), boolOr(spec["ok"], true), boolOr(spec["reviewed"], true), metrics)
		}
	}
	compareText := func() string {
		return compare(filepath.Join(tmp, "before"), filepath.Join(tmp, "after"), matrix, tmp)
	}
	batch("before", []map[string]any{{"model": "m1", "input_tokens": 100}})
	batch("after", []map[string]any{{"model": "m1", "input_tokens": 80}})
	if indexOfText(compareText(), "pass rate") >= indexOfText(compareText(), "efficiency") {
		t.Fatal("pass rates did not precede efficiency")
	}
	if !contains(compareText(), "smoke") {
		t.Fatal("a single successful pair was not reported as smoke")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("before", []map[string]any{{"model": "m1", "ok": false, "input_tokens": 100}})
	batch("after", []map[string]any{{"model": "m1", "input_tokens": 80}})
	if !contains(compareText(), "no comparable pair") {
		t.Fatal("a failed baseline was treated as an efficiency win")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("before", []map[string]any{{"model": "m1", "input_tokens": 100}})
	batch("after", []map[string]any{{"model": "m2", "input_tokens": 80}})
	if !contains(compareText(), "no comparable pair") {
		t.Fatal("unlike models were pooled")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("before", []map[string]any{{"model": "m1", "input_tokens": 100}, {"model": "m2", "input_tokens": 100}})
	batch("after", []map[string]any{{"model": "m1", "input_tokens": 80}, {"model": "m2", "input_tokens": 80}})
	if contains(compareText(), "smoke") {
		t.Fatal("two successful pairs were reported as smoke")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("before", []map[string]any{{"model": "m1", "terminal": "interrupted"}, {"model": "m1", "reviewed": false}})
	batch("after", []map[string]any{{"model": "m1", "input_tokens": 80}})
	if !contains(compareText(), "interrupted") || !contains(compareText(), "unreviewed") {
		t.Fatal("nonpassing attempts were not visible")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("after", []map[string]any{{"model": "m1", "input_tokens": 80}})
	if !contains(compareText(), "unavailable") {
		t.Fatal("an unavailable scenario was not visible")
	}

}

func TestCompareCrossVersionAndDrift(t *testing.T) {
	tmp := t.TempDir()
	declaration := testScenario(map[string]any{"id": "ask-read-only"})
	matrix := map[string]any{"scenarios": []any{declaration}}
	fixtures := filepath.Join(tmp, "scenarios", "fixtures")
	digest := scenarioDigest(declaration, tmp, fixtures)

	batch := func(side, at string, specs []map[string]any) {
		for index, spec := range specs {
			path := filepath.Join(tmp, side, "ask-read-only", "a"+formatNumber(float64(index))+"-"+at[:8])
			metrics := map[string]any{
				"scenario": "ask-read-only", "model": spec["model"], "input_tokens": spec["input_tokens"],
				"output_tokens": nil, "cached_input": nil, "tool_calls": 0, "command_output_bytes": 0,
				"wall_seconds": 1.0, "terminal_status": "ok",
			}
			writeAttempt(t, path, at, true, true, metrics)
		}
	}
	batch("before", "oldharness", []map[string]any{{"model": "m1", "input_tokens": 100}})
	batch("after", digest, []map[string]any{{"model": "m1", "input_tokens": 80}})
	if contains(compare(filepath.Join(tmp, "before"), filepath.Join(tmp, "after"), matrix, tmp), "no comparable pair") {
		t.Fatal("cross-version batches did not pair")
	}

	os.RemoveAll(filepath.Join(tmp, "before"))
	os.RemoveAll(filepath.Join(tmp, "after"))
	batch("before", "newharness", []map[string]any{{"model": "m1", "input_tokens": 100}, {"model": "m1", "input_tokens": 90}})
	writeAttempt(t, filepath.Join(tmp, "before", "ask-read-only", "0-old"), "oldharness", true, true, map[string]any{
		"scenario": "ask-read-only", "model": "m1", "input_tokens": 90, "output_tokens": nil, "cached_input": nil,
		"tool_calls": 0, "command_output_bytes": 0, "wall_seconds": 1.0, "terminal_status": "ok",
	})
	batch("after", digest, []map[string]any{{"model": "m1", "input_tokens": 80}})
	text := compare(filepath.Join(tmp, "before"), filepath.Join(tmp, "after"), matrix, tmp)
	if !contains(text, "stale=1") || !contains(text, "pass 2/3") {
		t.Fatalf("within-batch drift was not visible:\n%s", text)
	}
}

func TestMetrics(t *testing.T) {
	summary := []any{map[string]any{"type": "result", "model": "claude-x", "usage": map[string]any{
		"input_tokens": 1200, "output_tokens": 340, "cache_read_input_tokens": 800,
	}}}
	usage := parseUsage(summary)
	if floatOf(usage["input_tokens"]) != 1200 || floatOf(usage["output_tokens"]) != 340 || floatOf(usage["cached_input"]) != 800 {
		t.Fatalf("parse_usage = %v", usage)
	}
	if model := modelFromEvents(summary); model == nil || *model != "claude-x" {
		t.Fatalf("model_from_events = %v", model)
	}

	missing := parseUsage([]any{map[string]any{"type": "result"}})
	for _, field := range []string{"input_tokens", "output_tokens", "cached_input"} {
		if missing[field] != nil {
			t.Fatalf("an absent measurement should be unknown, got %v", missing)
		}
	}

	malformed := parseUsage([]any{
		map[string]any{"type": "result", "usage": "nope"},
		map[string]any{"type": "assistant", "message": map[string]any{"usage": map[string]any{"input_tokens": "lots"}}},
	})
	for _, field := range []string{"input_tokens", "output_tokens", "cached_input"} {
		if malformed[field] != nil {
			t.Fatalf("malformed usage should be unknown, got %v", malformed)
		}
	}

	summed := parseUsage([]any{
		map[string]any{"type": "assistant", "message": map[string]any{"usage": map[string]any{"input_tokens": 10, "output_tokens": 2}}},
		map[string]any{"type": "assistant", "message": map[string]any{"usage": map[string]any{"input_tokens": 5, "output_tokens": 3}}},
	})
	if floatOf(summed["input_tokens"]) != 15 || floatOf(summed["output_tokens"]) != 5 || summed["cached_input"] != nil {
		t.Fatalf("per-message usage = %v", summed)
	}

	cached := parseUsage([]any{map[string]any{"type": "result", "usage": map[string]any{
		"input_tokens": 4, "output_tokens": 1, "prompt_tokens_details": map[string]any{"cached_tokens": 3},
	}}})
	if floatOf(cached["cached_input"]) != 3 {
		t.Fatalf("cached variant = %v", cached)
	}

	for _, c := range []struct {
		code *int
		want string
	}{{nil, "unknown"}, {intPtr(0), "ok"}, {intPtr(3), "failed"}, {intPtr(-9), "interrupted"}, {intPtr(143), "interrupted"}} {
		if got := terminalStatus(c.code); got != c.want {
			t.Fatalf("terminal_status(%v) = %q, want %q", c.code, got, c.want)
		}
	}

	calls := countToolCalls([]any{
		map[string]any{"type": "tool_use"},
		map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use"}, map[string]any{"type": "text"}}}},
		map[string]any{"type": "tool_result"},
	})
	if calls != 2 {
		t.Fatalf("count_tool_calls = %d, want 2", calls)
	}

	bytesCount := commandOutputBytes([]any{
		map[string]any{"type": "tool_result", "content": "abc"},
		map[string]any{"type": "tool_result", "content": "de"},
		map[string]any{"type": "assistant"},
	})
	if bytesCount != 5 {
		t.Fatalf("command_output_bytes = %d, want 5", bytesCount)
	}
	nested := commandOutputBytes([]any{map[string]any{"type": "user", "message": map[string]any{
		"content": []any{map[string]any{"type": "tool_result", "content": []any{map[string]any{"type": "text", "text": "abc"}}}},
	}}})
	if nested != 3 {
		t.Fatalf("nested command_output_bytes = %d, want 3", nested)
	}

	record := buildMetrics(map[string]any{"id": "s"}, []any{map[string]any{"type": "result", "model": "m", "usage": map[string]any{"input_tokens": 1, "output_tokens": 2}}}, 1.5, "ok")
	if len(record) != len(metricFields) {
		t.Fatalf("build_metrics fields = %v", record)
	}
	if record["scenario"] != "s" || record["model"] != "m" || floatOf(record["input_tokens"]) != 1 || floatOf(record["wall_seconds"]) != 1.5 || record["terminal_status"] != "ok" {
		t.Fatalf("build_metrics = %v", record)
	}
}

// ---- helpers ----

func testRepoRoot() string {
	abs, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return abs
}

func loadTestMatrix(t *testing.T) map[string]any {
	t.Helper()
	return loadTestMatrixAt(t, filepath.Join(testRepoRoot(), "scenarios", "matrix.json"))
}

func loadTestMatrixAt(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	matrix, _ := decoded.(map[string]any)
	return matrix
}

func findScenario(matrix map[string]any, id string) map[string]any {
	for _, raw := range list(matrix, "scenarios") {
		scenario, _ := raw.(map[string]any)
		if getString(scenario, "id") == id {
			return scenario
		}
	}
	return nil
}

func findEntry(entries []map[string]any, name string) map[string]any {
	for _, item := range entries {
		if getString(item, "name") == name {
			return item
		}
	}
	return nil
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func indexOfText(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func intPtr(value int) *int { return &value }

func numberOr(value any, fallback float64) any {
	if f, ok := toNumber(value); ok {
		return f
	}
	return fallback
}

func stringOr(value any, fallback string) string {
	if text, ok := value.(string); ok {
		return text
	}
	return fallback
}

func boolOr(value any, fallback bool) bool {
	if flag, ok := value.(bool); ok {
		return flag
	}
	return fallback
}
