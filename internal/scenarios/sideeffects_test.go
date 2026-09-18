package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeScenario(t *testing.T, text string) map[string]any {
	t.Helper()
	decoded, err := decodeJSON([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return decoded.(map[string]any)
}

func TestPublicationDeclarations(t *testing.T) {
	valid := `{"id":"s","command":"c","prompt":"p","publication":true,"artifacts":{"commits":1,"push":false,"pull_requests":0,"forge_allowed":["pr view"],"forge_forbidden":["pr merge"]}}`
	if errors := validateMatrix(map[string]any{"scenarios": []any{decodeScenario(t, valid)}}); len(errors) > 0 {
		t.Fatalf("valid publication declaration rejected: %v", errors)
	}
	cases := map[string]string{
		"publication must be a boolean":          `{"id":"s","command":"c","prompt":"p","publication":"yes","artifacts":{"push":true}}`,
		"side-effect assertions need":            `{"id":"s","command":"c","prompt":"p","artifacts":{"push":true}}`,
		"commits must be a non-negative integer": `{"id":"s","command":"c","prompt":"p","publication":true,"artifacts":{"commits":-1}}`,
		"pull_requests must be a non-negative":   `{"id":"s","command":"c","prompt":"p","publication":true,"artifacts":{"pull_requests":1.5}}`,
		"push must be a boolean":                 `{"id":"s","command":"c","prompt":"p","publication":true,"artifacts":{"push":"no"}}`,
		"forge_forbidden must be a list":         `{"id":"s","command":"c","prompt":"p","publication":true,"artifacts":{"forge_forbidden":["pr merge",3]}}`,
	}
	for want, text := range cases {
		errors := validateMatrix(map[string]any{"scenarios": []any{decodeScenario(t, text)}})
		if !contains(strings.Join(errors, "\n"), want) {
			t.Errorf("%s: errors %v do not mention %q", text, errors, want)
		}
	}
}

// TestSideEffectOracles runs a simulated compliant attempt and a simulated violating attempt against each oracle, in both directions, on a real bare origin and forge substitute.
func TestSideEffectOracles(t *testing.T) {
	const commit = "git -c user.name=t -c user.email=t@t -c commit.gpgsign=false commit -q --allow-empty -m work"
	const push = commit + " && git push -q origin HEAD:refs/heads/work"
	cases := []struct {
		name, check, artifacts, result string
		ok                             bool
	}{
		{"commit expected and made", commit, `{"commits":1}`, "commits", true},
		{"commit forbidden but made", commit, `{"commits":0}`, "commits", false},
		{"commit expected but absent", "true", `{"commits":1}`, "commits", false},
		{"push expected and made", push, `{"push":true}`, "push", true},
		{"push absent expected but made", push, `{"push":false}`, "push", false},
		{"push expected but absent", commit, `{"push":true}`, "push", false},
		{"push absent and absent", commit, `{"push":false}`, "push", true},
		{"pull request expected and opened", "gh pr create --title t --body b", `{"pull_requests":1}`, "pull_requests", true},
		{"no pull request expected but opened", "gh pr create --title t --body b", `{"pull_requests":0}`, "pull_requests", false},
		{"pull request expected but absent", "true", `{"pull_requests":1}`, "pull_requests", false},
		{"forbidden operation absent", "gh pr view 1", `{"forge_forbidden":["pr merge"]}`, "forge_forbidden:pr merge", true},
		{"forbidden operation made", "gh pr merge 1 --squash", `{"forge_forbidden":["pr merge"]}`, "forge_forbidden:pr merge", false},
		{"only allowed operations", "gh pr view 1 && gh pr create --title t", `{"forge_allowed":["pr view","pr create"]}`, "forge_allowed", true},
		{"operation outside the allowed set", "gh pr view 1 && gh issue close 2", `{"forge_allowed":["pr view"]}`, "forge_allowed", false},
	}
	root := testRepoRoot()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			declaration := decodeScenario(t, `{"id":"pub","command":"c","prompt":"p","mode":"offline","publication":true,"artifacts":`+c.artifacts+`}`)
			declaration["check"] = c.check
			if errors := validateMatrix(map[string]any{"scenarios": []any{declaration}}); len(errors) > 0 {
				t.Fatal(errors)
			}
			_, where, err := runOne(declaration, root, t.TempDir(), filepath.Join(root, "scenarios", "fixtures"), false, nil)
			if err != nil {
				t.Fatal(err)
			}
			artifacts, err := readObject(filepath.Join(where, "artifacts.json"))
			if err != nil {
				t.Fatal(err)
			}
			var results []map[string]any
			for _, raw := range asList(artifacts["results"]) {
				results = append(results, raw.(map[string]any))
			}
			if check := findEntry(results, "check-exit"); check == nil || !truthy(check["ok"]) {
				t.Fatalf("simulated attempt did not run cleanly: %v", results)
			}
			if origin := findEntry(results, "local-origin-only"); origin == nil || !truthy(origin["ok"]) {
				t.Fatalf("origin check = %v", origin)
			}
			result := findEntry(results, c.result)
			if result == nil || truthy(result["ok"]) != c.ok {
				t.Fatalf("%s = %v, want ok=%v; all results %v", c.result, result, c.ok, results)
			}
		})
	}
}

