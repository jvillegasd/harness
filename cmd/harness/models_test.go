package main

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/models"
)

// modelsBase is a sandbox whose shims list available OpenCode and Codex models.
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
		"executor client=codex model=gpt-6-luna effort=inherit (preset:balanced)",
		"code-search client=claude model=sonnet effort=inherit (preset:balanced)",
		"reviewer client=claude model=opus effort=low (preset:balanced)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("show lacks %q:\n%s", want, r.stdout)
		}
	}
}

// TestModelsWithoutTTYPrintsUsage keeps a script from hanging on the picker: with piped stdin the bare command prints the show and set usage and exits nonzero.
func TestModelsWithoutTTYPrintsUsage(t *testing.T) {
	r := runModels(t, modelsBase(t))
	if r.code == 0 {
		t.Fatalf("bare models without a TTY exited 0")
	}
	for _, want := range []string{"show ", "set --preset", "set <role>"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("usage lacks %q:\n%s", want, r.stderr)
		}
	}
}

func TestModelsSetPresetKeepsOtherStateKeys(t *testing.T) {
	base := modelsBase(t)
	writeMode(t, stateFile(base), `{"version": 1, "installed": {"claude": ["policy"]}, "files": []}`+"\n", 0o644)
	ok(t, runModels(t, base, "set", "--preset", "diversity"), "set preset")
	r := ok(t, runModels(t, base, "show"), "models show")
	if !strings.Contains(r.stdout, "reviewer client=codex model=gpt-6.1-sol effort=low (preset:diversity)") {
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
	ok(t, runModels(t, base, "set", "reviewer", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "set role")
	r := ok(t, runModels(t, base, "show"), "models show")
	if !strings.Contains(r.stdout, "reviewer client=opencode model=openai/gpt-5.6-sol effort=inherit (override)") {
		t.Errorf("override not reported:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "code-search client=claude model=sonnet effort=inherit (preset:balanced)") {
		t.Errorf("the override leaked into another role:\n%s", r.stdout)
	}
}

// TestModelsSetRejectsAnInvalidValue is the command wiring for a refused set: which values are refused is settled in internal/models.
func TestModelsSetRejectsAnInvalidValue(t *testing.T) {
	base := modelsBase(t)
	seed := `{"version": 1, "installed": {}, "files": [], "models": {"preset": "economy"}}` + "\n"
	writeMode(t, stateFile(base), seed, 0o644)
	if r := runModels(t, base, "set", "reviewer", "--client", "claude", "--model", "claudius"); r.code == 0 {
		t.Error("set exited 0")
	}
	if readFile(t, stateFile(base)) != seed {
		t.Error("a rejected set rewrote state.json")
	}
}

func TestModelsResetRemovesOverrideThenBlock(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "--preset", "economy"), "set preset")
	ok(t, runModels(t, base, "set", "reviewer", "--client", "claude", "--model", "opus", "--effort", "high"), "set role")
	ok(t, runModels(t, base, "reset", "reviewer"), "reset role")
	if r := ok(t, runModels(t, base, "show"), "show"); !strings.Contains(r.stdout, "reviewer client=claude model=sonnet effort=inherit (preset:economy)") {
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

// TestModelsResolveReportsTheAssignment covers what resolve is for: the client, model, effort, and source a parent maps onto a native subagent or a T3 Code delegate_task target.
func TestModelsResolveReportsTheAssignment(t *testing.T) {
	base := modelsBase(t)
	resolved := func(role string) map[string]string {
		t.Helper()
		r := ok(t, runModels(t, base, "resolve", role, "--json"), "resolve")
		var out map[string]string
		if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
			t.Fatalf("resolve output is not JSON: %v\n%s", err, r.stdout)
		}
		return out
	}
	if got := resolved("executor"); got["client"] != "codex" || got["model"] != "gpt-6-luna" || got["source"] != "preset:balanced" {
		t.Errorf("executor: %v", got)
	}
	// A role on the session's own client is reported the same way, because it is launched the same way.
	if got := resolved("reviewer"); got["client"] != "claude" || got["route"] != "" {
		t.Errorf("reviewer: %v", got)
	}
	if r := runModels(t, base, "resolve", "executor", "--session", "claude", "--json"); r.code == 0 {
		t.Error("resolve still accepted --session")
	}
	if r := ok(t, runModels(t, base, "resolve", "reviewer"), "resolve, plain"); !strings.Contains(r.stdout, "reviewer client=claude model=opus effort=low (preset:balanced)") || strings.Contains(r.stdout, "route=") {
		t.Errorf("plain resolve: %s", r.stdout)
	}
}

func TestInstallRendersTheResolvedAssignments(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runInstaller(t, base, "--skip-packages"), "install with the default preset")
	if !strings.Contains(r.stdout, "assignment: codex executor model=gpt-6-luna (preset:balanced)") {
		t.Errorf("install did not report the preset source:\n%s", r.stdout)
	}
	agent := func(client, role string) string {
		switch client {
		case "codex":
			return readFile(t, filepath.Join(base, "home", ".codex", "agents", role+".toml"))
		case "opencode":
			return readFile(t, filepath.Join(base, "home", ".config", "opencode", "agents", role+".md"))
		}
		return readFile(t, filepath.Join(base, "home", ".claude", "agents", role+".md"))
	}
	if !strings.Contains(agent("codex", "executor"), "\nmodel = \"gpt-6-luna\"\n") {
		t.Error("the Codex executor does not carry the balanced model")
	}
	if !strings.Contains(agent("claude", "executor"), "model: inherit\n") {
		t.Error("the Claude executor, assigned to Codex, does not keep the client default")
	}
	if strings.Contains(agent("opencode", "executor"), "model:") {
		t.Error("the OpenCode executor, assigned to Codex, carries a model instead of the client default")
	}
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set performance")
	ok(t, runInstaller(t, base, "--skip-packages"), "install with performance")
	if r := ok(t, runModels(t, base, "show"), "show after install"); !strings.Contains(r.stdout, "(preset:performance)") {
		t.Errorf("install dropped the model choice:\n%s", r.stdout)
	}
	if a := agent("claude", "reviewer"); !strings.Contains(a, "model: fable\n") || !strings.Contains(a, "effort: low\n") {
		t.Errorf("claude reviewer lacks fable at low effort:\n%s", a)
	}
	if strings.Contains(agent("opencode", "reviewer"), "model:") {
		t.Error("opencode reviewer, assigned to Claude, carries a model instead of the client default")
	}
}

// TestModelsSetBeforeTheFirstInstall keeps a choice made on a never-installed machine, and installs over it.
func TestModelsSetBeforeTheFirstInstall(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "--preset", "performance"), "set before install")
	ok(t, runInstaller(t, base, "--skip-packages"), "first install over a models-only state")
	if r := ok(t, runModels(t, base, "show"), "show"); !strings.Contains(r.stdout, "(preset:performance)") {
		t.Errorf("the first install dropped the choice:\n%s", r.stdout)
	}
	ok(t, runModels(t, base, "reset"), "reset all")
	ok(t, runInstaller(t, base, "--skip-packages"), "install over a reset state")
}

