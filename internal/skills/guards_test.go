package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jvillegasd/harness/internal/checkout"
)

// TestGuardedCountsAGuardOnlyWhereItsClientReadsIt covers the frontmatter shapes at the parser, so the install and verify commands need one wiring case each rather than this table.
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

// writeSkill writes one skill body under the checkout's skills directory.
func writeSkill(t *testing.T, c checkout.Checkout, name, body string) {
	t.Helper()
	dir := filepath.Join(c.Skills(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestUnguardedNamesEachLocalSkillAndKey reports the skill and the missing key, exempts locked bodies and an unguarded skill outside mustGuard, and separates an unreadable entry from a missing one.
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
	if err := os.MkdirAll(filepath.Join(c.Skills(), "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(c.Root, "nowhere"), filepath.Join(c.Skills(), "dangling")); err != nil {
		t.Fatal(err)
	}
	problems, err := Unguarded(c)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"dangling: unreadable",
		"empty: missing SKILL.md",
		"half-guarded: missing opencode/autoinvoke",
		"metadata-only: missing disable-model-invocation",
		"land: missing disable-model-invocation",
		"land: missing opencode/autoinvoke",
	}
	got := strings.Join(problems, "\n")
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("Unguarded did not report %q:\n%s", w, got)
		}
	}
	for _, exempt := range []string{"locked", "guarded-one", "workflow"} {
		if strings.Contains(got, exempt+":") {
			t.Errorf("Unguarded reported %s:\n%s", exempt, got)
		}
	}
	if len(problems) != len(want) {
		t.Errorf("Unguarded reported %d problems, want %d:\n%s", len(problems), len(want), got)
	}
}

// TestShippedLocalSkillsCarryBothGuards runs the shipped parser over the shipped skills, so a local skill added without both guards fails here rather than in a client.
func TestShippedLocalSkillsCarryBothGuards(t *testing.T) {
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
