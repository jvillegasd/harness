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
	write(t, filepath.Join(root, "harness", "POLICY.md"), "# policy\n")
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
	write(t, filepath.Join(inner, "harness", "POLICY.md"), "# policy\n")
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
	write(t, filepath.Join(dir, "harness", "POLICY.md"), "# policy\n")

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
		c.Assets():       filepath.Join(root, "harness"),
		c.Policy():       filepath.Join(root, "harness", "POLICY.md"),
		c.Skills():       filepath.Join(root, "harness", "skills"),
		c.Commands():     filepath.Join(root, "harness", "commands"),
		c.Aliases():      filepath.Join(root, "harness", "aliases.sh"),
		c.Clients():      filepath.Join(root, "harness", "clients"),
		c.Generated():    filepath.Join(root, "harness", ".generated"),
		c.Bench():        filepath.Join(root, "harness", "bench"),
		c.Lock():         filepath.Join(root, "skills-lock.json"),
		c.ClaudeSkills(): filepath.Join(root, ".claude", "skills"),
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("asset = %s, want %s", got, want)
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
