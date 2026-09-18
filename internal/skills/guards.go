package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/jvillegasd/harness/internal/checkout"
)

// guards are the two frontmatter lines that keep a locally authored skill from firing on a description match, one per client.
var guards = [][2]string{
	{"disable-model-invocation", "disable-model-invocation: true"},
	{"opencode/autoinvoke", `opencode/autoinvoke: "false"`},
}

// Unguarded reports every skill absent from the lock whose SKILL.md frontmatter lacks a guard, as "<name>: missing <key>". Locked bodies are exempt, because their frontmatter is upstream content.
func Unguarded(c checkout.Checkout) ([]string, error) {
	lf, err := readLock(c.Lock())
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(c.Skills())
	if err != nil {
		return nil, err
	}
	var problems []string
	for _, e := range entries {
		if !e.IsDir() || lf.skills.has(e.Name()) {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(c.Skills(), e.Name(), "SKILL.md"))
		head := frontmatter(string(data))
		for _, g := range guards {
			if !strings.Contains(head, g[1]) {
				problems = append(problems, e.Name()+": missing "+g[0])
			}
		}
	}
	return problems, nil
}

// frontmatter returns the text between a leading "---" line and the next one, or "" when there is none.
func frontmatter(text string) string {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	head, _, _ := strings.Cut(rest, "\n---")
	return head
}
