package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

func TestGuardedCountsAGuardOnlyWhereItsClientReadsIt(t *testing.T) {
	both := "---\ndisable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\n---\n"
	for name, c := range map[string]struct {
		text             string
		claude, opencode bool
	}{
		"both where each client reads them": {both, true, true},
		"CRLF line endings":                 {strings.ReplaceAll(both, "\n", "\r\n"), true, true},
		"opencode key at top level":         {"---\ndisable-model-invocation: true\nopencode/autoinvoke: \"false\"\n---\n", true, false},
		"claude key under metadata":         {"---\nmetadata:\n  disable-model-invocation: true\n  opencode/autoinvoke: \"false\"\n---\n", false, true},
		"guards quoted in a value":          {"---\ndescription: \"disable-model-invocation: true, opencode/autoinvoke: \\\"false\\\"\"\n---\n", false, false},
		"later override to false":           {"---\ndisable-model-invocation: true\ndisable-model-invocation: false\nmetadata:\n  opencode/autoinvoke: \"false\"\n---\n", false, true},
		"opencode key nested deeper":        {"---\ndisable-model-invocation: true\nmetadata:\n  other:\n    opencode/autoinvoke: \"false\"\n---\n", true, false},
		"opencode key in a block scalar":    {"---\ndisable-model-invocation: true\nmetadata:\n  note: |\n    opencode/autoinvoke: \"false\"\n---\n", true, false},
		"second metadata block drops it":    {"---\ndisable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\nmetadata:\n  other: 1\n---\n", true, false},
		"no closing delimiter":              {"---\ndisable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\nbody\n", false, false},
		"no frontmatter at all":             {"# skill\n", false, false},
	} {
		claude, opencode := guarded(c.text)
		if claude != c.claude || opencode != c.opencode {
			t.Errorf("%s: guarded = %v/%v, want %v/%v", name, claude, opencode, c.claude, c.opencode)
		}
	}
}

func TestCodexGuardCountsOnlyUnderPolicy(t *testing.T) {
	for name, c := range map[string]struct {
		text string
		want bool
	}{
		"guard":                        {"policy:\n  allow_implicit_invocation: false\n", true},
		"beside interface":             {"interface:\n  display_name: x\n# note\npolicy:\n  allow_implicit_invocation: false\n", true},
		"allowed":                      {"policy:\n  allow_implicit_invocation: true\n", false},
		"top level":                    {"allow_implicit_invocation: false\n", false},
		"nested deeper":                {"policy:\n  other:\n    allow_implicit_invocation: false\n", false},
		"under another block":          {"interface:\n  allow_implicit_invocation: false\n", false},
		"second policy block drops it": {"policy:\n  allow_implicit_invocation: false\npolicy:\n  other: 1\n", false},
		"missing file":                 {"", false},
		"tab indent":                   {"policy:\n\tallow_implicit_invocation: false\n", false},
		"tab elsewhere":                {"interface:\n\tdisplay_name: x\npolicy:\n  allow_implicit_invocation: false\n", false},
	} {
		if got := codexGuarded(c.text); got != c.want {
			t.Errorf("%s: codexGuarded = %v, want %v", name, got, c.want)
		}
	}
}

