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

// TestLandCheckCommand proves the CLI is wired to the landing rules: it reads the forge through gh and the repository through git, prints one verdict per pull request, and exits 1 when the forge cannot be read.
func TestLandCheckCommand(t *testing.T) {
	dir := repo(t)
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
	head := git(t, dir, "rev-parse", "HEAD")

	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	run := func(forge string, args ...string) result {
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

	const forge = `"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;
"api user") echo me;;
"pr list") echo '[]';;
`
	view := `"pr view") echo '{"number":7,"state":"MERGED","headRefName":"` + branch + `","headRefOid":"` + head + `","baseRefName":"main","author":{"login":"me"}}';;` + "\n"
	r := ok(t, run(forge+view, "land", "check"), "land check")
	var checks []struct {
		Number           int
		Lane, Mode, Head string
		Mine             bool
		Holds            []string
		Review           struct{ Tier string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &checks); err != nil || len(checks) != 1 {
		t.Fatalf("land check printed %q: %v", r.stdout, err)
	}
	if c := checks[0]; c.Number != 7 || c.Lane != "merged" || c.Mode != "local" || c.Head != head || !c.Mine || len(c.Holds) != 0 || c.Review.Tier != "tests-docs" {
		t.Errorf("verdict %+v", c)
	}

	if r := run(forge, "land", "check", "7"); r.code != 1 || r.stdout != "" || r.stderr == "" {
		t.Errorf("an unreadable pull request exited %d with stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

// TestLandFinishCommand proves the CLI is wired to the cleanup: sync reports the default branch, and finish refuses an open pull request with nothing removed.
func TestLandFinishCommand(t *testing.T) {
	dir, head, run := prRepo(t)
	const forge = `"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;
"api user") echo me;;
`
	if r := run(forge, "land", "sync"); r.code != 0 || !strings.Contains(r.stdout, "main") {
		t.Errorf("sync exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	open := forge + `"pr view") echo '{"number":7,"state":"OPEN","headRefName":"` + branch + `","headRefOid":"` + head + `","baseRefName":"main","author":{"login":"me"}}';;` + "\n"
	r := run(open, "land", "finish", "7")
	if r.code != 1 || !strings.Contains(r.stderr, "not merged") || r.stdout != "" {
		t.Errorf("finish of an open pull request exited %d: %q %q", r.code, r.stdout, r.stderr)
	}
	if got := git(t, dir, "branch", "--show-current"); got != branch {
		t.Errorf("the checkout moved to %q", got)
	}
}

// TestLandRepairCommand proves the CLI is wired to the repair: a push with no repair waiting is refused, and a dependent that is someone else's is refused before any write.
func TestLandRepairCommand(t *testing.T) {
	_, head, run := prRepo(t)
	if r := run("", "land", "repair", "7", "--push"); r.code != 1 || !strings.Contains(r.stderr, "no repair of #7 is waiting") {
		t.Errorf("a push with nothing waiting exited %d: %s", r.code, r.stderr)
	}
	forge := `"repo view") echo '{"defaultBranchRef":{"name":"main"}}';;
"api user") echo me;;
"pr view") if [ "$3" = 7 ]; then echo '{"number":7,"state":"OPEN","headRefName":"` + branch + `","headRefOid":"` + head + `","baseRefName":"x","author":{"login":"someone"}}'; else echo '{"number":6,"state":"MERGED","headRefName":"x","headRefOid":"` + head + `","baseRefName":"main","author":{"login":"me"}}'; fi;;
`
	if r := run(forge, "land", "repair", "7", "--landed", "6"); r.code != 1 || !strings.Contains(r.stderr, "someone") || r.stdout != "" {
		t.Errorf("someone else's dependent exited %d: %q %q", r.code, r.stdout, r.stderr)
	}
}
