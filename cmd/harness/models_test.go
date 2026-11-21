package main

import (
	"encoding/json"
	"fmt"
	"os"
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
	got := shown(t, base)
	for _, want := range []string{
		"standard client=codex model=gpt-6.1-terra (preset:balanced)",
		"executor tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"worker tier=standard client=codex model=gpt-6.1-terra effort=inherit (preset:balanced)",
		"reviewer tier=high client=claude model=opus effort=low (preset:balanced)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("show --json lacks %q:\n%s", want, got)
		}
	}
	if r := ok(t, runModels(t, base, "show"), "models show"); !strings.Contains(r.stdout, " STANDARD   codex  › gpt-6.1-terra") {
		t.Errorf("show does not draw the tier tree:\n%s", r.stdout)
	}
}

func TestModelsWithoutTTYPrintsUsage(t *testing.T) {
	r := runModels(t, modelsBase(t))
	if r.code == 0 {
		t.Fatalf("bare models without a TTY exited 0")
	}
	for _, want := range []string{"show ", "tier preset <name>", "tier set <tier>", "tier reset", "role set <role>", "role reset", "role resolve <role>"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("usage lacks %q:\n%s", want, r.stderr)
		}
	}
}

func TestModelsSetKeepsOtherStateKeysAndRejectsWithoutWriting(t *testing.T) {
	base := modelsBase(t)
	seed := `{"version": 1, "installed": {"claude": ["policy"]}, "files": []}` + "\n"
	writeMode(t, stateFile(base), seed, 0o644)
	if r := runModels(t, base, "role", "set", "reviewer", "--tier", "ultra"); r.code == 0 {
		t.Error("set accepted an unknown tier")
	}
	if readFile(t, stateFile(base)) != seed {
		t.Error("a rejected set rewrote state.json")
	}
	ok(t, runModels(t, base, "tier", "preset", "codex"), "set preset")
	ok(t, runModels(t, base, "role", "set", "code-search", "--tier", "high"), "set role")
	got := shown(t, base)
	for _, want := range []string{"reviewer tier=high client=codex model=gpt-6.1-sol effort=low (preset:codex)", "heavy client=codex model=unavailable (preset:codex) [codex lists no heavy (astra) model; run `codex debug models`: not listed; roles at heavy fall back to high]", "code-search tier=high client=codex model=gpt-6.1-sol effort=inherit (override)"} {
		if !strings.Contains(got, want) {
			t.Errorf("show lacks %q:\n%s", want, got)
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
	r := ok(t, runModels(t, base, "role", "resolve", "worker", "--tier", "heavy", "--json"), "resolve")
	var got map[string]string
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatalf("resolve output is not JSON: %v\n%s", err, r.stdout)
	}
	if got["role"] != "worker" || got["tier"] != "heavy" || got["client"] != "claude" || got["model"] != "opus" || got["effort"] != "inherit" || got["source"] != "launch" {
		t.Errorf("worker at heavy: %v", got)
	}
	if r := runModels(t, base, "role", "resolve", "reviewer", "--tier", "fast"); r.code == 0 {
		t.Error("resolve gave a non-generic role a launch tier")
	}
}

func TestInstallRendersTheResolvedTiers(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runInstaller(t, base, "agents"), "install with the default preset")
	if !strings.Contains(r.stdout, "codex     executor, worker on gpt-6.1-terra") || strings.Contains(r.stdout, "assignment:") {
		t.Errorf("install did not draw the codex agents' models, or kept the raw render report:\n%s", r.stdout)
	}
	if !strings.Contains(agentFile(t, base, "codex", "executor"), "\nmodel = \"gpt-6.1-terra\"\n") {
		t.Error("the Codex executor does not carry the standard tier's model")
	}
	if !strings.Contains(agentFile(t, base, "claude", "executor"), "model: inherit\n") {
		t.Error("the Claude executor, served by Codex, does not keep the client default")
	}
	if a := agentFile(t, base, "claude", "reviewer"); !strings.Contains(a, "model: opus\n") || !strings.Contains(a, "effort: low\n") {
		t.Errorf("claude reviewer lacks opus at low effort:\n%s", a)
	}
	if !strings.Contains(agentFile(t, base, "codex", "worker"), "\nmodel = \"gpt-6.1-terra\"\n") {
		t.Error("the Codex worker does not carry its default tier's model")
	}
	ok(t, runModels(t, base, "tier", "set", "high", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "serve high from OpenCode")
	ok(t, runInstaller(t, base, "agents"), "install with OpenCode serving high")
	if a := agentFile(t, base, "opencode", "review-bugs"); !strings.Contains(a, "model: openai/gpt-5.6-sol\n") || !strings.Contains(a, "reasoningEffort: low\n") {
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
	if r := runInstaller(t, base, "agents"); r.code == 0 || !strings.Contains(r.stderr, "harness models tier reset") {
		t.Errorf("install over a stale choice: exit %d, stderr %q", r.code, r.stderr)
	}
	ok(t, runModels(t, base, "tier", "reset"), "the recommended reset")
	ok(t, runInstaller(t, base, "agents"), "install after reset")
}

func TestModelsSetAppliesToTheInstalledAgentsOnly(t *testing.T) {
	base := modelsBase(t)
	ok(t, runInstaller(t, base, "agents", "--client", "claude"), "install Claude's agents alone")
	r := ok(t, runModels(t, base, "role", "set", "reviewer", "--tier", "fast"), "set without a reinstall")
	if !strings.Contains(r.stdout, "Applied to the role agents of claude.") || strings.Contains(r.stdout, "assignment:") {
		t.Errorf("set did not report a quiet apply:\n%s", r.stdout)
	}
	if a := agentFile(t, base, "claude", "reviewer"); !strings.Contains(a, "model: haiku\n") || strings.Contains(a, "effort:") {
		t.Errorf("the Claude reviewer does not carry the new tier:\n%s", a)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".codex", "agents")); err == nil {
		t.Error("set rendered agents for a client whose agents were never installed")
	}
	ok(t, runModels(t, base, "reset"), "reset")
	if a := agentFile(t, base, "claude", "reviewer"); !strings.Contains(a, "model: opus\n") {
		t.Errorf("reset did not reach the Claude reviewer:\n%s", a)
	}
	if r := ok(t, runModels(t, base, "show"), "show"); strings.Contains(r.stdout, "Applied") {
		t.Errorf("show applied the choice:\n%s", r.stdout)
	}
}

func TestModelsSetBeforeAnyInstallRendersNothing(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runModels(t, base, "role", "set", "reviewer", "--tier", "fast"), "set before install")
	if !strings.Contains(r.stdout, "`harness install agents` renders them") {
		t.Errorf("set does not say the agents wait for an install:\n%s", r.stdout)
	}
	if _, err := os.Stat(filepath.Join(base, "home", ".claude", "agents")); err == nil {
		t.Error("set created the Claude agents before any install")
	}
}

