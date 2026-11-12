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

// reviewRun runs the built binary in dir; the ledger lands in the repository's Git directory.
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

type listed struct {
	ID          string `json:"id"`
	Disposition string `json:"disposition"`
	Open        bool   `json:"open"`
	Rewritten   bool   `json:"rewritten"`
}

func listReview(t *testing.T, dir, data string) []listed {
	t.Helper()
	r := reviewRun(t, dir, data, "list")
	if r.code != 0 {
		t.Fatalf("list exited %d: %s", r.code, r.stderr)
	}
	var out []listed
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("list printed %q: %v", r.stdout, err)
	}
	return out
}

// TestReviewLedgerCommand proves the CLI is wired to the ledger rules: record, update by id, list, rejection without a write, and a rebase that keeps an open finding.
func TestReviewLedgerCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "switch", "-q", "-c", "work")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")

	p := reviewRun(t, dir, data, "path")
	file := strings.TrimSpace(p.stdout)
	common := git(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if p.code != 0 || file != filepath.Join(common, "harness-review", "branches", "work", "ledger.jsonl") {
		t.Fatalf("path exited %d with %q", p.code, p.stdout)
	}
	// A slash in a branch name stays inside one directory name.
	if o := reviewRun(t, dir, data, "path", "--branch", "feat/x"); strings.TrimSpace(o.stdout) != filepath.Join(common, "harness-review", "branches", "feat%2Fx", "ledger.jsonl") {
		t.Errorf("path --branch feat/x printed %q", o.stdout)
	}

	add := func(disposition string) string {
		r := reviewRun(t, dir, data, "record", "--depth", "standard", "--axis", "bugs", "--path", "a.go", "--lines", "40-52", "--severity", "bug", "--summary", "s", "--disposition", disposition)
		if r.code != 0 {
			t.Fatalf("record exited %d: %s", r.code, r.stderr)
		}
		return strings.TrimSpace(r.stdout)
	}
	fixed, open := add("confirmed"), add("unverified")
	reviewed := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "fix")
	if r := reviewRun(t, dir, data, "record", "--id", fixed, "--disposition", "fixed", "--evidence", "red test now green", "--commit", "HEAD"); r.code != 0 {
		t.Fatalf("update exited %d: %s", r.code, r.stderr)
	}

	before, _ := os.ReadFile(file)
	for _, bad := range [][]string{
		{"record", "--depth", "standard", "--path", "a.go", "--lines", "40-x", "--severity", "bug", "--summary", "s", "--disposition", "confirmed"},
		{"record", "--id", "missing", "--disposition", "fixed", "--evidence", "e", "--commit", "HEAD"},
		{"record", "--id", open, "--severity", "nit", "--disposition", "nit"},
		{"record", "--id", open, "--disposition", "fixed", "--evidence", "e"},
		// A fix is a commit after the reviewed one: the reviewed commit, an older one, or a name that does not resolve closes nothing.
		{"record", "--id", open, "--disposition", "fixed", "--evidence", "e", "--commit", reviewed},
		{"record", "--id", open, "--disposition", "fixed", "--evidence", "e", "--commit", base},
		{"record", "--id", open, "--disposition", "fixed", "--evidence", "e", "--commit", "no-such-rev"},
		// A proven finding is never dropped on an agent's word.
		{"record", "--id", fixed, "--disposition", "dropped", "--evidence", "e"},
	} {
		if r := reviewRun(t, dir, data, bad...); r.code != 2 || r.stderr == "" {
			t.Errorf("%v exited %d with %q, want 2 and a diagnostic", bad, r.code, r.stderr)
		}
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(before, after) {
		t.Error("a rejected record changed the ledger")
	}

	got := listReview(t, dir, data)
	if len(got) != 2 || got[0].ID != fixed || got[0].Open || got[1].ID != open || !got[1].Open || got[1].Rewritten {
		t.Fatalf("list = %+v", got)
	}

	git(t, dir, "switch", "-q", "-C", "work", base)
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "rewritten")
	got = listReview(t, dir, data)
	if len(got) != 2 || !got[0].Open || !got[1].Open || !got[1].Rewritten {
		t.Fatalf("after a rewrite the open finding must stay, marked rewritten, and a fix the head lost must reopen: %+v", got)
	}

	// Another worktree of the branch reads the same file, and another branch has its own.
	sibling := filepath.Join(t.TempDir(), "sibling")
	git(t, dir, "worktree", "add", "-q", "--detach", sibling)
	if s := reviewRun(t, sibling, data, "path", "--branch", "work"); strings.TrimSpace(s.stdout) != file {
		t.Errorf("a sibling worktree uses %q, want %q", s.stdout, file)
	}
	if s := reviewRun(t, sibling, data, "list", "--branch", "work"); !strings.Contains(s.stdout, fixed) {
		t.Errorf("a sibling worktree does not see the branch's findings: %s", s.stdout)
	}
	git(t, dir, "switch", "-q", "-c", "other")
	if got := listReview(t, dir, data); len(got) != 0 {
		t.Errorf("another branch sees %d findings, want its own empty ledger", len(got))
	}
	// A detached head has no branch to key a ledger by.
	if s := reviewRun(t, sibling, data, "path"); s.code != 2 || s.stderr == "" {
		t.Errorf("path on a detached head exited %d with %q, want 2 and a diagnostic", s.code, s.stderr)
	}
}

// TestReviewRemoveCommand proves the CLI is wired to the removal rule: the named branch's directory goes, another branch's stays, a second run is a no-op, and a recorded worktree is refused. The rule itself is settled in internal/reviewledger.
func TestReviewRemoveCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	record := func(branch string) string {
		if r := reviewRun(t, dir, data, "record", "--branch", branch, "--depth", "medium", "--disposition", "clean", "--summary", "s"); r.code != 0 {
			t.Fatalf("record exited %d: %s", r.code, r.stderr)
		}
		return filepath.Dir(strings.TrimSpace(reviewRun(t, dir, data, "path", "--branch", branch).stdout))
	}
	merged, kept := record("feat/x"), record("feat/y")

	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/x"); r.code != 0 || strings.TrimSpace(r.stdout) != merged {
		t.Fatalf("remove exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	if _, err := os.Stat(merged); !os.IsNotExist(err) {
		t.Errorf("the removed branch's directory is still there: %v", err)
	}
	if _, err := os.Stat(filepath.Join(kept, "ledger.jsonl")); err != nil {
		t.Errorf("another branch's ledger was touched: %v", err)
	}
	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/x"); r.code != 0 || r.stdout != "" {
		t.Errorf("a second remove exited %d with %q, want 0 and nothing", r.code, r.stdout)
	}

	if err := os.WriteFile(filepath.Join(kept, "worktree.txt"), []byte("/tmp/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := reviewRun(t, dir, data, "remove", "--branch", "feat/y"); r.code != 1 || r.stderr == "" {
		t.Errorf("remove with a recorded worktree exited %d with %q, want 1 and a diagnostic", r.code, r.stderr)
	}
	if _, err := os.Stat(filepath.Join(kept, "ledger.jsonl")); err != nil {
		t.Errorf("a refused remove deleted the ledger: %v", err)
	}
}

// TestReviewLevelCommand proves the CLI is wired to the level rules, and that an install keeps the stored level. Which values are accepted is settled in internal/reviewledger.
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
