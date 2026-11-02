package models

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleGuard(t *testing.T) {
	for _, executable := range []string{"orca", "orca-dev", "orca-ide", "/opt/Orca CLI/orca"} {
		t.Run(executable, func(t *testing.T) {
			t.Setenv("HARNESS_ORCA_COMMAND", executable)
			prefix := shellQuote(executable) + " orchestration "
			for _, tc := range []struct {
				command string
				allow   bool
			}{
				{prefix + `send --type worker_done --body "Checked the diff. No findings. Nothing remains." --outcome succeeded`, true},
				{prefix + `send --type heartbeat --phase reviewing --dispatch-capability abc/DEF+=`, true},
				{prefix + `ask --question 'Which option should I use' --timeout-ms 1000`, true},
				{prefix + `check --terminal term-1 --json`, true},
				{prefix + `send --type worker_done --subject "Bugs review: 1 bug, 3 q" --body "Found a bug in run.ts (line 12); the retry loop never exits? See #4 & [docs]! Done." --outcome succeeded`, true},
				{prefix + `send --body 'Literal $HOME and ` + "`x`" + ` stay text in single quotes.'`, true},
				{prefix + `send --body "Cost is $5"`, false},
				{prefix + "send --body \"`touch tracked.md`\"", false},
				{prefix + `send --body "a \" ; touch tracked.md"`, false},
				{prefix + `send --body "unterminated`, false},
				{prefix + `send --body x"; touch tracked.md"; check`, false},
				{`git commit -m x`, false},
				{prefix + `worker-start --spec x`, false},
				{prefix + `send --body x > tracked.md`, false},
				{prefix + `check; touch tracked.md`, false},
				{prefix + `check && touch tracked.md`, false},
				{prefix + `send --body "$(touch tracked.md)"`, false},
				{prefix + "check\ntouch tracked.md", false},
				{prefix + "send --body `touch tracked.md`", false},
				{prefix + `send --body x | sh`, false},
				{prefix + `check < private.txt`, false},
				{prefix + `check $(echo --json)`, false},
				{prefix + `check \
--json`, false},
				{`other-orca orchestration check`, false},
			} {
				input, _ := json.Marshal(map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": tc.command}})
				var output bytes.Buffer
				if code := LifecycleGuard(bytes.NewReader(input), &output); code != 0 {
					t.Fatal(code)
				}
				want := `"permissionDecision":"deny"`
				if tc.allow {
					want = `"permissionDecision":"allow"`
				}
				if !strings.Contains(output.String(), want) {
					t.Errorf("%s => %s, want %s", tc.command, output.String(), want)
				}
			}
		})
	}
	t.Setenv("HARNESS_ORCA_COMMAND", "")
	if lifecycleCommand("orca orchestration check", "") {
		t.Fatal("missing scope accepted")
	}
	var output bytes.Buffer
	LifecycleGuard(strings.NewReader("malformed"), &output)
	if !strings.Contains(output.String(), `"permissionDecision":"deny"`) {
		t.Fatal(output.String())
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// The worker preamble spells the executable as Orca prints it, which need not match the coordinator's spelling; both resolve to one file.
func TestLifecycleGuardMatchesResolvedExecutable(t *testing.T) {
	dir := t.TempDir()
	orca := filepath.Join(dir, "orca")
	if err := os.WriteFile(orca, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !lifecycleCommand("orca orchestration check", orca) {
		t.Error("PATH spelling of the configured absolute path refused")
	}
	if !lifecycleCommand(orca+" orchestration check", "orca") {
		t.Error("absolute spelling of the configured PATH name refused")
	}
	t.Chdir(filepath.Dir(dir))
	relative := "./" + filepath.Join(filepath.Base(dir), "orca")
	if !lifecycleCommand(relative+" orchestration check", orca) {
		t.Error("relative spelling of the configured absolute path refused")
	}
	if !lifecycleCommand(orca+" orchestration check", relative) {
		t.Error("absolute spelling of the configured relative path refused")
	}
	if lifecycleCommand("other orchestration check", orca) {
		t.Error("a different executable accepted")
	}
}

// A client name in --orca-command would leave the guard allowing nothing, so preparation refuses it.
func TestValidateOrcaCommandRefusesNonOrca(t *testing.T) {
	t.Setenv("ORCA_CLI_COMMAND", "/mnt/wsl/orca-wrapper.sh")
	for _, ok := range []string{"orca", "orca-dev", "orca-ide", "/Applications/Orca.app/Contents/Resources/bin/orca", "/mnt/wsl/orca-wrapper.sh"} {
		if err := validateOrcaCommand(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"claude", "codex", "opencode", "/usr/bin/claude", ""} {
		if validateOrcaCommand(bad) == nil {
			t.Errorf("%s accepted", bad)
		}
	}
}