// TestModelsRejectsAStaleChoice keeps the installer refusing a stored choice it cannot resolve, and naming the reset that clears it. Which stored blocks are stale is settled in internal/models.
func TestModelsRejectsAStaleChoice(t *testing.T) {
	base := modelsBase(t)
	writeMode(t, stateFile(base), `{"version": 1, "installed": {}, "files": [], "models": {"preset": "lavish"}}`+"\n", 0o644)
	if r := runInstaller(t, base, "--skip-packages"); r.code == 0 || !strings.Contains(r.stderr, "harness models reset") {
		t.Errorf("install over a stale choice: exit %d, stderr %q", r.code, r.stderr)
	}
	ok(t, runModels(t, base, "reset"), "the recommended reset")
	ok(t, runInstaller(t, base, "--skip-packages"), "install after reset")

	// A per-role reset drops only that role, leaving the preset to answer for it.
	writeMode(t, stateFile(base), `{"version": 1, "installed": {}, "files": [], "models": {"preset": "economy", "roles": {"executor": {"client": "claude", "model": "claudius"}}}}`+"\n", 0o644)
	ok(t, runModels(t, base, "reset", "executor"), "reset the malformed role")
	if r := ok(t, runModels(t, base, "show"), "show after role reset"); !strings.Contains(r.stdout, "executor client=codex model=gpt-5.6-luna effort=inherit (preset:economy)") {
		t.Errorf("role reset lost the preset:\n%s", r.stdout)
	}
}

