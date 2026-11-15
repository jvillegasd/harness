package main

import (
	"bytes"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// reviewRun runs the built binary in dir; review scratch lands in the repository's Git directory.
func reviewRun(t *testing.T, dir, data string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"review"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env(dir)...)
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

// scratchDir makes key's review directory holding a saved patch, as a review run leaves it, and returns it.
func scratchDir(t *testing.T, dir, key string) string {
	t.Helper()
	d := filepath.Join(git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir"), "harness-review", "branches", url.PathEscape(key))
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "review.patch"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return d
}

// TestReviewRemoveCommand proves the CLI is wired to the removal rule: the named branch's directory goes, another branch's stays, a second run is a no-op, and a recorded worktree is refused. The rule itself is settled in internal/review.
func TestReviewRemoveCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	merged, kept := scratchDir(t, dir, "feat/x"), scratchDir(t, dir, "feat/y")

	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/x"); r.code != 0 || strings.TrimSpace(r.stdout) != merged {
		t.Fatalf("remove exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(merged); !os.IsNotExist(err) {
		t.Errorf("the removed branch's directory is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(kept, "review.patch")); err != nil {
		t.Errorf("another branch's directory was touched: %v", err)
	}
	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/x"); r.code != 0 || r.stdout != "" {
		t.Errorf("a second remove exited %d with %q, want 0 and nothing", r.code, r.stdout)
	}

	for _, name := range []string{".", ".."} {
		if r := reviewRun(t, dir, data, "remove", "--branch", name); r.code != 2 || r.stderr == "" {
			t.Errorf("remove --branch %s exited %d with %q, want 2 and a diagnostic", name, r.code, r.stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(kept, "review.patch")); err != nil {
		t.Fatalf("a dot-segment branch name deleted another branch's directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(kept, "worktree.txt"), []byte(t.TempDir()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/y"); r.code != 1 || r.stderr == "" {
		t.Errorf("remove with a recorded worktree exited %d with %q, want 1 and a diagnostic", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(kept, "review.patch")); err != nil {
		t.Errorf("a refused remove deleted the patch: %v", err)
	}
}

// TestReviewPruneCommand proves prune reads local and remote-tracking branches from git; which directories go is settled in internal/review.
func TestReviewPruneCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	git(t, dir, "update-ref", "refs/remotes/origin/feat/remote", "HEAD")
	local, remote, gone := scratchDir(t, dir, branch), scratchDir(t, dir, "feat/remote"), scratchDir(t, dir, "feat/gone")

	if r := reviewRun(t, dir, data, "prune", "--dry-run"); r.code != 0 || strings.TrimSpace(r.stdout) != gone {
		t.Fatalf("prune --dry-run exited %d with %q, want only %s: %s", r.code, r.stdout, gone, r.stderr)
	}
	if _, err := os.Stat(gone); err != nil {
		t.Fatalf("a dry run deleted %s: %v", gone, err)
	}
	if r := reviewRun(t, dir, data, "prune"); r.code != 0 || strings.TrimSpace(r.stdout) != gone {
		t.Fatalf("prune exited %d with %q, want only %s: %s", r.code, r.stdout, gone, r.stderr)
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Errorf("the gone branch's directory survived: %v", err)
	}
	for _, d := range []string{local, remote} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("a live branch's directory was removed: %v", err)
		}
	}
}

// TestReviewLevelCommand proves the CLI is wired to the level rules, and that an install keeps the stored level. Which values are accepted is settled in internal/review.
func TestReviewLevelCommand(t *testing.T) {
	base := modelsBase(t)
	level := func(args ...string) result {
		return runHarness(t, base, nil, append([]string{"review", "level"}, args...)...)
	}
	if r := ok(t, level("--json"), "level --json"); strings.TrimSpace(r.stdout) != `{"level":"medium","source":"default"}` {
		t.Errorf("default level printed %q", r.stdout)
	}
	ok(t, level("high"), "set high")
	before := readFile(t, stateFile(base))
	for _, bad := range [][]string{{"max"}, {"--json", "off"}, {"off", "--json"}} {
		if r := level(bad...); r.code != 2 || r.stderr == "" {
			t.Errorf("level %v exited %d with %q, want 2 and a diagnostic", bad, r.code, r.stderr)
		}
	}
	if readFile(t, stateFile(base)) != before {
		t.Error("a rejected level rewrote state.json")
	}
	ok(t, runInstaller(t, base, "agents"), "install")
	if r := ok(t, level(), "level"); strings.TrimSpace(r.stdout) != "high (global)" {
		t.Errorf("after install the level is %q, want high (global)", r.stdout)
	}
	if r := ok(t, level("--help"), "level --help"); r.stdout == "" {
		t.Error("level --help printed nothing")
	}
}

// TestReviewVerdictCommand proves the CLI is wired to the verdict rules: a dry run prints the comment and posts nothing, a verdict for a commit that is not the head or on a closed pull request is refused, and a post sends the comment from a file.
func TestReviewVerdictCommand(t *testing.T) {
	dir, head, run := prRepo(t)
	base := git(t, dir, "rev-parse", "HEAD~1")
	log := filepath.Join(t.TempDir(), "gh.log")
	forge := func(state, oid string) string {
		return `"pr view") echo '{"state":"` + state + `","headRefOid":"` + oid + `"}';;
"pr comment") cat "$5" >> '` + log + `';;
`
	}
	notes := filepath.Join(t.TempDir(), "notes.md")
	if err := os.WriteFile(notes, []byte("- **bug** `a.go:4`: wrong sign\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := func(result string, extra ...string) []string {
		return append([]string{"review", "verdict", "7", "--result", result, "--base", base, "--sha", "HEAD", "--level", "medium"}, extra...)
	}

	r := run(forge("OPEN", head), args("FAIL", "--notes", notes, "--dry-run")...)
	if r.code != 0 || !strings.HasPrefix(r.stdout, "<!-- harness-review verdict=FAIL base="+base+" sha="+head+" level=medium -->\n") || !strings.Contains(r.stdout, "wrong sign") {
		t.Fatalf("dry run exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a dry run posted")
	}
	for name, c := range map[string]struct {
		forge string
		args  []string
		code  int
	}{
		"FAIL without notes": {forge("OPEN", head), args("FAIL"), 2},
		"moved head":         {forge("OPEN", strings.Repeat("c", 40)), args("PASS"), 1},
		"merged":             {forge("MERGED", head), args("PASS"), 1},
	} {
		if r := run(c.forge, c.args...); r.code != c.code || r.stderr == "" {
			t.Errorf("%s exited %d with %q, want %d and a diagnostic", name, r.code, r.stderr, c.code)
		}
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("a refused verdict posted")
	}
	if r := run(forge("OPEN", head), args("PASS")...); r.code != 0 || readFile(t, log) != r.stdout {
		t.Errorf("post exited %d, printed %q, posted %q: %s", r.code, r.stdout, readFile(t, log), r.stderr)
	}
}

// TestReviewEvalsCommand proves the CLI is wired to the evals: seed prints candidates and writes nothing, and score reads evals.jsonl from the Git directory, appends its summary, and refuses a second scoring.
func TestReviewEvalsCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "f.txt")
	git(t, dir, "commit", "-q", "-m", "feat: f")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("a\nB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "commit", "-q", "-am", "fix: b")
	status := git(t, dir, "status", "--porcelain")
	r := reviewRun(t, dir, data, "evals", "seed")
	if r.code != 0 || !strings.Contains(r.stdout, `"lines":"2"`) || git(t, dir, "status", "--porcelain") != status {
		t.Errorf("seed exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}

	common := git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	evals := filepath.Join(common, "harness-review", "evals")
	if err := os.MkdirAll(filepath.Join(evals, "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evals, "evals.jsonl"), []byte(`{"id":"b1","kind":"bug","expected":[{"path":"f.txt","lines":"2"}],"confirmed_by":"human"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := filepath.Join(evals, "runs", "2026-11-01.jsonl")
	if err := os.WriteFile(run, []byte(`{"case":"b1","path":"f.txt","lines":"2","severity":"bug","disposition":"confirmed"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := reviewRun(t, dir, data, "evals", "score", run, "--depth", "medium", "--model", "review-bugs=sonnet"); r.code != 0 || !strings.Contains(r.stdout, `"recall": 1`) {
		t.Errorf("score exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if raw, _ := os.ReadFile(run); !strings.Contains(string(raw), `"summary":true`) || !strings.Contains(string(raw), `"review-bugs":"sonnet"`) {
		t.Errorf("run file holds %q", raw)
	}
	if r := reviewRun(t, dir, data, "evals", "score", run); r.code != 2 {
		t.Errorf("a second scoring exited %d: %s", r.code, r.stderr)
	}
}
