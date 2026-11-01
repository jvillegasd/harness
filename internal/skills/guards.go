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

// mustGuard are the local skills only a human may start: merging, discarding work, provisioning a repository, a human-led interview, unattended building, which spends model budget without supervision, and retrospectives, which read session records and propose changes to the harness itself. Every other local skill may be loaded by the model when its description fits.
var mustGuard = map[string]bool{
	"land": true, "abandon": true, "setup-harness": true, "grill-with-docs": true,
	"build": true, "retro": true,
}

// Unguarded reports every skill absent from the lock whose invocation guard is required or partly present, as "<name>: missing <key>". A skill in mustGuard needs all three guards, and any other local skill carrying one needs the others, because a guard only one client reads fires in the rest. Claude Code reads `disable-model-invocation: true` at the top level, OpenCode reads `opencode/autoinvoke: "false"` under `metadata:`, and Codex reads `allow_implicit_invocation: false` under `policy:` in the skill's agents/openai.yaml, so a guard anywhere else does not count. Locked bodies are exempt, because their frontmatter is upstream content.
func Unguarded(c checkout.Checkout) ([]string, error) {
	lf, err := readLock(c.Lock())
	if err != nil {
		return nil, fmt.Errorf("reading the lock that separates local skills from locked ones: %w", err)
	}
	entries, err := os.ReadDir(c.Skills())
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, e := range entries {
		dir := filepath.Join(c.Skills(), e.Name())
		if lf.skills.has(e.Name()) {
			continue
		}
		info, err := os.Stat(dir)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: unreadable (%v)", e.Name(), err))
			continue
		}
		if !info.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		if errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, e.Name()+": missing SKILL.md")
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: unreadable SKILL.md (%v)", e.Name(), err))
			continue
		}
		claude, opencode := guarded(string(data))
		sidecar, err := os.ReadFile(filepath.Join(dir, "agents", "openai.yaml"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			problems = append(problems, fmt.Sprintf("%s: unreadable agents/openai.yaml (%v)", e.Name(), err))
			continue
		}
		codex := codexGuarded(string(sidecar))
		if !mustGuard[e.Name()] && !claude && !opencode && !codex {
			continue
		}
		if !claude {
			problems = append(problems, e.Name()+": missing disable-model-invocation")
		}
		if !opencode {
			problems = append(problems, e.Name()+": missing opencode/autoinvoke")
		}
		if !codex {
			problems = append(problems, e.Name()+": missing agents/openai.yaml allow_implicit_invocation")
		}
	}
	return problems, nil
}

// guarded reads the frontmatter line by line: the Claude guard must be a top-level line, and the OpenCode guard a direct child of the top-level `metadata:` block, at the indent of that block's first child.
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
				// A later metadata block replaces the earlier one, so its guard must be restated.
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
	// No closing delimiter: there is no frontmatter to trust.
	return false, false
}

// codexGuarded reads agents/openai.yaml, where the guard must be a direct child of the top-level `policy:` block, at the indent of that block's first child. A tab indent or a repeated policy key is not YAML a strict parser takes, so either one counts as unguarded.
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
