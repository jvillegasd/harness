package models

import (
	"encoding/json"
	"io"
	"os"
	"slices"
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

// secretGuardReason tells the agent why the call was refused and what stays open.
const secretGuardReason = "Env files hold secrets: no agent may read, search, edit, or name one in a command, except .env.example, .env.local, and .env.<mode>.local. Work from .env.example, or ask the human for the value."

// SecretGuard is the PreToolUse hook every Claude tool call passes through, the main session and every subagent alike. It denies a call that names a secret env file (envSecret) in a file tool's path or glob, or anywhere in a Bash command, and otherwise prints nothing, so the client's own permission rules still decide. Claude Code cannot express this as rules, since a deny always beats an allow. A Bash command is also checked with its quotes, backslashes, and backticks removed, since the shell joins quoted fragments back into the one name it opens. It decodes the whole input with no size cap, because a capped decode would deny every large Write to an ordinary file. It reads names only: a command that reaches a secret without naming it, such as a recursive search of a directory or a glob like .e*, is not caught.
func SecretGuard(input io.Reader, output io.Writer) int {
	var request struct {
		Tool  string         `json:"tool_name"`
		Input map[string]any `json:"tool_input"`
	}
	if err := json.NewDecoder(input).Decode(&request); err != nil || request.Tool == "" {
		return denySecret(output)
	}
	keys := []string{"file_path", "notebook_path", "path", "glob"}
	switch request.Tool {
	case "Bash":
		keys = []string{"command"}
	case "Glob":
		keys = append(keys, "pattern")
	}
	for _, key := range keys {
		text, _ := request.Input[key].(string)
		parts := nameParts(text)
		if request.Tool == "Bash" {
			parts = append(parts, nameParts(strings.NewReplacer(`'`, "", `"`, "", `\`, "", "`", "").Replace(text))...)
		}
		if slices.ContainsFunc(parts, envSecret) {
			return denySecret(output)
		}
	}
	return 0
}

// nameParts splits text at every character a file name in a path, a git revision, or a shell word does not carry, so each piece is one candidate name.
func nameParts(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("/:=\"'`$;&|<>(){},\\!#~", r)
	})
}

func denySecret(output io.Writer) int {
	err := json.NewEncoder(output).Encode(map[string]any{"hookSpecificOutput": map[string]string{
		"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": secretGuardReason,
	}})
	if err != nil {
		return 2
	}
	return 0
}
