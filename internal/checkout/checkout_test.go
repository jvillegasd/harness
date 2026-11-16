package checkout

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func checkoutAt(t *testing.T, root string) string {
	t.Helper()
	write(t, filepath.Join(root, "FLOOR.md"), "# floor\n")
	write(t, filepath.Join(root, "go.mod"), "module github.com/jvillegasd/harness\n\ngo 1.26.0\n")
	return root
}

func TestFindResolvesRootAndNested(t *testing.T) {
	root := checkoutAt(t, t.TempDir())
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{root, nested} {
		got, err := Find(start)
		if err != nil {
			t.Fatalf("Find(%s): %v", start, err)
		}
		if got.Root != root {
			t.Errorf("Find(%s).Root = %s, want %s", start, got.Root, root)
		}
	}
}

func TestFindResolvesTheNearestCheckout(t *testing.T) {
	outer := checkoutAt(t, t.TempDir())
	inner := checkoutAt(t, filepath.Join(outer, "worktree"))

	for start, want := range map[string]string{
		filepath.Join(inner, "deep", "dir"): inner,
		filepath.Join(outer, "elsewhere"):   outer,
	} {
		got, err := Find(start)
		if err != nil {
			t.Fatalf("Find(%s): %v", start, err)
		}
		if got.Root != want {
			t.Errorf("Find(%s).Root = %s, want %s", start, got.Root, want)
		}
	}
}

func TestFindRejectsADifferentModule(t *testing.T) {
	outer := checkoutAt(t, t.TempDir())
	inner := filepath.Join(outer, "fork")
	write(t, filepath.Join(inner, "FLOOR.md"), "# floor\n")
	write(t, filepath.Join(inner, "go.mod"), "module example.com/other\n")
	nested := filepath.Join(inner, "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, start := range []string{inner, nested} {
		got, err := Find(start)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("Find(%s) = %v, %v; want ErrNotFound rather than an enclosing checkout", start, got, err)
		}
		if !strings.Contains(err.Error(), "not the Harness module") {
			t.Errorf("Find(%s) error %q does not name the module mismatch", start, err)
		}
	}
}

func TestFindRejectsAMarkerWithoutTheModule(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "FLOOR.md"), "# floor\n")

	_, err := Find(dir)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find = %v, want ErrNotFound for a marker that is not the Harness module", err)
	}
	if !strings.Contains(err.Error(), "no Harness module") {
		t.Errorf("error %q does not name the missing module", err)
	}
}

func TestFindOutsideAnyCheckout(t *testing.T) {
	_, err := Find(t.TempDir())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Find outside a checkout = %v, want ErrNotFound", err)
	}
	if err == nil || !strings.Contains(err.Error(), "Harness checkout") {
		t.Errorf("error %q does not name the Harness checkout requirement", err)
	}
}

func TestModuleOf(t *testing.T) {
	cases := map[string]string{
		"module github.com/jvillegasd/harness\n\ngo 1.26.0\n": "github.com/jvillegasd/harness",
		"// comment\n\nmodule example.com/x\n":                "example.com/x",
		"go 1.26.0\n":                                         "",
	}
	for input, want := range cases {
		if got := moduleOf([]byte(input)); got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSkillDirsFlattensGroups(t *testing.T) {
	c := Checkout{Root: t.TempDir()}
	write(t, filepath.Join(c.Skills(), "workflow", "tdd", "SKILL.md"), "x\n")
	write(t, filepath.Join(c.Skills(), "support", "ponytail", "SKILL.md"), "x\n")
	write(t, filepath.Join(c.Skills(), "support", "notes.md"), "x\n")
	write(t, filepath.Join(c.Skills(), ".hidden", "skip", "SKILL.md"), "x\n")
	if err := os.MkdirAll(filepath.Join(c.Skills(), "ops", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(c.Skills(), "workflow", "tdd"), filepath.Join(c.Skills(), "ops", "linked")); err != nil {
		t.Fatal(err)
	}
	got, err := c.SkillDirs()
	if err != nil {
		t.Fatal(err)
	}
	want := []Skill{
		{"empty", filepath.Join(c.Skills(), "ops", "empty")},
		{"ponytail", filepath.Join(c.Skills(), "support", "ponytail")},
		{"tdd", filepath.Join(c.Skills(), "workflow", "tdd")},
	}
	if len(got) != len(want) {
		t.Fatalf("SkillDirs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SkillDirs[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestSkillDirsRefusesAmbiguousLayouts(t *testing.T) {
	for name, seed := range map[string]func(c Checkout){
		"same name in two groups": func(c Checkout) {
			write(t, filepath.Join(c.Skills(), "a", "dup", "SKILL.md"), "x\n")
			write(t, filepath.Join(c.Skills(), "b", "dup", "SKILL.md"), "x\n")
		},
		"skill directly under skills": func(c Checkout) {
			write(t, filepath.Join(c.Skills(), "bare", "SKILL.md"), "x\n")
		},
		"SKILL.md at the skills root": func(c Checkout) {
			write(t, filepath.Join(c.Skills(), "SKILL.md"), "x\n")
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := Checkout{Root: t.TempDir()}
			seed(c)
			if got, err := c.SkillDirs(); err == nil {
				t.Fatalf("SkillDirs = %v, want an error", got)
			}
		})
	}
}
