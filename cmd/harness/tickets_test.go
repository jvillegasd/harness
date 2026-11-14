package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDraft writes one draft under the repository's draft directory.
func writeDraft(t *testing.T, dir, feature, name, title, blocked string) {
	t.Helper()
	path := filepath.Join(dir, "docs", "agents", "tickets", feature, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# " + title + "\n\n## What to build\n\nA thing.\n\n## Acceptance criteria\n\n- It works.\n\n## Seam\n\nThe package tests.\n\n## Blocked by\n\n" + blocked + "\n\n## Out of scope\n\nNothing.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTicketsCheckCommand proves the CLI is wired to the draft rules: a clean feature exits 0 silently, a broken one prints path:line problems and exits 1, and no draft directory is not a failure.
func TestTicketsCheckCommand(t *testing.T) {
	dir := repo(t)
	ok(t, harness(t, dir, "tickets", "check"), "check with no drafts")
	writeDraft(t, dir, "good", "01-a.md", "01 A", "none")
	writeDraft(t, dir, "bad", "01-a.md", "01 A", "02")
	if r := ok(t, harness(t, dir, "tickets", "check", "good"), "check good"); r.stdout != "" {
		t.Errorf("clean feature printed %q", r.stdout)
	}
	r := harness(t, filepath.Join(dir, "docs"), "tickets", "check")
	want := filepath.Join("docs", "agents", "tickets", "bad", "01-a.md") + ":17: blocked by 02, which is not a ticket of this feature\n"
	if r.code != 1 || r.stdout != want {
		t.Errorf("check every feature exited %d with %q, want 1 with %q", r.code, r.stdout, want)
	}
	if !strings.HasPrefix(r.stdout, "docs/") {
		t.Errorf("paths are not repository-relative from a subdirectory: %q", r.stdout)
	}
}
