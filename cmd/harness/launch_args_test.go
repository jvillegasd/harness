package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/agents"
)

// renderAgent writes the frontmatter `harness install` renders for a role agent on one client.
func renderAgent(t *testing.T, base, client, role, model, effort string) {
	t.Helper()
	dir, effortKey := filepath.Join(base, "home", ".claude", "agents"), "effort: "
	if client == "opencode" {
		dir, effortKey = filepath.Join(base, "home", ".config", "opencode", "agents"), "reasoningEffort: "
	}
	generated := filepath.Join(base, "generated")
	if code := agents.Generate(filepath.Join("..", "..", "clients", "assignments.json"), generated, nil, io.Discard, io.Discard); code != 0 {
		t.Fatal("generate role fixture")
	}
	if client == "codex" {
		raw := readFile(t, filepath.Join(generated, client, role+".toml"))
		settings := "model = \"" + model + "\"\n"
		if effort != "" {
			settings += "model_reasoning_effort = \"" + effort + "\"\n"
		}
		raw = strings.Replace(raw, "developer_instructions = ", settings+"developer_instructions = ", 1)
		writeMode(t, filepath.Join(base, "home", ".codex", "agents", role+".toml"), raw, 0o644)
		return
	}
	raw := readFile(t, filepath.Join(generated, client, role+".md"))
	if client == "claude" {
		raw = strings.Replace(raw, "model: inherit\n", "model: "+model+"\n", 1)
	} else {
		raw = strings.Replace(raw, "mode: all\n", "mode: all\nmodel: "+model+"\n", 1)
	}
	if effort != "" {
		raw = strings.Replace(raw, "---\n", "---\n"+effortKey+effort+"\n", 1)
	}
	writeMode(t, filepath.Join(dir, role+".md"), raw, 0o644)
}

func TestLaunchArgsPreparesWithoutLaunching(t *testing.T) {
	for _, tc := range []struct {
		role, client, model, effort string
		want                        []string
	}{
		{"reviewer", "claude", "opus", "low", []string{"claude", "--agent", "reviewer"}},
		{"code-search", "claude", "sonnet", "", []string{"claude", "--agent", "code-search"}},
		{"executor", "codex", "gpt-6-luna", "", []string{"codex", "-m", "gpt-6-luna", "-c"}},
		{"executor", "opencode", "opencode-go/glm-5.3", "", []string{"opencode", "-m", "opencode-go/glm-5.3", "--agent", "executor"}},
	} {
		t.Run(tc.role+"-"+tc.client, func(t *testing.T) {
			base := modelsBase(t)
			for _, executable := range []string{"orca", "orca-dev", "orca-ide"} {
				writeShim(t, base, executable, "exit 1")
			}
			if tc.client == "opencode" {
				ok(t, runModels(t, base, "set", tc.role, "--client", tc.client, "--model", tc.model), "assign the role")
				if err := os.Remove(filepath.Join(base, "shims.log")); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(stateFile(base))
			renderAgent(t, base, tc.client, tc.role, tc.model, tc.effort)
			r := ok(t, runModels(t, base, "launch-args", tc.role, "--orca-command", "orca"), "prepare launch")
			var got []string
			if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
				t.Fatal(err)
			}
			if tc.client == "codex" {
				// The body, the approval policy, and the sandbox follow; internal/models pins their spelling.
				if !strings.HasPrefix(got[4], "developer_instructions=\"# ") || !slices.Contains(got, "workspace-write") || !slices.Contains(got, "sandbox_workspace_write.network_access=true") {
					t.Fatalf("codex launch lacks its instructions or sandbox: %v", got)
				}
				got = got[:4]
			}
			if tc.client == "claude" {
				if got[0] != "env" || !slices.Contains(got, "HARNESS_ORCA_COMMAND=orca") || !slices.Contains(got, "--permission-mode") || !slices.Contains(got, "dontAsk") {
					t.Fatalf("unguarded launch: %v", got)
				}
				i := slices.Index(got, "claude")
				got = got[i : i+3]
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv = %v, want %v", got, tc.want)
			}
			raw, err := os.ReadFile(filepath.Join(base, "shims.log"))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
				if line != "" && !strings.HasPrefix(line, "opencode models ") && !strings.HasPrefix(line, "codex debug models ") && !strings.HasPrefix(line, "CODEX_HOME=") {
					t.Errorf("preparation ran a runtime command: %s", line)
				}
			}
			if after, _ := os.ReadFile(stateFile(base)); string(after) != string(before) {
				t.Errorf("preparation wrote machine state:\n%s", after)
			}
		})
	}
}

func TestLaunchArgsRefusesMissingOrStaleAgent(t *testing.T) {
	for _, tc := range []struct{ name, client, role, model, effort string }{
		{"missing", "claude", "reviewer", "", ""},
		{"model", "claude", "reviewer", "opus", ""},
		{"effort", "claude", "reviewer", "inherit", "high"},
		{"mode", "opencode", "executor", "opencode-go/glm-5.3", ""},
		{"codex missing", "codex", "executor", "", ""},
		{"codex model", "codex", "executor", "gpt-6.1-sol", ""},
		{"codex effort", "codex", "executor", "gpt-6-luna", "high"},
		{"codex widened", "codex", "executor", "gpt-6-luna", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := modelsBase(t)
			if tc.client == "opencode" {
				ok(t, runModels(t, base, "set", tc.role, "--client", tc.client, "--model", tc.model), "assign the role")
			}
			if tc.model != "" {
				renderAgent(t, base, tc.client, tc.role, tc.model, tc.effort)
			}
			if tc.name == "codex widened" {
				path := filepath.Join(base, "home", ".codex", "agents", tc.role+".toml")
				writeMode(t, path, readFile(t, path)+"\n[features]\nmulti_agent = true\n", 0o644)
			}
			if tc.name == "mode" {
				path := filepath.Join(base, "home", ".config", "opencode", "agents", tc.role+".md")
				writeMode(t, path, strings.ReplaceAll(readFile(t, path), "mode: all", "mode: subagent"), 0o644)
			}
			r := runModels(t, base, "launch-args", tc.role, "--orca-command", "orca")
			if r.code == 0 || r.stdout != "" || !strings.Contains(r.stderr, "human must run harness install") {
				t.Fatalf("unsafe preparation: %#v", r)
			}
		})
	}
}

