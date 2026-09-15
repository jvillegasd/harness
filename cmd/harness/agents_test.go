package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runBinary runs the built harness in dir with the inherited environment, which the scenario runner needs for git and go.
func runBinary(t *testing.T, dir string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{stdout.String(), stderr.String(), code}
}

// TestAgentsRendersTheCapturedGolden runs the command boundary against the golden captured from the Python renderer before it was deleted.
func TestAgentsRendersTheCapturedGolden(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	out := resolvedTemp(t)
	r := runBinary(t, root, "agents", filepath.Join(root, "harness", "clients", "assignments.json"), out)
	if r.code != 0 {
		t.Fatalf("agents exited %d: %s", r.code, r.stderr)
	}
	goldenRoot := filepath.Join("testdata", "agents")
	expected := filesUnder(t, goldenRoot)
	if len(expected) != 8 {
		t.Fatalf("golden holds %d files, want 8", len(expected))
	}
	actual := filesUnder(t, out)
	if len(actual) != len(expected) {
		t.Fatalf("rendered %v, want %v", actual, expected)
	}
	for _, rel := range expected {
		want, err := os.ReadFile(filepath.Join(goldenRoot, rel))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(out, rel))
		if err != nil {
			t.Errorf("missing %s", rel)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s differs from the captured golden", rel)
		}
	}
	if check := runBinary(t, root, "agents", "--check", filepath.Join(root, "harness", "clients", "assignments.json"), out); check.code != 0 {
		t.Fatalf("check on fresh output exited %d: %s", check.code, check.stderr)
	}
}

func TestAgentsCheckExitsNonZeroOnStaleOrMissing(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "harness", "clients", "assignments.json")
	out := resolvedTemp(t)
	if r := runBinary(t, root, "agents", config, out); r.code != 0 {
		t.Fatalf("agents exited %d: %s", r.code, r.stderr)
	}
	target := filepath.Join(out, "claude", "reviewer-a.md")
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, append(raw, []byte("\nExtra.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := runBinary(t, root, "agents", "--check", config, out); r.code == 0 || !strings.Contains(r.stdout, "changed: claude/reviewer-a.md") {
		t.Fatalf("check on drifted output exited %d with %q", r.code, r.stdout)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if r := runBinary(t, root, "agents", "--check", config, out); r.code == 0 || !strings.Contains(r.stdout, "missing: claude/reviewer-a.md") {
		t.Fatalf("check on missing output exited %d with %q", r.code, r.stdout)
	}
}

// resolvedTemp resolves the temporary directory, because macOS temp paths pass through a symlink the renderer refuses.
func resolvedTemp(t *testing.T) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func filesUnder(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if _, err := os.Stat(root); err != nil {
		return files
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
