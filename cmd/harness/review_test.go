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

// reviewRun runs the built binary in dir with an isolated data home, so the ledger lands under data.
func reviewRun(t *testing.T, dir, data string, args ...string) result {
	t.Helper()
	cmd := exec.Command(binary, append([]string{"review"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append(env(dir), "XDG_DATA_HOME="+data)...)
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
	git(t, dir, "switch", "-q", "-c", "work")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "change")

	p := reviewRun(t, dir, data, "path")
	file := strings.TrimSpace(p.stdout)
	if p.code != 0 || !strings.HasPrefix(file, filepath.Join(data, "harness", "reviews")) || !strings.HasSuffix(file, "ledger.jsonl") {
		t.Fatalf("path exited %d with %q", p.code, p.stdout)
	}

	add := func(disposition string) string {
		r := reviewRun(t, dir, data, "record", "--depth", "standard", "--axis", "bugs", "--path", "a.go", "--lines", "40-52", "--severity", "bug", "--summary", "s", "--disposition", disposition)
		if r.code != 0 {
			t.Fatalf("record exited %d: %s", r.code, r.stderr)
		}
		return strings.TrimSpace(r.stdout)
	}
	fixed, open := add("confirmed"), add("unverified")
	if r := reviewRun(t, dir, data, "record", "--id", fixed, "--disposition", "fixed", "--evidence", "abc123"); r.code != 0 {
		t.Fatalf("update exited %d: %s", r.code, r.stderr)
	}

	before, _ := os.ReadFile(file)
	for _, bad := range [][]string{
		{"record", "--depth", "standard", "--path", "a.go", "--lines", "40-x", "--severity", "bug", "--summary", "s", "--disposition", "confirmed"},
		{"record", "--id", "missing", "--disposition", "fixed"},
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

	git(t, dir, "commit", "-q", "--amend", "--allow-empty", "-m", "rewritten")
	got = listReview(t, dir, data)
	if len(got) != 2 || !got[1].Open || !got[1].Rewritten {
		t.Fatalf("after a rewrite the open finding must stay, marked rewritten: %+v", got)
	}
}
