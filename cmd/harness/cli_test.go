package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var shimTools = []string{"codex", "npm", "engram", "agent-browser", "node", "uv", "zed", "git", "gh", "npx", "brew", "wt", "go", "claude", "opencode", "bash", "sh", "mv", "du", "security", "secret-tool", "curl"}

type cliResult struct {
	stdout, stderr string
	code           int
	timedOut       bool
}

func runCLI(t *testing.T, cwd, home, shims, log string, args ...string) cliResult {
	t.Helper()
	if err := os.MkdirAll(shims, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range shimTools {
		script := "#!/bin/sh\nprintf '%s\\n' \"" + name + " $*\" >> \"" + log + "\"\nexit 1\n"
		if err := os.WriteFile(filepath.Join(shims, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = []string{"HOME=" + home, "PATH=" + shims}
	cmd.Stdin = read
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	result := cliResult{stdout: stdout.String(), stderr: stderr.String()}
	if ctx.Err() != nil {
		result.timedOut = true
		return result
	}
	if exit, ok := err.(*exec.ExitError); ok {
		result.code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return result
}

type matrixCase struct {
	result cliResult
	cwd    string
	home   string
	log    string
}

func runMatrixCase(t *testing.T, args []string) matrixCase {
	t.Helper()
	base := t.TempDir()
	cwd, home, shims, log := filepath.Join(base, "cwd"), filepath.Join(base, "home"), filepath.Join(base, "shims"), filepath.Join(base, "subprocesses")
	for _, dir := range []string{cwd, home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return matrixCase{runCLI(t, cwd, home, shims, log, args...), cwd, home, log}
}

func (c matrixCase) assertClean(t *testing.T) {
	t.Helper()
	if raw, err := os.ReadFile(c.log); err == nil && strings.TrimSpace(string(raw)) != "" {
		t.Errorf("a subprocess ran: %s", strings.TrimSpace(string(raw)))
	}
	for _, dir := range []string{c.cwd, c.home} {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
			t.Errorf("%s gained %v", dir, entries)
		}
	}
}

func TestCommandHelpMatrix(t *testing.T) {
	t.Parallel()
	commands := [][]string{
		{},
		{"install"},
		{"update"},
		{"update", "clients"},
		{"update", "clients", "mcps"},
		{"uninstall"},
		{"skills"},
		{"skills", "add"},
		{"skills", "add", "owner/repo", "myskill"},
		{"skills", "update"},
		{"skills", "remove"},
		{"skills", "verify"},
		{"skills", "new"},
		{"skills", "move"},
		{"statusline"},
		{"models"},
		{"change-map"},
		{"review"},
		{"review", "target"},
		{"review", "worktree"},
		{"review", "worktree", "add"},
		{"review", "worktree", "retire"},
		{"review", "snapshot"},
		{"review", "verdict"},
		{"worktree"},
		{"worktree", "audit"},
		{"worktree", "retire"},
		{"worktree", "retire", "some/path"},
		{"land"},
		{"land", "check"},
		{"land", "check", "--merge", "12"},
		{"land", "merge"},
		{"land", "merge", "12", "--accepted"},
		{"land", "sync"},
		{"land", "finish"},
		{"land", "finish", "12", "--keep-branch"},
		{"land", "repair"},
		{"land", "repair", "12", "--push"},
		{"pr"},
		{"pr", "status"},
		{"pr", "status", "--stack", "12"},
		{"pr", "threads"},
		{"pr", "threads", "reply", "12", "T1"},
		{"tickets"},
		{"tickets", "check"},
		{"tickets", "frontier"},
		{"tickets", "publish"},
		{"resume"},
	}
	for _, command := range commands {
		for _, flag := range []string{"--help", "-h"} {
			want := append(append([]string{}, command...), flag)
			t.Run(strings.Join(want, " "), func(t *testing.T) {
				t.Parallel()
				c := runMatrixCase(t, want)
				if c.result.timedOut {
					t.Fatalf("help read stdin and hung")
				}
				if c.result.code != 0 {
					t.Fatalf("help exited %d: %s", c.result.code, c.result.stderr)
				}
				if c.result.stdout == "" {
					t.Fatal("help printed no usage to stdout")
				}
				if c.result.stderr != "" {
					t.Fatalf("help wrote to stderr: %q", c.result.stderr)
				}
				c.assertClean(t)
			})
		}
	}
}

func TestCommandInvocationMatrix(t *testing.T) {
	t.Parallel()
	args := [][]string{
		{"bogus"},
		{"update", "bogus"},
		{"update", "agents"},
		{"update", "--client", "cursor"},
		{"update", "clients", "--help", "extra"},
		{"install", "--bogus"},
		{"install", "bogus"},
		{"install", "skills", "--client"},
		{"install", "--client", "cursor"},
		{"uninstall", "--clear-backups", "--yes"},
		{"uninstall", "--all", "skills"},
		{"uninstall", "--client", "cursor"},
		{"uninstall", "--no-backup"},
		{"skills"},
		{"skills", "bogus"},
		{"skills", "add"},
		{"skills", "add", "owner/repo"},
		{"skills", "add", "bad-source", "name"},
		{"skills", "update", "extra"},
		{"skills", "remove"},
		{"skills", "verify", "extra"},
		{"skills", "new"},
		{"skills", "new", "demo"},
		{"skills", "new", "demo", "g", "--bogus"},
		{"skills", "move", "demo", "Bad/Group"},
		{"skills", "move", "demo", "g", "--guarded"},
		{"models"},
		{"statusline", "--bogus"},
		{"change-map", "--bogus"},
		{"change-map", "extra"},
		{"change-map", "--base"},
		{"change-map", "--json=maybe"},
		{"review"},
		{"review", "bogus"},
		{"review", "target", "--bogus"},
		{"review", "target", "1", "2"},
		{"review", "worktree"},
		{"review", "worktree", "bogus"},
		{"review", "worktree", "add"},
		{"review", "worktree", "add", "a", "b"},
		{"review", "worktree", "retire", "extra"},
		{"review", "worktree", "retire", "--patch", "x"},
		{"review", "snapshot"},
		{"review", "snapshot", "--base", "x"},
		{"review", "snapshot", "--bogus", "--", "a"},
		{"worktree"},
		{"worktree", "bogus"},
		{"worktree", "audit", "extra"},
		{"worktree", "retire"},
		{"worktree", "retire", "--bogus"},
		{"worktree", "retire", "some/path", "--branch", "x"},
		{"worktree", "retire", "--branch", "x", "--keep-branch"},
		{"worktree", "retire", "one", "two"},
		{"worktree", "audit", "--forge=maybe"},
		{"review", "verdict"},
		{"review", "verdict", "7"},
		{"review", "verdict", "--result", "PASS", "--base", "a", "--sha", "b", "--level", "medium"},
		{"review", "verdict", "x", "--result", "PASS", "--base", "a", "--sha", "b", "--level", "medium"},
		{"review", "verdict", "7", "8", "--result", "PASS", "--base", "a", "--sha", "b", "--level", "medium"},
		{"review", "verdict", "--bogus", "7"},
		{"review", "prune", "--forge"},
		{"land"},
		{"land", "bogus"},
		{"land", "check", "--bogus"},
		{"land", "check", "feat/x"},
		{"land", "check", "--stack"},
		{"land", "check", "--stack", "1", "2"},
		{"land", "check", "42", ""},
		{"land", "merge"},
		{"land", "merge", "12"},
		{"land", "merge", "12", "--head", "abc"},
		{"land", "merge", "12", "--head"},
		{"land", "merge", "12", "13", "--head", "abc", "--body-file", "b.md"},
		{"land", "merge", "--bogus", "--head", "abc", "--body-file", "b.md"},
		{"land", "sync", "extra"},
		{"land", "finish"},
		{"land", "finish", "--bogus"},
		{"land", "finish", "feat/x"},
		{"land", "repair"},
		{"land", "repair", "12"},
		{"land", "repair", "12", "--landed", "11", "--push"},
		{"land", "repair", "12", "13", "--push"},
		{"land", "repair", "12", "--landed"},
		{"pr"},
		{"pr", "bogus"},
		{"pr", "status", "--bogus"},
		{"pr", "status", "feat/x"},
		{"pr", "status", "--stack"},
		{"pr", "status", "--watch", "--interval", "0"},
		{"pr", "status", "--timeout", "5"},
		{"pr", "threads"},
		{"pr", "threads", "bogus", "1", "T1"},
		{"pr", "threads", "reply", "x", "T1", "--body-file", "f"},
		{"pr", "threads", "reply", "1", "T1"},
		{"pr", "threads", "reply", "1", "T1", "T2", "--body-file", "f"},
		{"pr", "threads", "resolve", "1"},
		{"pr", "threads", "resolve", "1", "T1", "--body-file", "f"},
		{"tickets"},
		{"tickets", "bogus"},
		{"tickets", "check", "a", "b"},
		{"tickets", "check", "../x"},
		{"tickets", "check", "--bogus"},
		{"tickets", "frontier"},
		{"tickets", "frontier", "a", "--dir", "d"},
		{"tickets", "frontier", "a", "--done", "x"},
		{"tickets", "publish"},
		{"tickets", "publish", "a", "b"},
		{"tickets", "publish", "../a"},
		{"resume", "extra"},
	}
	for _, want := range args {
		t.Run(strings.Join(want, " "), func(t *testing.T) {
			t.Parallel()
			c := runMatrixCase(t, want)
			if c.result.timedOut {
				t.Fatalf("an invalid invocation read stdin and hung")
			}
			if c.result.code != 2 {
				t.Fatalf("invalid invocation exited %d, want 2: %s", c.result.code, c.result.stderr)
			}
			if c.result.stderr == "" {
				t.Fatal("invalid invocation printed no diagnostic to stderr")
			}
			if c.result.stdout != "" {
				t.Fatalf("invalid invocation wrote to stdout: %q", c.result.stdout)
			}
			c.assertClean(t)
		})
	}
}

func TestOperationalFailuresExitOne(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	for _, args := range [][]string{
		{"skills", "verify"},
	} {
		if r := runBinary(t, outside, args...); r.code != 1 || r.stderr == "" {
			t.Errorf("%v outside a checkout or configured repository exited %d with stderr %q, want 1 and a diagnostic", args, r.code, r.stderr)
		}
	}
}
