package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// evalRun runs the built binary with its own HOME and scratch root.
func evalRun(t *testing.T, home, root string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"eval"}, args...)...)
	cmd.Dir = home
	cmd.Env = []string{"HOME=" + home, "HARNESS_SCRATCH_ROOT=" + root, "PATH=/usr/bin:/bin"}
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if exit, ok := cmd.Run().(*exec.ExitError); ok {
		code = exit.ExitCode()
	}
	return result{stdout.String(), stderr.String(), code}
}

// TestEvalPlantAndCleanCommand proves the CLI is wired to planting and cleaning: plant prints a run id and fills the scratch root from the environment, clean empties it, and an unknown run id exits 1.
func TestEvalPlantAndCleanCommand(t *testing.T) {
	base := t.TempDir()
	home, root, skill := filepath.Join(base, "home"), filepath.Join(base, "scratch"), filepath.Join(base, "tidy")
	for _, dir := range []string{home, skill} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: tidy\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := evalRun(t, home, root, "plant", "--variant", skill, "--baseline", skill, "--client", "codex", "--runs", "1")
	run := strings.TrimSpace(r.stdout)
	if r.code != 0 || run == "" {
		t.Fatalf("plant exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 2 {
		t.Fatalf("plant created %v under the scratch root", entries)
	}
	if r := evalRun(t, home, root, "clean", "20990101-000000-abcdef"); r.code != 1 {
		t.Errorf("an unknown run exited %d", r.code)
	}
	if r := evalRun(t, home, root, "clean", run); r.code != 0 {
		t.Fatalf("clean exited %d: %s", r.code, r.stderr)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("clean left %v", entries)
	}
}
