package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// prRepo is a checkout on branch with one pushed docs commit, published as pull request 7, and a runner that answers gh from a case list.
func prRepo(t *testing.T) (dir, head string, run func(forge string, args ...string) result) {
	t.Helper()
	dir = repo(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	git(t, dir, "remote", "add", "origin", origin)
	git(t, dir, "push", "-q", "origin", "HEAD:refs/heads/main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "README.md")
	git(t, dir, "commit", "-q", "-m", "docs")
	git(t, dir, "push", "-q", "origin", "HEAD:refs/heads/"+branch, "HEAD:refs/pull/7/head")
	head = git(t, dir, "rev-parse", "HEAD")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	run = func(forge string, args ...string) result {
		t.Helper()
		shims := t.TempDir()
		script := "#!/bin/sh\ncase \"$1 $2\" in\n" + forge + "*) echo \"unexpected gh $*\" >&2; exit 1;;\nesac\n"
		if err := os.WriteFile(filepath.Join(shims, "gh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		cmd.Env = append(env(dir), "PATH="+shims+":"+filepath.Dir(gitPath)+":/usr/bin:/bin")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		code := 0
		if exit, ok := cmd.Run().(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		return result{stdout.String(), stderr.String(), code}
	}
	return dir, head, run
}

// prForge answers the reads a status pass makes for pull request 7 at head, with the given required checks.
func prForge(head, checks string) string {
	return `"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;
"api user") echo me;;
"pr list") echo '[]';;
"pr view") echo '{"number":7,"state":"OPEN","headRefName":"` + branch + `","headRefOid":"` + head + `","baseRefName":"main","author":{"login":"me"},"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}';;
"pr checks") echo '` + checks + `';;
"api graphql") echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}';;
`
}

// TestPrStatusCommand proves the CLI is wired to the status verdict: it prints one verdict per pull request and exits with the code naming its first blocker.
func TestPrStatusCommand(t *testing.T) {
	_, head, run := prRepo(t)

	r := run(prForge(head, `[{"name":"ci","bucket":"pass"}]`), "pr", "status")
	var statuses []struct {
		Number int
		Ready  bool
		Holds  []string
	}
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &statuses) != nil || len(statuses) != 1 || !statuses[0].Ready || statuses[0].Number != 7 {
		t.Fatalf("pr status exited %d with %q, stderr %q", r.code, r.stdout, r.stderr)
	}

	if r := run(prForge(head, `[{"name":"ci","bucket":"fail"}]`), "pr", "status", "7"); r.code != 5 {
		t.Errorf("a failing required check exited %d: %s", r.code, r.stdout)
	}
	if r := run(`"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;`+"\n", "pr", "status", "7"); r.code != 1 || r.stdout != "" {
		t.Errorf("an unreadable forge exited %d with stdout %q", r.code, r.stdout)
	}
}
