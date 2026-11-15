package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	home, root, variant, baseline, skeleton string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := t.TempDir()
	f := fixture{
		home:     filepath.Join(base, "home"),
		root:     filepath.Join(base, "scratch"),
		variant:  filepath.Join(base, "v2", "tidy"),
		baseline: filepath.Join(base, "v1", "tidy"),
		skeleton: filepath.Join(base, "skeleton"),
	}
	t.Setenv(RootEnv, f.root)
	files := map[string]string{
		filepath.Join(f.variant, "SKILL.md"):            "---\nname: tidy\n---\nnew body\n",
		filepath.Join(f.variant, "references", "a.md"):  "a\n",
		filepath.Join(f.baseline, "SKILL.md"):           "---\nname: tidy\n---\nold body\n",
		filepath.Join(f.skeleton, "main.go"):            "package main\n",
		filepath.Join(f.skeleton, "docs", "README.md"):  "readme\n",
		filepath.Join(f.home, ".claude", "skills", "x"): "the human's own\n",
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f fixture) plant(t *testing.T, client string, runs int) (Manifest, Key) {
	t.Helper()
	m, err := Plant(f.home, PlantOptions{Variant: f.variant, Baseline: f.baseline, Skeleton: f.skeleton, Client: client, Runs: runs})
	if err != nil {
		t.Fatal(err)
	}
	_, k, err := ReadRun(f.home, m.Run)
	if err != nil {
		t.Fatal(err)
	}
	return m, k
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestPlantCreatesBlindedDirectoriesPerArm(t *testing.T) {
	f := newFixture(t)
	m, k := f.plant(t, "claude", 2)
	if len(m.Dirs) != 4 {
		t.Fatalf("planted %d directories", len(m.Dirs))
	}
	arms := map[string]int{}
	for _, p := range m.Dirs {
		if filepath.Dir(p.Dir) != f.root {
			t.Errorf("%s is not under the scratch root", p.Dir)
		}
		arm := k.Labels[p.Label]
		arms[arm]++
		want := map[string]string{Variant: "new body", Baseline: "old body"}[arm]
		if got := read(t, filepath.Join(p.Dir, ".claude", "skills", "tidy", "SKILL.md")); !strings.Contains(got, want) {
			t.Errorf("%s (%s) planted %q", p.Label, arm, got)
		}
		if read(t, filepath.Join(p.Dir, "main.go")) != "package main\n" {
			t.Errorf("%s lacks the skeleton", p.Dir)
		}
		filepath.WalkDir(p.Dir, func(path string, d os.DirEntry, err error) error {
			rel, _ := filepath.Rel(f.root, path)
			for _, word := range Banned {
				if strings.Contains(strings.ToLower(rel), word) {
					t.Errorf("a candidate sees %q in %s", word, rel)
				}
			}
			return nil
		})
	}
	if arms[Variant] != 2 || arms[Baseline] != 2 || len(k.Labels) != 4 {
		t.Errorf("arms %v, key %v", arms, k.Labels)
	}
	meta := MetaDir(f.home, m.Run)
	for _, p := range m.Dirs {
		if strings.HasPrefix(meta, p.Dir) {
			t.Errorf("the key is inside %s", p.Dir)
		}
	}
	if read(t, filepath.Join(f.home, ".claude", "skills", "x")) != "the human's own\n" {
		t.Error("the human's installed skills changed")
	}
	if entries, _ := os.ReadDir(filepath.Join(f.home, ".claude", "skills")); len(entries) != 1 {
		t.Errorf("the human's skills gained %v", entries)
	}
}

func TestPlantPutsTheSkillAtEachClientsPath(t *testing.T) {
	f := newFixture(t)
	for client, path := range map[string]string{"claude": ".claude/skills/tidy", "codex": ".agents/skills/tidy", "opencode": ".agents/skills/tidy"} {
		m, _ := f.plant(t, client, 1)
		if _, err := os.Stat(filepath.Join(m.Dirs[0].Dir, filepath.FromSlash(path), "SKILL.md")); err != nil {
			t.Errorf("%s: no skill at %s", client, path)
		}
	}
}

func TestPlantRefusesMismatchedSkillsAndUnknownClients(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.baseline, "SKILL.md"), []byte("---\nname: other\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Plant(f.home, PlantOptions{Variant: f.variant, Baseline: f.baseline, Client: "claude", Runs: 1}); err == nil {
		t.Error("two different skills were accepted")
	}
	if _, err := Plant(f.home, PlantOptions{Variant: f.variant, Baseline: f.variant, Client: "cursor", Runs: 1}); err == nil {
		t.Error("an unknown client was accepted")
	}
	if _, err := os.Stat(f.root); err == nil {
		if entries, _ := os.ReadDir(f.root); len(entries) != 0 {
			t.Errorf("a refused plant created %v", entries)
		}
	}
}

func TestCleanRemovesOnlyItsRun(t *testing.T) {
	f := newFixture(t)
	other, _ := f.plant(t, "claude", 1)
	m, _ := f.plant(t, "claude", 1)
	stranger := filepath.Join(f.root, "stranger")
	if err := os.Mkdir(stranger, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Clean(f.home, "20990101-000000-abcdef"); err == nil {
		t.Error("an unknown run id was cleaned")
	}
	if _, err := Clean(f.home, m.Run); err != nil {
		t.Fatal(err)
	}
	for _, p := range m.Dirs {
		if _, err := os.Stat(p.Dir); !os.IsNotExist(err) {
			t.Errorf("%s survived", p.Dir)
		}
	}
	if _, err := os.Stat(MetaDir(f.home, m.Run)); !os.IsNotExist(err) {
		t.Error("the key and manifest survived")
	}
	for _, kept := range []string{stranger, other.Dirs[0].Dir, MetaDir(f.home, other.Run)} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("clean removed %s", kept)
		}
	}
}

func TestPlantRefusesASkillNameACandidateMustNotSee(t *testing.T) {
	f := newFixture(t)
	for _, name := range []string{"testing-guide", "../escape"} {
		for _, dir := range []string{f.variant, f.baseline} {
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: "+name+"\n---\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Plant(f.home, PlantOptions{Variant: f.variant, Baseline: f.baseline, Client: "claude", Runs: 1}); err == nil {
			t.Errorf("skill name %q was planted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.root), "escape")); err == nil {
		t.Error("a skill was written outside its candidate directory")
	}
}
