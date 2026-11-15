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
