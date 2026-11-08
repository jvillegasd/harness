package models

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode"
)

// ReadGuard is the PreToolUse permission hook for read-search roles. It allows only the read-only commands readCommand accepts and never executes the requested command.
func ReadGuard(input io.Reader, output io.Writer) int {
	var request struct {
		Tool  string `json:"tool_name"`
		Cwd   string `json:"cwd"`
		Input struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	decision := "deny"
	if err := json.NewDecoder(io.LimitReader(input, 1<<20)).Decode(&request); err == nil && request.Tool == "Bash" {
		dir := request.Cwd
		if dir == "" {
			dir, _ = os.Getwd()
		}
		if readOnlyCommand(request.Input.Command, dir) {
			decision = "allow"
		}
	}
	err := json.NewEncoder(output).Encode(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName": "PreToolUse", "permissionDecision": decision,
		"permissionDecisionReason": "Read-only roles may run only cat, head, tail, wc, ls, find, rg, and read-only git subcommands on paths inside the working directory; one command per call, on one line, with shell syntax only inside quotes and no $, backtick, or backslash inside double quotes.",
	}})
	if err != nil {
		return 2
	}
	return 0
}

func readOnlyCommand(command, dir string) bool {
	words, ok := literalWords(command)
	return ok && len(words) > 0 && readCommand(words, dir)
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
