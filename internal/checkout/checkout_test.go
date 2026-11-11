package checkout

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// write creates a file and its parent directories.
func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkoutAt lays out a checkout: the root policy marker beside the Harness module.
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

// TestFindResolvesTheNearestCheckout is the worktree case: a checkout nested
// inside another resolves itself, not the enclosing one.
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

// TestFindRejectsADifferentModule covers a marker beside a foreign module: Find stops there rather than resolving an enclosing checkout.
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

// TestFindRejectsAMarkerWithoutTheModule pins the contract that a checkout is the marker together with the module, so a marker-only tree is not one.
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

func TestAssetLocations(t *testing.T) {
	root := "/repo"
	c := Checkout{Root: root}
	cases := map[string]string{
		c.Floor():        filepath.Join(root, "FLOOR.md"),
		c.Skills():       filepath.Join(root, "skills"),
		c.Aliases():      filepath.Join(root, "aliases.sh"),
		c.Clients():      filepath.Join(root, "clients"),
		c.Generated():    filepath.Join(root, ".generated"),
		c.Bench():        filepath.Join(root, "bench"),
		c.Lock():         filepath.Join(root, "skills-lock.json"),
		c.ClaudeSkills(): filepath.Join(root, ".claude", "skills"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("asset = %s, want %s", got, want)
		}
	}
}

// TestManagedIsExplicitNamespaces pins the ownership predicate: a managed asset and everything beneath it is the installer's, while the repository's own tooling at the promoted root is not.
func TestManagedIsExplicitNamespaces(t *testing.T) {
	c := Checkout{Root: "/repo"}
	managed := []string{
		"/repo/skills",
		"/repo/skills/tdd/SKILL.md",
		"/repo/FLOOR.md",
		"/repo/.generated/agents/claude/executor.md",
	}
	unmanaged := []string{
		"/repo",
		"/repo/cmd/harness/main.go",
		"/repo/internal/install/links.go",
		"/repo/README.md",
		"/repo/docs/agents/harness.json",
		"/repo/skills-lock.json",
		"/repo/elsewhere/FLOOR.md",
	}
	for _, path := range managed {
		if !c.Manages(path) {
			t.Errorf("Manages(%s) = false, want true", path)
		}
	}
	for _, path := range unmanaged {
		if c.Manages(path) {
			t.Errorf("Manages(%s) = true, want false: an unrelated repository file is not installer-owned", path)
		}
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

// TestSkillDirsFlattensGroups lists every grouped skill by its bare name, sorted, skipping hidden entries, files, and symlinks, and keeps a folder without its SKILL.md so a caller can report it.
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

// TestSkillDirsRefusesAmbiguousLayouts rejects a name two groups share and a skill outside any group, because clients see only the bare name.
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
