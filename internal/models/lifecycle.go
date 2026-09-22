package models

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// LifecycleGuard is a client permission hook. It never executes the requested command.
func LifecycleGuard(input io.Reader, output io.Writer) int {
	var request struct {
		Tool  string `json:"tool_name"`
		Input struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	decision := "deny"
	if err := json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&request); err == nil && request.Tool == "Bash" && lifecycleCommand(request.Input.Command, os.Getenv("HARNESS_ORCA_COMMAND")) {
		decision = "allow"
	}
	err := json.NewEncoder(output).Encode(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName": "PreToolUse", "permissionDecision": decision,
		"permissionDecisionReason": "Read-only roles may run only literal Orca send, ask, and check commands from their live worker preamble; shell syntax is prohibited.",
	}})
	if err != nil {
		return 2
	}
	return 0
}

func lifecycleCommand(command, executable string) bool {
	if executable == "" {
		return false
	}
	// A deliberately small literal-command grammar: even quoted shell syntax is refused.
	// This avoids interpreting shell programs in a permission hook.
	if strings.ContainsAny(command, "\\`$;&|<>(){}*?!#~[]") || strings.ContainsFunc(command, unicode.IsControl) {
		return false
	}
	prefixes := []string{shellQuote(executable), `"` + executable + `"`}
	if !strings.ContainsAny(executable, " \t'\"") {
		prefixes = append(prefixes, executable)
	}
	for _, prefix := range prefixes {
		for _, verb := range []string{"send", "ask", "check"} {
			start := prefix + " orchestration " + verb
			if command == start || strings.HasPrefix(command, start+" ") {
				return true
			}
		}
	}
	return false
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func validateOrcaCommand(executable string) error {
	if executable == "" || strings.ContainsAny(executable, "\\`$;&|<>(){}*?!#~[]'\"") || strings.ContainsFunc(executable, unicode.IsControl) {
		return fmt.Errorf("launch-args requires --orca-command with the exact literal executable selected by the orchestration skill")
	}
	return nil
}
