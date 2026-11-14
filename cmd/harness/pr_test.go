package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if r := run(prForge(head, `[{"name":"ci","bucket":"pass"}]`), "pr", "status", "--watch", "7"); r.code != 0 || !strings.Contains(r.stdout, `"ready": true`) {
		t.Errorf("a watch over a ready pull request exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	// A timeout no longer than one interval gives up after the first pass, without sleeping.
	if r := run(prForge(head, `[{"name":"ci","bucket":"pending"}]`), "pr", "status", "--watch", "--interval", "5", "--timeout", "5", "7"); r.code != 8 || !strings.Contains(r.stdout, `"blocker": "pending"`) {
		t.Errorf("a timed-out watch exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	if r := run(`"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;`+"\n", "pr", "status", "7"); r.code != 1 || r.stdout != "" {
		t.Errorf("an unreadable forge exited %d with stdout %q", r.code, r.stdout)
	}
}

// TestPrThreadsCommand proves the CLI is wired to the thread writes: a reply reaches gh as one argument and prints the thread read back, and a foreign id is refused.
func TestPrThreadsCommand(t *testing.T) {
	_, _, run := prRepo(t)
	log := filepath.Join(t.TempDir(), "gh.log")
	body := filepath.Join(t.TempDir(), "reply.md")
	text := "it's `fixed`; \"see\" $HOME\n"
	if err := os.WriteFile(body, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	// The shim records each argument on its own line, then answers with one open thread.
	forge := `"api graphql") for a in "$@"; do printf '%s\n--\n' "$a" >> '` + log + `'; done; echo '{"data":{"repository":{"pullRequest":{"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[{"id":"T1","isResolved":false,"comments":{"totalCount":1,"nodes":[{"body":"b","url":"u"}]}}]}}}}}';;
`
	r := run(forge, "pr", "threads", "reply", "7", "T1", "--body-file", body)
	if r.code != 0 || !strings.Contains(r.stdout, `"id": "T1"`) {
		t.Fatalf("reply exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	raw, _ := os.ReadFile(log)
	if !strings.Contains(string(raw), "\n--\nbody="+text+"\n--\n") {
		t.Errorf("gh did not get the body as one argument: %q", raw)
	}
	if r := run(forge, "pr", "threads", "resolve", "7", "T9"); r.code != 1 || !strings.Contains(r.stderr, "T9") {
		t.Errorf("a foreign id exited %d: %s", r.code, r.stderr)
	}
}
