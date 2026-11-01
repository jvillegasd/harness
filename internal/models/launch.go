package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jvillegasd/harness/internal/agents"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// executorDenies keeps publication and worktree cleanup with the parent. Orca owns delegated placement, so its `worktree rm`, the spelling the installed Orca CLI's own help gives, is denied under the three literal executable spellings; a command taken from ORCA_CLI_COMMAND is an arbitrary path no prefix can match. `wt` stays denied because retiring Worktrunk leaves an installed one on the machine.
var executorDenies = []string{"Bash(git commit:*)", "Bash(git push:*)", "Bash(git merge:*)", "Bash(git worktree remove:*)", "Bash(orca worktree rm:*)", "Bash(orca-ide worktree rm:*)", "Bash(orca-dev worktree rm:*)", "Bash(wt remove:*)", "Bash(wt merge:*)", "Bash(wt step commit:*)", "Bash(gh:*)"}

// preparedArgs validates installed role configuration and returns client argv. It never launches a worker or calls Orca.
func preparedArgs(choice Choice, role, orca string) ([]string, error) {
	if err := knownRole(role); err != nil {
		return nil, err
	}
	if err := validateOrcaCommand(orca); err != nil {
		return nil, err
	}
	a := choice.Resolve()[role].Assignment
	if _, err := exec.LookPath(a.Client); err != nil {
		return nil, fmt.Errorf("%s is not on PATH", a.Client)
	}
	if err := Validate(a); err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if a.Client == "codex" {
		return codexArgs(home, role, a)
	}
	if err := renderedMatches(home, role, a); err != nil {
		return nil, err
	}
	argv := launchArgs(role, a)
	if role == "executor" {
		return argv, nil
	}
	guard, err := os.Executable()
	if err != nil {
		return nil, err
	}
	env := []string{"env", "HARNESS_ORCA_COMMAND=" + orca, "HARNESS_LIFECYCLE_GUARD=" + guard, "HARNESS_LIFECYCLE_ROLE=" + role}
	if a.Client == "claude" {
		argv = append(argv, "--permission-mode", "dontAsk")
	} else {
		if os.Getenv("OPENCODE_CONFIG_CONTENT") != "" {
			return nil, errors.New("launch preparation cannot replace existing OPENCODE_CONFIG_CONTENT; reconcile that runtime configuration first")
		}
		plugin := filepath.Join(home, ".agents", "skills", "review", "scripts", "orca-lifecycle.mjs")
		if _, err := os.Stat(plugin); err != nil {
			return nil, fmt.Errorf("missing lifecycle permission plugin; a human must run harness install")
		}
		config, _ := json.Marshal(map[string]any{
			"plugin": []string{(&url.URL{Scheme: "file", Path: plugin}).String()},
		})
		env = append(env, "OPENCODE_CONFIG_CONTENT="+string(config))
	}
	return append(env, argv...), nil
}

// renderedMatches refuses a role whose installed agent was rendered for another model or effort. Claude's model and both clients' effort come from frontmatter, so changing an assignment without reinstalling could otherwise run stale settings.
func renderedMatches(home, role string, a Assignment) error {
	path, effortKey := filepath.Join(home, ".claude", "agents", role+".md"), "effort: "
	if a.Client == "opencode" {
		config := os.Getenv("XDG_CONFIG_HOME")
		if config == "" {
			config = filepath.Join(home, ".config")
		}
		path, effortKey = filepath.Join(config, "opencode", "agents", role+".md"), "reasoningEffort: "
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.New("no rendered " + a.Client + " agent for " + role + "; a human must run harness install")
	}
	head := ""
	if parts := strings.SplitN(string(data), "---\n", 3); strings.HasPrefix(string(data), "---\n") && len(parts) == 3 {
		head = parts[1]
	}
	if role != "executor" && !strings.Contains(string(data), "Orca lifecycle") {
		return errors.New("the installed role lacks scoped Orca lifecycle access; a human must run harness install")
	}
	if role != "executor" && !agents.RestrictionsMatch(a.Client+"/"+role+".md", string(data), "read-search") {
		return errors.New("the installed role has missing or changed permission controls; a human must run harness install")
	}
	lines := strings.Split(head, "\n")
	effort := slices.IndexFunc(lines, func(l string) bool { return strings.HasPrefix(l, effortKey) })
	rendered := ""
	if effort >= 0 {
		rendered = strings.TrimPrefix(lines[effort], effortKey)
	}
	if !slices.Contains(lines, "model: "+a.Model) || rendered != a.Effort {
		return errors.New("the installed " + a.Client + " " + role + " agent was not rendered for " + a.Model + " at effort " + effortOrInherit(a.Effort) + "; a human must run harness install")
	}
	// A mode: subagent agent launched by --agent runs as OpenCode's default agent with none of its controls.
	if a.Client == "opencode" && !slices.Contains(lines, "mode: all") {
		return errors.New("the installed opencode " + role + " agent is not mode: all; a human must run harness install")
	}
	return nil
}