func writeSkill(t *testing.T, c checkout.Checkout, name, body string) {
	t.Helper()
	dir := filepath.Join(c.Skills(), "g", name)
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if claude, opencode := guarded(body); claude && opencode {
		if err := os.WriteFile(filepath.Join(dir, "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnguardedNamesEachLocalSkillAndKey(t *testing.T) {
	c := checkout.Checkout{Root: t.TempDir()}
	if err := os.MkdirAll(c.Skills(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(c.Lock(), []byte(`{"version": 1, "skills": {"locked": {}}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, c, "guarded-one", "---\ndisable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\n---\nbody\n")
	writeSkill(t, c, "locked", "---\nname: locked\n---\nupstream body\n")
	writeSkill(t, c, "half-guarded", "---\ndisable-model-invocation: true\n---\nbody\n")
	writeSkill(t, c, "metadata-only", "---\nmetadata:\n  opencode/autoinvoke: \"false\"\n---\nbody\n")
	writeSkill(t, c, "workflow", "---\ndescription: auto-invoked\n---\nbody\n")
	writeSkill(t, c, "land", "---\ndescription: merges\n---\nbody\n")
	writeSkill(t, c, "build", "---\ndescription: runs unattended\n---\nbody\n")
	writeSkill(t, c, "open-pr", "---\ndescription: model-invoked delivery\n---\nbody\n")
	writeSkill(t, c, "codex-only", "---\ndescription: guarded for Codex alone\n---\nbody\n")
	if err := os.WriteFile(filepath.Join(c.Skills(), "g", "codex-only", "agents", "openai.yaml"), []byte("policy:\n  allow_implicit_invocation: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(c.Skills(), "g", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	problems, err := Unguarded(c)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"empty: missing SKILL.md",
		"half-guarded: missing opencode/autoinvoke",
		"half-guarded: missing agents/openai.yaml allow_implicit_invocation",
		"metadata-only: missing disable-model-invocation",
		"metadata-only: missing agents/openai.yaml allow_implicit_invocation",
		"land: missing disable-model-invocation",
		"land: missing opencode/autoinvoke",
		"land: missing agents/openai.yaml allow_implicit_invocation",
		"build: missing disable-model-invocation",
		"build: missing opencode/autoinvoke",
		"build: missing agents/openai.yaml allow_implicit_invocation",
		"codex-only: missing disable-model-invocation",
		"codex-only: missing opencode/autoinvoke",
		"workflow: missing disable-model-invocation",
		"workflow: missing opencode/autoinvoke",
		"workflow: missing agents/openai.yaml allow_implicit_invocation",
	}
	got := strings.Join(problems, "\n")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("Unguarded did not report %q:\n%s", w, got)
		}
	}
	for _, exempt := range []string{"locked", "guarded-one", "open-pr"} {
		if strings.Contains(got, exempt+":") {
			t.Errorf("Unguarded reported %s:\n%s", exempt, got)
		}
	}
	if len(problems) != len(want) {
		t.Errorf("Unguarded reported %d problems, want %d:\n%s", len(problems), len(want), got)
	}
}

func TestShippedLocalSkillsCarryAllGuards(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	problems, err := Unguarded(checkout.Checkout{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("shipped skills are unguarded: %v", problems)
	}
}

func TestShippedSkillLinksResolve(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c := checkout.Checkout{Root: root}
	skills, err := c.SkillDirs()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, sk := range skills {
		names[sk.Name] = true
		rel, err := filepath.Rel(c.AgentSkills(), sk.Dir)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := os.Readlink(filepath.Join(c.AgentSkills(), sk.Name)); err != nil || got != rel {
			t.Errorf(".agents/skills/%s = %q, %v; want %q", sk.Name, got, err, rel)
		}
		if got, err := os.Readlink(filepath.Join(c.ClaudeSkills(), sk.Name)); err != nil || got != "../../.agents/skills/"+sk.Name {
			t.Errorf(".claude/skills/%s = %q, %v; want ../../.agents/skills/%s", sk.Name, got, err, sk.Name)
		}
	}
	for _, dir := range []string{c.AgentSkills(), c.ClaudeSkills()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !names[e.Name()] {
				t.Errorf("%s holds a link for %s, which no skill group has", dir, e.Name())
			}
		}
	}
}

func TestShippedLockedSkillsStayOutOfTheClaudeListing(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c := checkout.Checkout{Root: root}
	var settings struct {
		SkillOverrides map[string]string `json:"skillOverrides"`
	}
	data, err := os.ReadFile(filepath.Join(root, "clients", "claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	lf, err := readLock(c.Lock())
	if err != nil {
		t.Fatal(err)
	}
	skills, err := c.SkillDirs()
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range skills {
		if !lf.skills.has(sk.Name) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(sk.Dir, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if claude, _ := guarded(string(body)); !claude && settings.SkillOverrides[sk.Name] == "" {
			t.Errorf("locked skill %s reaches the Claude listing in full; add it to skillOverrides in clients/claude.json", sk.Name)
		}
	}
}

func TestSkillToolCallsReachASkillTheModelSees(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	c := checkout.Checkout{Root: root}
	var settings struct {
		SkillOverrides map[string]string `json:"skillOverrides"`
	}
	data, err := os.ReadFile(filepath.Join(root, "clients", "claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	skills, err := c.SkillDirs()
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]string{}
	for _, sk := range skills {
		dirs[sk.Name] = sk.Dir
	}
	call := regexp.MustCompile(`Skill tool|load (?:the )?` + "`" + `[a-z0-9-]+` + "`" + ` skill`)
	name := regexp.MustCompile(`"([a-z0-9-]+)"|` + "`" + `([a-z0-9-]+)` + "`")
	calls := 0
	for _, top := range []string{"skills", "roles"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
				return err
			}
			text, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(text), "\n") {
				if !call.MatchString(line) {
					continue
				}
				for _, m := range name.FindAllStringSubmatch(line[call.FindStringIndex(line)[0]:], -1) {
					target := m[1] + m[2]
					dir, ok := dirs[target]
					if !ok {
						continue
					}
					calls++
					body, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
					if err != nil {
						return err
					}
					hidden, _ := guarded(string(body))
					if o := settings.SkillOverrides[target]; o == "user-invocable-only" || o == "off" {
						hidden = true
					}
					if hidden {
						rel, _ := filepath.Rel(root, path)
						t.Errorf("%s:%d loads %s through the Skill tool, which refuses it: list it name-only in clients/claude.json or read its SKILL.md by path", rel, i+1, target)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls == 0 {
		t.Fatal("found no Skill tool calls; the pattern no longer matches the skill bodies")
	}
}
