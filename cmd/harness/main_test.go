package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var binary string

// TestMain builds the real binary once, so every test drives the command boundary a caller sees.
func TestMain(m *testing.M) {
	if os.Getenv("HARNESS_FIXTURE_CONTROL") != "" {
		os.Exit(m.Run()) // Local acceptance service; no CLI build or model launch.
	}
	dir, err := os.MkdirTemp("", "harness-bin")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "harness")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const branch = "feat/1-x"

// repo creates a throwaway git repository on branch.
func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", branch)
	return dir
}

// env isolates git from the user's config; each directory gets its own HOME and global config file.
func env(dir string) []string {
	home := filepath.Join(dir, ".home")
	return []string{"HOME=" + home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + filepath.Join(home, "gitconfig")}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env(dir)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(bytes.TrimSpace(out))
}

type result struct {
	stdout, stderr string
	code           int
}

// runBinary runs the built harness in dir with the inherited environment.
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

// harness runs the built binary with isolated Git on PATH.
func harness(t *testing.T, dir string, args ...string) result {
	t.Helper()
	shims := t.TempDir()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(shims, "git")); err != nil {
		t.Fatal(err)
	}
	path := shims + ":/usr/bin:/bin"
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = append(env(dir), "PATH="+path)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{stdout.String(), stderr.String(), code}
}

func ok(t *testing.T, r result, what string) result {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("%s exited %d: %s", what, r.code, r.stderr)
	}
	return r
}
