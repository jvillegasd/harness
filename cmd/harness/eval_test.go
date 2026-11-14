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

// TestEvalRunCommand proves the CLI is wired to the runs: a dry run prints each command and starts nothing, and a run starts the stand-in client in each planted directory and prints its label.
func TestEvalRunCommand(t *testing.T) {
	base := t.TempDir()
	home, root, skill, bin := filepath.Join(base, "home"), filepath.Join(base, "scratch"), filepath.Join(base, "tidy"), filepath.Join(base, "bin")
	for _, dir := range []string{home, skill, bin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(base, "started")
	files := map[string]string{
		filepath.Join(skill, "SKILL.md"): "---\nname: tidy\n---\n",
		filepath.Join(base, "ask.md"):    "Tidy up.\n",
		filepath.Join(bin, "claude"):     "#!/bin/sh\npwd >> '" + log + "'\ncat >/dev/null\necho done\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := strings.TrimSpace(evalRun(t, home, root, "plant", "--variant", skill, "--baseline", skill, "--client", "claude", "--runs", "1").stdout)
	runWith := func(args ...string) result {
		cmd := exec.Command(binary, append([]string{"eval", "run", run, "--prompt-file", filepath.Join(base, "ask.md"), "--model", "sonnet"}, args...)...)
		cmd.Dir = home
		cmd.Env = []string{"HOME=" + home, "HARNESS_SCRATCH_ROOT=" + root, "PATH=" + bin + ":/usr/bin:/bin"}
		var stdout, stderr strings.Builder
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if exit, ok := cmd.Run().(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		return result{stdout.String(), stderr.String(), code}
	}
	if r := runWith("--dry-run"); r.code != 0 || strings.Count(r.stdout, "claude -p --permission-mode acceptEdits --model sonnet") != 2 {
		t.Fatalf("dry run exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a dry run started a client")
	}
	r := runWith()
	if r.code != 0 || !strings.Contains(r.stdout, "r1") || !strings.Contains(r.stdout, "r2") {
		t.Fatalf("run exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	raw, _ := os.ReadFile(log)
	if strings.Count(string(raw), root) != 2 {
		t.Errorf("clients started in %q", raw)
	}
}