func TestModelsSetKeepsAnAgentEditedByHand(t *testing.T) {
	base := modelsBase(t)
	ok(t, runInstaller(t, base, "agents", "--client", "claude"), "install Claude's agents")
	path := filepath.Join(base, "home", ".claude", "agents", "reviewer.md")
	edited := readFile(t, path) + "\nMy own note.\n"
	writeMode(t, path, edited, 0o644)
	r := ok(t, runModels(t, base, "role", "set", "code-search", "--tier", "high"), "set with an edited agent")
	if readFile(t, path) != edited {
		t.Error("set replaced an agent edited by hand")
	}
	if !strings.Contains(r.stderr, path) || !strings.Contains(r.stderr, "changed since the harness wrote it") {
		t.Errorf("set did not name the kept agent:\n%s", r.stderr)
	}
	if !strings.Contains(agentFile(t, base, "claude", "code-search"), "model: opus\n") {
		t.Error("an edited agent stopped the others from applying")
	}
	ok(t, runModels(t, base, "role", "set", "code-search", "--tier", "fast"), "a second set over the same edit")
	if readFile(t, path) != edited {
		t.Error("a second set replaced the edited agent")
	}
}

// shown reads `models show --json` as one line per tier and role.
func shown(t *testing.T, base string) string {
	t.Helper()
	r := ok(t, runModels(t, base, "show", "--json"), "models show --json")
	var rep struct {
		Tiers []struct{ Tier, Client, Model, Source, Problem, Fallback string }
		Roles []struct{ Role, Tier, Client, Model, Effort, Source, Note, Problem string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("show --json: %v\n%s", err, r.stdout)
	}
	var b strings.Builder
	for _, v := range rep.Tiers {
		fmt.Fprintf(&b, "%s client=%s model=%s (%s)", v.Tier, v.Client, v.Model, v.Source)
		if v.Fallback != "" {
			fmt.Fprintf(&b, " [%s; roles at %s fall back to %s]", strings.TrimSuffix(v.Problem, "; install renders the client default"), v.Tier, v.Fallback)
		} else if v.Problem != "" {
			b.WriteString(" [" + v.Problem + "]")
		}
		b.WriteString("\n")
	}
	for _, v := range rep.Roles {
		fmt.Fprintf(&b, "%s tier=%s client=%s model=%s effort=%s (%s)\n", v.Role, v.Tier, v.Client, v.Model, v.Effort, v.Source)
	}
	return b.String()
}

func TestInstallGroupsItsLogBySection(t *testing.T) {
	base := modelsBase(t)
	r := ok(t, runInstaller(t, base, "agents"), "install agents")
	for _, want := range []string{"harness install  ", "\n TOOLS ", "\n AGENTS ", "  └─ ok "} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("install output lacks %q:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "\x1b[") || strings.Contains(r.stdout, "installed "+filepath.Join(base, "home")) {
		t.Errorf("install wrote color to a pipe, or listed files without --verbose:\n%s", r.stdout)
	}
	writeMode(t, filepath.Join(base, "home", ".claude", "agents", "worker.md"), "edited\n", 0o644)
	r = ok(t, runInstaller(t, base, "agents", "--no-backup", "--verbose"), "install agents --verbose")
	if !strings.Contains(r.stdout, "installed ~/.claude/agents/worker.md") {
		t.Errorf("--verbose did not list the rewritten agent:\n%s", r.stdout)
	}
}
