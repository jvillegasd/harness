package main

import (
	"strings"
	"testing"
)

// TestReviewStampCommand proves the CLI is wired to the stamp: a clean record prints the Reviewed line, and an open finding prints nothing and exits 1.
func TestReviewStampCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
	head := git(t, dir, "rev-parse", "HEAD")
	reviewRun(t, dir, data, "record", "--base", base, "--depth", "medium", "--disposition", "clean")

	if r := reviewRun(t, dir, data, "stamp"); r.code != 0 || strings.TrimSpace(r.stdout) != "Reviewed: "+base+".."+head {
		t.Fatalf("stamp exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
	reviewRun(t, dir, data, "record", "--depth", "medium", "--path", "a.go", "--severity", "bug", "--summary", "s", "--disposition", "confirmed")
	if r := reviewRun(t, dir, data, "stamp"); r.code != 1 || r.stdout != "" || !strings.Contains(r.stderr, "still open") {
		t.Errorf("an open finding exited %d with %q: %s", r.code, r.stdout, r.stderr)
	}
}
