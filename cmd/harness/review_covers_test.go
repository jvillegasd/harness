package main

import (
	"encoding/json"
	"testing"
)

// TestReviewCoversCommand proves the CLI is wired to the coverage rule: a clean record covers its range, a deeper level is not covered, and a missing flag is a usage error.
func TestReviewCoversCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")
	head := git(t, dir, "rev-parse", "HEAD")
	if r := reviewRun(t, dir, data, "record", "--base", base, "--depth", "medium", "--disposition", "clean"); r.code != 0 {
		t.Fatalf("record exited %d: %s", r.code, r.stderr)
	}

	answer := func(level string) bool {
		t.Helper()
		r := reviewRun(t, dir, data, "covers", "--base", base, "--sha", head, "--level", level)
		var c struct{ Covered bool }
		if r.code != 0 || json.Unmarshal([]byte(r.stdout), &c) != nil {
			t.Fatalf("covers exited %d: %q %s", r.code, r.stdout, r.stderr)
		}
		return c.Covered
	}
	if !answer("medium") || answer("high") {
		t.Error("a clean medium record did not cover medium alone")
	}
	if r := reviewRun(t, dir, data, "covers", "--base", base, "--sha", head, "--level", "low"); r.code != 2 {
		t.Errorf("an unknown level exited %d", r.code)
	}
}