// TestModelsPresetNeedsNoCatalog lets any built-in preset be chosen while Codex cannot list its models, and show marks what install will fall back on.
func TestModelsPresetNeedsNoCatalog(t *testing.T) {
	base := sandbox(t)
	writeShim(t, base, "codex", `case "$*" in "debug models") exit 1 ;; esac`)
	ok(t, runModels(t, base, "set", "--preset", "balanced"), "set balanced without a Codex catalog")
	if r := ok(t, runModels(t, base, "show"), "show"); !strings.Contains(r.stdout, "executor client=codex model=gpt-6-luna effort=inherit (preset:balanced) [unverified:") {
		t.Errorf("show does not mark the unverifiable Codex model:\n%s", r.stdout)
	}
	r := ok(t, runInstaller(t, base, "--skip-packages"), "install without a Codex catalog")
	if got := strings.Count(r.stderr+r.stdout, "listing Codex models failed"); got != 1 {
		t.Errorf("want one listing-failure warning, got %d:\n%s%s", got, r.stdout, r.stderr)
	}
}

// TestInstallFallsBackForAnUnlistedModel keeps the client default and warns when this machine cannot run the assigned Codex or OpenCode model.
func TestInstallFallsBackForAnUnlistedModel(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "reviewer", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "assign the reviewer to OpenCode")
	writeShim(t, base, "opencode", `case "$1" in models) echo 'opencode-go/glm-5.3' ;; esac`)
	writeShim(t, base, "codex", `case "$*" in "debug models") echo '{"models":[{"slug":"gpt-6.1-sol","visibility":"list"}]}' ;; esac`)
	r := ok(t, runInstaller(t, base, "--skip-packages"), "install without the assigned models")
	for _, want := range []string{"executor is assigned codex model gpt-6-luna, which this machine does not list", "reviewer is assigned opencode model openai/gpt-5.6-sol, which this machine does not list"} {
		if !strings.Contains(r.stderr+r.stdout, want) {
			t.Errorf("no fallback warning %q:\n%s%s", want, r.stdout, r.stderr)
		}
	}
	if strings.Contains(readFile(t, filepath.Join(base, "home", ".codex", "agents", "executor.toml")), "model = ") {
		t.Error("the Codex executor carries a model this machine cannot run")
	}
	if strings.Contains(readFile(t, filepath.Join(base, "home", ".config", "opencode", "agents", "reviewer.md")), "model:") {
		t.Error("the OpenCode reviewer carries a model this machine cannot run")
	}
}

// TestInstallRefusesATrackedPerRoleAssignment keeps the machine choice the only per-role source.
func TestInstallRefusesATrackedPerRoleAssignment(t *testing.T) {
	base := modelsBase(t)
	path := filepath.Join(base, "checkout", "clients", "assignments.json")
	var config map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &config); err != nil {
		t.Fatal(err)
	}
	config["roles"].(map[string]any)["reviewer"].(map[string]any)["opencode"] = map[string]any{"model": "openai/gpt-5.6-sol"}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeMode(t, path, string(data)+"\n", 0o644)
	if r := runInstaller(t, base, "--skip-packages"); r.code == 0 || !strings.Contains(r.stdout+r.stderr, "chosen with `harness models`") {
		t.Errorf("exit %d, output %q%q", r.code, r.stdout, r.stderr)
	}
}

// TestEveryAssignedRoleIsDispatchable ties rendered roles to the CLI registry and preset assignments.
func TestEveryAssignedRoleIsDispatchable(t *testing.T) {
	var config struct {
		Roles map[string]struct{} `json:"roles"`
	}
	raw := readFile(t, filepath.Join("..", "..", "clients", "assignments.json"))
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	for role := range config.Roles {
		if !slices.Contains(models.Roles, role) {
			t.Errorf("clients/assignments.json assigns %q but models.Roles omits it, so the role cannot be resolved", role)
		}
		for preset, table := range models.Presets {
			if _, ok := table[role]; !ok {
				t.Errorf("preset %q has no assignment for %q, so Resolve yields an empty client", preset, role)
			}
		}
	}
}
