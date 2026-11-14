package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorktreeAuditForgeCommand proves --forge is wired to the pull request read: the plain audit reaches no gh, and --forge adds the PR and INTO columns.
func TestWorktreeAuditForgeCommand(t *testing.T) {
	dir, _, run := prRepo(t)
	path := filepath.Join(filepath.Dir(dir), "open")
	git(t, dir, "worktree", "add", "-q", "-b", "feat/open", path)
	log := filepath.Join(t.TempDir(), "gh.log")
	forge := `"pr list") echo "$*" >> '` + log + `'; echo '[{"number":8,"state":"OPEN","headRefOid":"x"}]';;
`
	plain := run(forge, "worktree", "audit", "--into", branch)
	if plain.code != 0 || strings.Contains(plain.stdout, "PR") {
		t.Fatalf("plain audit exited %d: %s %s", plain.code, plain.stdout, plain.stderr)
	}
	if _, err := os.Stat(log); err == nil {
		t.Fatal("the plain audit reached gh")
	}
	r := run(forge, "worktree", "audit", "--into", branch, "--forge")
	if r.code != 0 || !strings.Contains(r.stdout, "PR") || !strings.Contains(r.stdout, "INTO") {
		t.Fatalf("audit --forge exited %d: %s %s", r.code, r.stdout, r.stderr)
	}
	found := false
	for _, line := range strings.Split(r.stdout, "\n") {
		found = found || strings.Contains(line, "#8 OPEN held") && strings.HasSuffix(line, path)
	}
	if !found {
		t.Errorf("no held row for %s:\n%s", path, r.stdout)
	}
}

// TestReviewPruneForgeCommand proves prune --dry-run --forge marks an open pull request's pr-<n> directory, removes nothing, and refuses --forge without --dry-run.
func TestReviewPruneForgeCommand(t *testing.T) {
	_, _, run := prRepo(t)
	for _, n := range []string{"7", "9"} {
		if r := run("", "review", "record", "--branch", "pr-"+n, "--depth", "medium", "--disposition", "clean", "--summary", "s"); r.code != 0 {
			t.Fatalf("record exited %d: %s", r.code, r.stderr)
		}
	}
	forge := `"pr view") if [ "$3" = 7 ]; then echo '{"state":"OPEN"}'; else echo '{"state":"MERGED"}'; fi;;
`
	r := run(forge, "review", "prune", "--dry-run", "--forge")
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if r.code != 0 || len(lines) != 2 || !strings.HasSuffix(lines[0], "pr-7\tpull request #7 is open") || !strings.HasSuffix(lines[1], "pr-9") {
		t.Fatalf("prune --dry-run --forge exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	for _, line := range lines {
		if _, err := os.Stat(strings.SplitN(line, "\t", 2)[0]); err != nil {
			t.Errorf("a dry run removed %s", line)
		}
	}
	if r := run(forge, "review", "prune", "--forge"); r.code != 2 {
		t.Errorf("--forge without --dry-run exited %d", r.code)
	}
}
