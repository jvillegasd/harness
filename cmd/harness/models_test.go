package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

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

func agentFile(t *testing.T, base, client, role string) string {
	t.Helper()
	switch client {
	case "codex":
		return readFile(t, filepath.Join(base, "home", ".codex", "agents", role+".toml"))
	case "opencode":
		return readFile(t, filepath.Join(base, "home", ".config", "opencode", "agents", role+".md"))
	}
	return readFile(t, filepath.Join(base, "home", ".claude", "agents", role+".md"))
}

func TestModelsShowDefaultsToBalanced(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runModels(t, base, "show"), "models show")
	for _, want := range []string{
		"standard client=codex model=gpt-6.1-terra (preset:balanced)",
		"executor tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"worker tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"reviewer tier=high client=claude model=opus effort=medium (preset:balanced)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("show lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestModelsWithoutTTYPrintsUsage(t *testing.T) {
	r := runModels(t, modelsBase(t))
	if r.code == 0 {
		t.Fatalf("bare models without a TTY exited 0")
	}
	for _, want := range []string{"show ", "set --preset", "set <tier>", "set <role>", "resolve <role>"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("usage lacks %q:\n%s", want, r.stderr)
		}
	}
}

func TestModelsSetKeepsOtherStateKeysAndRejectsWithoutWriting(t *testing.T) {
	base := modelsBase(t)
	seed := `{"version": 1, "installed": {"claude": ["policy"]}, "files": []}` + "\n"
	writeMode(t, stateFile(base), seed, 0o644)
	if r := runModels(t, base, "set", "reviewer", "--tier", "ultra"); r.code == 0 {
		t.Error("set accepted an unknown tier")
	}
	if readFile(t, stateFile(base)) != seed {
		t.Error("a rejected set rewrote state.json")
	}
	ok(t, runModels(t, base, "set", "--preset", "codex"), "set preset")
	ok(t, runModels(t, base, "set", "code-search", "--tier", "high"), "set role")
	r := ok(t, runModels(t, base, "show"), "models show")
	for _, want := range []string{`reviewer tier=high client=codex model=gpt-6.1-sol effort=inherit (preset:codex) [codex gpt-6.1-sol does not support effort "medium"`, "heavy client=codex model=unavailable (preset:codex) [codex lists no heavy (astra) model; run `codex debug models`: not listed; roles at heavy fall back to high]", "code-search tier=high client=codex model=gpt-6.1-sol effort=inherit (override)"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("show lacks %q:\n%s", want, r.stdout)
		}
	}
	var s map[string]any
	if err := json.Unmarshal([]byte(readFile(t, stateFile(base))), &s); err != nil {
		t.Fatal(err)
	}
	if _, ok := s["installed"]; !ok {
		t.Error("set dropped the installer's keys from state.json")
	}
	ok(t, runModels(t, base, "reset"), "reset all")
	if strings.Contains(readFile(t, stateFile(base)), `"models"`) {
		t.Error("reset left a models block")
	}
}

func TestModelsResolveReportsTheAssignment(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runModels(t, base, "resolve", "worker", "--tier", "heavy", "--json"), "resolve")
	var got map[string]string
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("resolve output is not JSON: %v\n%s", err, r.stdout)
	}
	if got["role"] != "worker" || got["tier"] != "heavy" || got["client"] != "claude" || got["model"] != "opus" || got["effort"] != "inherit" || got["source"] != "launch" {
		t.Errorf("worker at heavy: %v", got)
	}
	if r := runModels(t, base, "resolve", "reviewer", "--tier", "fast"); r.code == 0 {
		t.Error("resolve gave a non-generic role a launch tier")
	}
}

func TestInstallRendersTheResolvedTiers(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runInstaller(t, base, "agents"), "install with the default preset")
	if !strings.Contains(r.stdout, "assignment: codex executor model=gpt-6.1-terra (standard, preset:balanced)") {
		t.Errorf("install did not report the tier and source:\n%s", r.stdout)
	}
	if !strings.Contains(agentFile(t, base, "codex", "executor"), "\nmodel = \"gpt-6.1-terra\"\n") {
		t.Error("the Codex executor does not carry the standard tier's model")
	}
	if !strings.Contains(agentFile(t, base, "claude", "executor"), "model: inherit\n") {
		t.Error("the Claude executor, served by Codex, does not keep the client default")
	}
	if a := agentFile(t, base, "claude", "reviewer"); !strings.Contains(a, "model: opus\n") || !strings.Contains(a, "effort: medium\n") {
		t.Errorf("claude reviewer lacks opus at medium effort:\n%s", a)
	}
	if !strings.Contains(agentFile(t, base, "codex", "worker"), "\nmodel = \"gpt-6.1-terra\"\n") {
		t.Error("the Codex worker does not carry its default tier's model")
	}
	ok(t, runModels(t, base, "set", "high", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "serve high from OpenCode")
	ok(t, runInstaller(t, base, "agents"), "install with OpenCode serving high")
	if a := agentFile(t, base, "opencode", "review-bugs"); !strings.Contains(a, "model: openai/gpt-5.6-sol\n") || !strings.Contains(a, "reasoningEffort: medium\n") {
		t.Errorf("the OpenCode lens does not follow the reviewer's tier:\n%s", a)
	}
	if strings.Contains(agentFile(t, base, "claude", "reviewer"), "model: opus") {
		t.Error("the Claude reviewer kept a model after its tier moved to OpenCode")
	}
}

func TestInstallFallsBackWhenATierCannotBeServed(t *testing.T) {
	base := modelsBase(t)
	writeShim(t, base, "codex", `case "$*" in "debug models") echo '{"models":[{"slug":"gpt-6.1-sol","visibility":"list"}]}' ;; esac`)
	r := ok(t, runInstaller(t, base, "agents"), "install without a terra model")
	if !strings.Contains(r.stderr+r.stdout, "executor runs at the standard tier on codex, which this machine cannot serve") {
		t.Errorf("no fallback warning:\n%s%s", r.stdout, r.stderr)
	}
	if strings.Contains(agentFile(t, base, "codex", "executor"), "model = ") {
		t.Error("the Codex executor carries a model this machine cannot run")
	}
	if strings.Contains(r.stderr+r.stdout, "review-bugs runs") {
		t.Errorf("a lens that follows the reviewer warned on its own:\n%s%s", r.stdout, r.stderr)
	}
}

func TestModelsRejectsAStaleChoice(t *testing.T) {
	base := modelsBase(t)
	writeMode(t, stateFile(base), `{"version": 1, "installed": {}, "files": [], "models": {"preset": "lavish"}}`+"\n", 0o644)
	if r := runInstaller(t, base, "agents"); r.code == 0 || !strings.Contains(r.stderr, "harness models reset") {
		t.Errorf("install over a stale choice: exit %d, stderr %q", r.code, r.stderr)
	}
	ok(t, runModels(t, base, "reset"), "the recommended reset")
	ok(t, runInstaller(t, base, "agents"), "install after reset")
}
