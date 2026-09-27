package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// listingPath puts an `opencode models` shim on PATH, so validation sees a fixed model list rather than this machine's.
func listingPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in models) printf 'opencode-go/glm-5.3\\nopencode-go/deepseek-v4-flash\\nopenai/gpt-5.6-sol\\n' ;; esac\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

// TestSetRejectsInvalidValuesWithoutWriting settles which assignments are refused at the package seam; models_test.go keeps one command wiring case.
func TestSetRejectsInvalidValuesWithoutWriting(t *testing.T) {
	listingPath(t)
	const seed = `{"version": 1, "installed": {}, "files": [], "models": {"preset": "economy"}}` + "\n"
	for name, args := range map[string][]string{
		"unknown opencode id":      {"reviewer", "--client", "opencode", "--model", "openai/nope"},
		"unknown claude alias":     {"reviewer", "--client", "claude", "--model", "claudius"},
		"unsupported haiku effort": {"reviewer", "--client", "claude", "--model", "haiku", "--effort", "high"},
		"unknown role":             {"planner", "--client", "claude", "--model", "opus"},
		"unknown preset":           {"--preset", "lavish"},
		"unknown client":           {"reviewer", "--client", "cursor", "--model", "opus"},
	} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := run(path, append([]string{"set"}, args...), &strings.Builder{}); err == nil {
			t.Errorf("%s: set was accepted", name)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != seed {
			t.Errorf("%s: a rejected set rewrote the state file:\n%s", name, got)
		}
	}
}

// TestShowRejectsAStaleChoice names the reset when a stored block no longer resolves.
func TestShowRejectsAStaleChoice(t *testing.T) {
	listingPath(t)
	for name, block := range map[string]string{
		"unknown preset":   `{"preset": "lavish"}`,
		"malformed role":   `{"roles": {"executor": {"client": "claude", "model": "claudius"}}}`,
		"unknown role key": `{"roles": {"planner": {"client": "claude", "model": "opus"}}}`,
	} {
		path := filepath.Join(t.TempDir(), "state.json")
		if err := os.WriteFile(path, []byte(`{"version": 1, "installed": {}, "files": [], "models": `+block+"}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := run(path, []string{"show"}, &strings.Builder{})
		if err == nil || !strings.Contains(err.Error(), "harness models reset") {
			t.Errorf("%s: show returned %v, want the reset named", name, err)
		}
		if err := run(path, []string{"reset"}, &strings.Builder{}); err != nil {
			t.Errorf("%s: the recommended reset failed: %v", name, err)
		}
		if err := run(path, []string{"show"}, &strings.Builder{}); err != nil {
			t.Errorf("%s: show after reset failed: %v", name, err)
		}
	}
}

// TestSetAcceptsSonnetXhigh pins Sonnet 5's effort range, which Claude Code's docs list with xhigh.
func TestSetAcceptsSonnetXhigh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"version": 1, "installed": {}, "files": [], "models": {"preset": "economy"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(path, []string{"set", "reviewer", "--client", "claude", "--model", "sonnet", "--effort", "xhigh"}, &strings.Builder{}); err != nil {
		t.Fatalf("set sonnet xhigh: %v", err)
	}
}

// TestLoadDropsRetiredRoleOverrides keeps an install working after the reviewers merged: an override for a retired role is dropped, and a live one is kept.
func TestLoadDropsRetiredRoleOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	block := `{"roles": {"reviewer-b": {"client": "claude", "model": "opus"}, "review-refuter": {"client": "claude", "model": "opus"}, "reviewer": {"client": "claude", "model": "sonnet"}}}`
	if err := os.WriteFile(path, []byte(`{"version": 1, "installed": {}, "files": [], "models": `+block+"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Roles) != 1 || c.Roles["reviewer"].Model != "sonnet" {
		t.Errorf("Roles = %v, want only the reviewer override", c.Roles)
	}
}
