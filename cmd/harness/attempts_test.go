package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAttemptsCommandWiring(t *testing.T) {
	dir := repo(t)
	log := filepath.Join(t.TempDir(), "attempts.tsv")
	args := []string{"attempts", "add", "--log", log, "--hypothesis", "test", "--change", "change", "--before", "10", "--after", "8", "--gate", "pass", "--outcome", "kept"}
	if result := harness(t, dir, args...); result.code != 0 {
		t.Fatalf("add exited %d: %s", result.code, result.stderr)
	}

	bad := append([]string(nil), args...)
	for i := range bad {
		if bad[i] == "pass" {
			bad[i] = "unknown"
			break
		}
	}
	if result := harness(t, dir, bad...); result.code != 2 {
		t.Fatalf("rejected add exited %d, want 2: %s", result.code, result.stderr)
	}
	result := harness(t, dir, "attempts", "list", "--log", log, "--json")
	if result.code != 0 || !strings.Contains(result.stdout, `"before":10`) || !strings.Contains(result.stdout, `"delta":-2`) {
		t.Fatalf("list --json => %d, %q, %q", result.code, result.stdout, result.stderr)
	}
}
