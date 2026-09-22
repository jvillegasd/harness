package models

import (
	"bytes"
	"encoding/json"
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
				{`git status`, false},
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