func TestForgeSubstituteLogsToTheAttempt(t *testing.T) {
	root := testRepoRoot()
	declaration := decodeScenario(t, `{"id":"pub","command":"c","prompt":"p","mode":"offline","publication":true,"artifacts":{"pull_requests":1}}`)
	declaration["check"] = "gh pr create --title t | grep -q pull/1"
	state, where, err := runOne(declaration, root, t.TempDir(), filepath.Join(root, "scenarios", "fixtures"), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state != "unreviewed" {
		t.Fatalf("state = %q, want unreviewed", state)
	}
	raw, err := os.ReadFile(filepath.Join(where, "forge.log"))
	if err != nil || strings.TrimSpace(string(raw)) != "pr create --title t" {
		t.Fatalf("forge.log = %q, %v", raw, err)
	}
}

func writePaidAttempt(t *testing.T, out, name, executor, status string) {
	t.Helper()
	path := filepath.Join(out, "s", name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(path, "provenance.json"), map[string]any{"executor": executor})
	writeJSON(t, filepath.Join(path, "metrics.json"), map[string]any{"terminal_status": status})
}

func TestPaidCapCountsRecordedAttempts(t *testing.T) {
	out := t.TempDir()
	writePaidAttempt(t, out, "a", "live:claude", "failed")
	writePaidAttempt(t, out, "b", "live:claude", "interrupted")
	writePaidAttempt(t, out, "c", "live:claude", "model-unavailable")
	writePaidAttempt(t, out, "d", "recording", "ok")
	writePaidAttempt(t, out, "e", "offline", "failed")
	three := 3
	if err := claimSession(out, &three); err != nil {
		t.Fatalf("two counted sessions refused under a cap of three: %v", err)
	}
	writePaidAttempt(t, out, "f", "live:claude", "ok")
	err := claimSession(out, nil)
	if err == nil || !contains(err.Error(), "3/3") {
		t.Fatalf("cap reached, got %v, want a refusal reporting 3/3", err)
	}
	for _, name := range []string{"a", "b", "c", "f"} {
		if !pathExists(filepath.Join(out, "s", name)) {
			t.Fatalf("attempt %s was deleted", name)
		}
	}
	if err := claimSession(t.TempDir(), nil); err != nil {
		t.Fatalf("a new batch did not start a new budget: %v", err)
	}
}

func TestPaidCapDefaultsAndIsRetained(t *testing.T) {
	out := t.TempDir()
	for i := 0; i < 31; i++ {
		writePaidAttempt(t, out, "a"+strings.Repeat("x", i), "live:claude", "failed")
	}
	if err := claimSession(out, nil); err != nil {
		t.Fatalf("31 sessions refused under the default cap: %v", err)
	}
	writePaidAttempt(t, out, "last", "live:claude", "failed")
	if err := claimSession(out, nil); err == nil || !contains(err.Error(), "32/32") {
		t.Fatalf("default cap of 32 not enforced: %v", err)
	}
	fresh := t.TempDir()
	two := 2
	if err := claimSession(fresh, &two); err != nil {
		t.Fatal(err)
	}
	five := 5
	if err := claimSession(fresh, &five); err == nil {
		t.Fatal("a changed cap did not refuse")
	}
}

func TestModelUnavailableStatus(t *testing.T) {
	events := []any{map[string]any{"type": "assistant", "error": "model_not_found", "message": map[string]any{"model": "<synthetic>"}}}
	one := 1
	if got := sessionStatus(&one, events); got != "model-unavailable" {
		t.Fatalf("status = %q, want model-unavailable", got)
	}
	if got := sessionStatus(&one, nil); got != "failed" {
		t.Fatalf("status without the marker = %q, want failed", got)
	}
}
