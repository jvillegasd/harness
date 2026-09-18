package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// modelsBase is a sandbox whose shims list two OpenCode models and whose orca shim is absent until a test adds it.
func modelsBase(t *testing.T) string {
	t.Helper()
	base := sandbox(t)
	writeShim(t, base, "opencode", `case "$1" in models) printf 'opencode-go/glm-5.3\nopencode-go/deepseek-v4-flash\nopenai/gpt-5.6-sol\n' ;; esac`)
	return base
}

func runModels(t *testing.T, base string, args ...string) result {
	t.Helper()
	return runHarness(t, base, nil, append([]string{"models"}, args...)...)
}

func stateFile(base string) string { return filepath.Join(base, "home", ".harness", "state.json") }

func TestModelsShowDefaultsToBalanced(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runModels(t, base, "show"), "models show")
	for _, want := range []string{
		"executor client=opencode model=opencode-go/glm-5.3 effort=inherit (preset:balanced)",
		"reviewer-a client=claude model=inherit effort=inherit (preset:balanced)",
		"reviewer-b client=claude model=inherit effort=inherit (preset:balanced)",
		"review-refuter client=claude model=inherit effort=inherit (preset:balanced)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("show lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestModelsSetPresetKeepsOtherStateKeys(t *testing.T) {
	base := modelsBase(t)
	writeMode(t, stateFile(base), `{"version": 1, "installed": {"claude": ["policy"]}, "files": []}`+"\n", 0o644)
	ok(t, runModels(t, base, "set", "--preset", "diversity"), "set preset")
	r := ok(t, runModels(t, base, "show"), "models show")
	if !strings.Contains(r.stdout, "reviewer-b client=opencode model=openai/gpt-5.6-sol effort=inherit (preset:diversity)") {
		t.Errorf("show does not report diversity:\n%s", r.stdout)
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(readFile(t, stateFile(base))), &s); err != nil {
		t.Fatal(err)
	}
	if _, ok := s["installed"]; !ok {
		t.Error("set dropped the installer's keys from state.json")
	}
}

func TestModelsSetRoleOverridesOnlyThatRole(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "reviewer-b", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "set role")
	r := ok(t, runModels(t, base, "show"), "models show")
	if !strings.Contains(r.stdout, "reviewer-b client=opencode model=openai/gpt-5.6-sol effort=inherit (override)") {
		t.Errorf("override not reported:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "reviewer-a client=claude model=inherit effort=inherit (preset:balanced)") {
		t.Errorf("the override leaked into another role:\n%s", r.stdout)
	}
}

func TestModelsSetRejectsInvalidValuesWithoutWriting(t *testing.T) {
	for name, args := range map[string][]string{
		"unknown opencode id":      {"reviewer-b", "--client", "opencode", "--model", "openai/nope"},
		"unknown claude alias":     {"reviewer-b", "--client", "claude", "--model", "claudius"},
		"unsupported haiku effort": {"reviewer-b", "--client", "claude", "--model", "haiku", "--effort", "high"},
		"sonnet has no xhigh":      {"reviewer-b", "--client", "claude", "--model", "sonnet", "--effort", "xhigh"},
		"unknown role":             {"planner", "--client", "claude", "--model", "opus"},
		"unknown preset":           {"--preset", "lavish"},
	} {
		base := modelsBase(t)
		if r := runModels(t, base, append([]string{"set"}, args...)...); r.code == 0 {
			t.Errorf("%s: set exited 0", name)
		}
		if _, err := os.Stat(stateFile(base)); err == nil {
			t.Errorf("%s: a rejected set wrote state.json", name)
		}
	}
}

func TestModelsResetRemovesOverrideThenBlock(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "--preset", "economy"), "set preset")
	ok(t, runModels(t, base, "set", "reviewer-b", "--client", "claude", "--model", "opus", "--effort", "high"), "set role")
	ok(t, runModels(t, base, "reset", "reviewer-b"), "reset role")
	if r := ok(t, runModels(t, base, "show"), "show"); !strings.Contains(r.stdout, "reviewer-b client=claude model=sonnet effort=inherit (preset:economy)") {
		t.Errorf("reset role did not fall back to the preset:\n%s", r.stdout)
	}
	ok(t, runModels(t, base, "reset"), "reset all")
	if r := ok(t, runModels(t, base, "show"), "show"); !strings.Contains(r.stdout, "(preset:balanced)") {
		t.Errorf("reset did not fall back to balanced:\n%s", r.stdout)
	}
	if strings.Contains(readFile(t, stateFile(base)), `"models"`) {
		t.Error("reset left a models block")
	}
}

func TestModelsResolveRoutes(t *testing.T) {
	base := modelsBase(t)
	route := func(role, session string) map[string]string {
		t.Helper()
		r := ok(t, runModels(t, base, "resolve", role, "--session", session, "--json"), "resolve")
		var out map[string]string
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
			t.Fatalf("resolve output is not JSON: %v\n%s", err, r.stdout)
		}
		return out
	}
	if got := route("reviewer-a", "claude"); got["route"] != "native" || got["client"] != "claude" {
		t.Errorf("same-client role: %v", got)
	}
	if got := route("executor", "claude"); got["route"] != "headless" || got["model"] != "opencode-go/glm-5.3" {
		t.Errorf("cross-client role outside Orca: %v", got)
	}
	writeShim(t, base, "orca", `case "$1 $2" in "worktree current") echo '{"ok":true}' ;; *) exit 1 ;; esac`)
	if got := route("executor", "claude"); got["route"] != "orca" {
		t.Errorf("cross-client role inside Orca: %v", got)
	}
	if got := route("executor", "opencode"); got["route"] != "native" {
		t.Errorf("executor in an OpenCode session: %v", got)
	}
	if r := runModels(t, base, "resolve", "executor", "--json"); r.code == 0 {
		t.Error("resolve without --session exited 0")
	}
}

func TestInstallRendersTheResolvedAssignments(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runInstaller(t, base, "--skip-packages"), "install with the default preset")
	if !strings.Contains(r.stdout, "assignment: opencode executor model=opencode-go/glm-5.3 (preset:balanced)") {
		t.Errorf("install did not report the preset source:\n%s", r.stdout)
	}
	agent := func(client, role string) string {
		dir := filepath.Join(base, "home", ".claude", "agents")
		if client == "opencode" {
			dir = filepath.Join(base, "home", ".config", "opencode", "agents")
		}
		return readFile(t, filepath.Join(dir, role+".md"))
	}
	if !strings.Contains(agent("opencode", "executor"), "model: opencode-go/glm-5.3\n") {
		t.Error("the OpenCode executor does not carry the balanced model")
	}
	if !strings.Contains(agent("claude", "executor"), "model: inherit\n") {
		t.Error("the Claude executor, assigned to OpenCode, does not keep the client default")
	}
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	ok(t, runInstaller(t, base, "--skip-packages"), "install with performance")
	if r := ok(t, runModels(t, base, "show"), "show after install"); !strings.Contains(r.stdout, "(preset:performance)") {
		t.Errorf("install dropped the model choice:\n%s", r.stdout)
	}
	if a := agent("claude", "reviewer-a"); !strings.Contains(a, "model: fable\n") || !strings.Contains(a, "effort: high\n") {
		t.Errorf("claude reviewer-a lacks fable at high effort:\n%s", a)
	}
	if strings.Contains(agent("opencode", "reviewer-a"), "model:") {
		t.Error("opencode reviewer-a, assigned to Claude, carries a model instead of the client default")
	}
}
