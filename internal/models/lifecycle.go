package models

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
		"permissionDecisionReason": "Read-only roles may run only Orca send, ask, and check commands from their live worker preamble, on one line, with shell syntax only inside quotes and no $, backtick, or backslash inside double quotes.",
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
	words, ok := literalWords(command)
	if !ok || len(words) < 3 || words[1] != "orchestration" || !slices.Contains([]string{"send", "ask", "check"}, words[2]) {
		return false
	}
	return sameExecutable(words[0], executable)
}

// literalWords splits a command the way a shell would only when the shell would do nothing but split it: outside quotes every metacharacter is refused, single quotes are fully literal, and double quotes refuse the three characters that still expand there. Message prose can then carry parentheses, semicolons, and question marks without a permission hook ever interpreting a shell program.
func literalWords(command string) ([]string, bool) {
	if strings.ContainsFunc(command, unicode.IsControl) {
		return nil, false
	}
	var words []string
	var word strings.Builder
	inWord := false
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case c == ' ':
			if inWord {
				words, inWord = append(words, word.String()), false
				word.Reset()
			}
			continue
		case c == '\'' || c == '"':
			end := strings.IndexByte(command[i+1:], c)
			if end < 0 {
				return nil, false
			}
			quoted := command[i+1 : i+1+end]
			if c == '"' && strings.ContainsAny(quoted, "$`\\") {
				return nil, false
			}
			word.WriteString(quoted)
			i += end + 1
		case strings.IndexByte("\\`$;&|<>(){}*?!#~[]", c) >= 0:
			return nil, false
		default:
			word.WriteByte(c)
		}
		inWord = true
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, true
}

// sameExecutable accepts the configured spelling, or another spelling the shell resolves to the same file, so `orca` and its absolute path are one executable.
func sameExecutable(word, executable string) bool {
	if word == executable {
		return true
	}
	a, b := resolveExecutable(word), resolveExecutable(executable)
	return a != "" && a == b
}

func resolveExecutable(name string) string {
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	if path, err = filepath.EvalSymlinks(path); err != nil {
		return ""
	}
	return path
}

// validateOrcaCommand refuses a value that cannot be the Orca CLI, such as a client name, because the guard would then allow no lifecycle command at all. Orca's own spellings start with orca; ORCA_CLI_COMMAND is the one arbitrary path Orca exports.
func validateOrcaCommand(executable string) error {
	if executable == "" || strings.ContainsAny(executable, "\\`$;&|<>(){}*?!#~[]'\"") || strings.ContainsFunc(executable, unicode.IsControl) {
		return fmt.Errorf("launch-args requires --orca-command with the exact literal executable selected by the orchestration skill")
	}
	if !strings.HasPrefix(filepath.Base(executable), "orca") && executable != os.Getenv("ORCA_CLI_COMMAND") {
		return fmt.Errorf("--orca-command %q is not the Orca CLI; pass the executable the orchestration skill selected (orca, orca-dev, orca-ide, or ORCA_CLI_COMMAND), not the role's client", executable)
	}
	return nil
}
