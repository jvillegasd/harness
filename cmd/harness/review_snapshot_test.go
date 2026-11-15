package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewSnapshotCommand(t *testing.T) {
	dir, data := repo(t), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "owned.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "owned.txt")
	git(t, dir, "commit", "-q", "-m", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "owned.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := reviewRun(t, dir, data, "snapshot", "--base", base, "--", "owned.txt")
	if r.code != 0 {
		t.Fatalf("snapshot exited %d: %s", r.code, r.stderr)
	}
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 2 || !strings.HasSuffix(lines[0], ".patch") || len(lines[1]) != 64 {
		t.Fatalf("snapshot printed %q, want patch path and sha256 digest", r.stdout)
	}
	if _, err := os.Stat(lines[0]); err != nil {
		t.Fatalf("patch is missing: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "owned.txt"), []byte("base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := reviewRun(t, dir, data, "snapshot", "--base", base, "--", "owned.txt")
	if empty.code != 1 || empty.stderr == "" {
		t.Fatalf("empty snapshot exited %d with %q, want 1 and a diagnostic", empty.code, empty.stderr)
	}
}
