package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

var mustGuard = map[string]bool{
	"land": true, "setup-harness": true, "build": true, "jvillegasd-mode": true,
	"epic": true,
	"cleanup": true,
}

func Unguarded(c checkout.Checkout) ([]string, error) {
	lf, err := readLock(c.Lock())
	if err != nil {
		return nil, fmt.Errorf("reading the lock that separates local skills from locked ones: %w", err)
	}
	skills, err := c.SkillDirs()
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, sk := range skills {
		dir := sk.Dir
		if lf.skills.has(sk.Name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, sk.Name+": missing SKILL.md")
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: unreadable SKILL.md (%v)", sk.Name, err))
			continue
		}
		claude, opencode := guarded(string(data))
		sidecar, err := os.ReadFile(filepath.Join(dir, "agents", "openai.yaml"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("%s: unreadable agents/openai.yaml (%v)", sk.Name, err))
			continue
		}
		codex := codexGuarded(string(sidecar))
		if !mustGuard[sk.Name] && !claude && !opencode && !codex {
			continue
		}
		if !claude {
			problems = append(problems, sk.Name+": missing disable-model-invocation")
		}
		if !opencode {
			problems = append(problems, sk.Name+": missing opencode/autoinvoke")
		}
		if !codex {
			problems = append(problems, sk.Name+": missing agents/openai.yaml allow_implicit_invocation")
		}
	}
	return problems, nil
}

func guarded(text string) (claude, opencode bool) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return false, false
	}
	inMetadata, childIndent := false, ""
	for _, line := range lines[1:] {
		if line == "---" {
			return claude, opencode
		}
		topLevel := line != "" && line[0] != ' ' && line[0] != '\t'
		if topLevel {
			inMetadata = line == "metadata:"
			if inMetadata {

				opencode, childIndent = false, ""
			}
			if strings.HasPrefix(line, "disable-model-invocation:") {
				claude = line == "disable-model-invocation: true"
			}
			continue
		}
		if !inMetadata || strings.TrimSpace(line) == "" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if childIndent == "" {
			childIndent = indent
		}
		if indent == childIndent && strings.HasPrefix(line[len(indent):], "opencode/autoinvoke:") {
			opencode = line[len(indent):] == `opencode/autoinvoke: "false"`
		}
	}

	return false, false
}

func codexGuarded(text string) bool {
	guard, inPolicy, childIndent, policies := false, false, "", 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			inPolicy = line == "policy:"
			if inPolicy {
				policies++
			}
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if strings.Contains(indent, "\t") {
			return false
		}
		if !inPolicy {
			continue
		}
		if childIndent == "" {
			childIndent = indent
		}
		if indent == childIndent && strings.HasPrefix(line[len(indent):], "allow_implicit_invocation:") {
			guard = line[len(indent):] == "allow_implicit_invocation: false"
		}
	}
	return guard && policies == 1
}