// launchArgs is the argv that starts the role as its own named agent on its client, inside the terminal Orca opens. The executor gets edits and Bash, minus the actions its parent owns.
func launchArgs(role string, a Assignment) []string {
	if a.Client == "opencode" {
		return []string{"opencode", "-m", a.Model, "--agent", role}
	}
	// No --model or --effort: they would override the agent's frontmatter, which renderedMatches proved current. Whether a main-thread --agent session applies the frontmatter's effort, not only its model, is unverified.
	argv := []string{"claude", "--agent", role}
	if role == "executor" {
		argv = append(append(argv, "--permission-mode", "acceptEdits", "--allowedTools", "Bash", "--disallowedTools"), executorDenies...)
	}
	return argv
}

// codexArgs launches a Codex role from its installed agent file, since Codex starts no session as a named agent: the file's instructions, model, and effort go in argv, which also keeps the launch independent of the CODEX_HOME Orca sets. Every role gets the workspace-write sandbox with network on, the narrowest one the Orca CLI can reach from; .git stays read-only there, so commits fail.
func codexArgs(home, role string, a Assignment) ([]string, error) {
	path := filepath.Join(home, ".codex", "agents", role+".toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("no rendered codex agent for " + role + "; a human must run harness install")
	}
	text := string(data)
	level := "read-search"
	if role == "executor" {
		level = "implement"
	}
	if !agents.RestrictionsMatch("codex/"+role+".toml", text, level) {
		return nil, errors.New("the installed role has missing or changed permission controls; a human must run harness install")
	}
	if role != "executor" && !strings.Contains(text, "Orca lifecycle") {
		return nil, errors.New("the installed role lacks scoped Orca lifecycle access; a human must run harness install")
	}
	model, effort := "", ""
	for _, line := range strings.Split(text, "\n") {
		if value, ok := strings.CutPrefix(line, "model = "); ok {
			model = strings.Trim(value, `"`)
		}
		if value, ok := strings.CutPrefix(line, "model_reasoning_effort = "); ok {
			effort = strings.Trim(value, `"`)
		}
		if strings.HasPrefix(line, "developer_instructions = ") {
			break
		}
	}
	if model == "" {
		model = "inherit"
	}
	if model != a.Model || effort != a.Effort {
		return nil, errors.New("the installed codex " + role + " agent was not rendered for " + a.Model + " at effort " + effortOrInherit(a.Effort) + "; a human must run harness install")
	}
	instructions, _ := agents.CodexInstructions(text)
	return codexLaunch(a, instructions), nil
}

// codexLaunch is the argv for a Codex role whose installed file was proven current. The instructions travel as a TOML basic string, which JSON's escapes are a subset of.
func codexLaunch(a Assignment, instructions string) []string {
	argv := []string{"codex"}
	if a.Model != "inherit" {
		argv = append(argv, "-m", a.Model)
	}
	if a.Effort != "" {
		argv = append(argv, "-c", `model_reasoning_effort="`+a.Effort+`"`)
	}
	var quoted strings.Builder
	enc := json.NewEncoder(&quoted)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(instructions)
	return append(argv, "-c", "developer_instructions="+strings.TrimSuffix(quoted.String(), "\n"),
		"--ask-for-approval", "never", "--sandbox", "workspace-write", "-c", "sandbox_workspace_write.network_access=true")
}