func TestOpenCodeLifecyclePluginBlocksGeneralShell(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is needed to exercise the OpenCode plugin")
	}
	plugin, err := filepath.Abs("../../skills/review/scripts/orca-lifecycle.mjs")
	if err != nil {
		t.Fatal(err)
	}
	script := `
 const {OrcaLifecycle} = await import(process.env.TEST_PLUGIN);
 const hooks = await OrcaLifecycle();
 const config = {agent: {'reviewer': {permission: {'*': 'deny', read: 'allow'}}}};
 await hooks.config(config);
 if (config.agent['reviewer'].permission.bash !== 'allow' || config.agent['reviewer'].permission['*'] !== 'deny') throw Error('bad scope');
 const before = hooks['tool.execute.before'];
 await before({tool:'bash'}, {args:{command:'orca orchestration check --terminal t --json'}});
 for (const command of ['git status', 'orca orchestration check > file', 'orca orchestration check; touch file']) {
   let denied = false;
   try { await before({tool:'bash'}, {args:{command}}); } catch { denied = true; }
   if (!denied) throw Error('allowed: '+command);
 }
 process.env.HARNESS_LIFECYCLE_GUARD = '/missing/guard';
 let denied = false;
 try { await before({tool:'bash'}, {args:{command:'orca orchestration check'}}); } catch { denied = true; }
 if (!denied) throw Error('missing guard did not block');
 `
	cmd := exec.Command(node, "--input-type=module", "-e", script)
	cmd.Env = append(os.Environ(), "TEST_PLUGIN="+plugin, "HARNESS_ORCA_COMMAND=orca", "HARNESS_LIFECYCLE_GUARD="+binary, "HARNESS_LIFECYCLE_ROLE=reviewer")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin: %v\n%s", err, output)
	}
}

func TestClaudeLifecycleHookFailsClosed(t *testing.T) {
	base := modelsBase(t)
	renderAgent(t, base, "claude", "reviewer", "inherit", "")
	role := readFile(t, filepath.Join(base, "home", ".claude", "agents", "reviewer.md"))
	var hook string
	for _, line := range strings.Split(role, "\n") {
		if strings.HasPrefix(line, "          command: '") {
			hook = strings.TrimSuffix(strings.TrimPrefix(line, "          command: '"), "'")
		}
	}
	if hook == "" {
		t.Fatal("missing hook")
	}
	for _, tc := range []struct {
		command, guard, decision string
		fails                    bool
	}{
		{"orca orchestration check --json", binary, "allow", false},
		{"orca orchestration check > tracked.md", binary, "deny", false},
		{"git status", binary, "deny", false},
		{"orca orchestration check", "/missing/guard", "", true},
		{"orca orchestration check", "", "", true},
	} {
		cmd := exec.Command("/bin/sh", "-c", hook)
		cmd.Env = []string{"HARNESS_LIFECYCLE_GUARD=" + tc.guard, "HARNESS_ORCA_COMMAND=orca"}
		input, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": tc.command}})
		cmd.Stdin = strings.NewReader(string(input))
		output, err := cmd.CombinedOutput()
		if tc.fails {
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
				t.Fatalf("missing guard: %v %s", err, output)
			}
			continue
		}
		if err != nil || !strings.Contains(string(output), `"permissionDecision":"`+tc.decision+`"`) {
			t.Fatalf("hook result: %v %s", err, output)
		}
	}
}

func TestOpenCodeLaunchPreparesBlockingPlugin(t *testing.T) {
	base := modelsBase(t)
	ok(t, runModels(t, base, "set", "reviewer", "--client", "opencode", "--model", "openai/gpt-5.6-sol"), "set role")
	renderAgent(t, base, "opencode", "reviewer", "openai/gpt-5.6-sol", "")
	if r := runModels(t, base, "launch-args", "reviewer", "--orca-command", "orca"); r.code == 0 {
		t.Fatal("missing plugin accepted")
	}
	plugin := filepath.Join(base, "home", ".agents", "skills", "review", "scripts", "orca-lifecycle.mjs")
	writeMode(t, plugin, readFile(t, "../../skills/review/scripts/orca-lifecycle.mjs"), 0o644)
	r := ok(t, runModels(t, base, "launch-args", "reviewer", "--orca-command", "orca"), "prepare")
	var argv []string
	if err := json.Unmarshal([]byte(r.stdout), &argv); err != nil {
		t.Fatal(err)
	}
	for _, arg := range argv {
		if raw, ok := strings.CutPrefix(arg, "OPENCODE_CONFIG_CONTENT="); ok {
			var config map[string]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &config); err != nil {
				t.Fatal(err)
			}
			if _, present := config["agent"]; present {
				t.Fatal("Bash granted before plugin loaded")
			}
			if !strings.Contains(string(config["plugin"]), "orca-lifecycle.mjs") {
				t.Fatal(raw)
			}
			return
		}
	}
	t.Fatal("plugin absent from launch")
}
