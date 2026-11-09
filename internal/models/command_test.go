package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// codexListing is what the codex shim prints for `codex debug models`: two listed models and a hidden one.
const codexListing = `{"models":[{"slug":"gpt-6-luna","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"}]},{"slug":"gpt-6.1-sol","visibility":"list","supported_reasoning_levels":[{"effort":"low"},{"effort":"high"},{"effort":"ultra"}]},{"slug":"gpt-reserve","visibility":"hide","supported_reasoning_levels":[{"effort":"low"}]}]}`

// listingPath puts `opencode models` and `codex debug models` shims on PATH, so validation sees fixed model lists rather than this machine's. The codex shim answers only with CODEX_HOME at ~/.codex, which is where the harness keeps Codex.
func listingPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "/elsewhere")
	scripts := map[string]string{
		"opencode": "#!/bin/sh\ncase \"$1\" in models) printf 'opencode-go/glm-5.3\\nopencode-go/deepseek-v4-flash\\nopenai/gpt-5.6-sol\\n' ;; esac\n",
		"codex":    "#!/bin/sh\n[ \"$CODEX_HOME\" = \"" + home + "/.codex\" ] || exit 9\n[ \"$1 $2\" = \"debug models\" ] && printf '%s' '" + codexListing + "'\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
}

// TestSetRejectsInvalidValuesWithoutWriting settles which assignments are refused at the package seam; models_test.go keeps one command wiring case.
func TestSetRejectsInvalidValuesWithoutWriting(t *testing.T) {
	listingPath(t)
	const seed = `{"version": 1, "installed": {}, "files": [], "models": {"preset": "economy"}}` + "\n"
	for name, args := range map[string][]string{
		"unknown opencode id":       {"reviewer", "--client", "opencode", "--model", "openai/nope"},
		"unknown codex model":       {"reviewer", "--client", "codex", "--model", "gpt-nope"},
		"hidden codex model":        {"reviewer", "--client", "codex", "--model", "gpt-reserve"},
		"unsupported codex effort":  {"reviewer", "--client", "codex", "--model", "gpt-6-luna", "--effort", "ultra"},
		"codex provider id":         {"reviewer", "--client", "codex", "--model", "openai/gpt-6-luna"},
		"codex inherit with effort": {"reviewer", "--client", "codex", "--model", "inherit", "--effort", "high"},
		"unknown claude alias":      {"reviewer", "--client", "claude", "--model", "claudius"},
		"unsupported haiku effort":  {"reviewer", "--client", "claude", "--model", "haiku", "--effort", "high"},
		"unknown role":              {"planner", "--client", "claude", "--model", "opus"},
		"unknown preset":            {"--preset", "lavish"},
		"unknown client":            {"reviewer", "--client", "cursor", "--model", "opus"},
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

// TestSetAcceptsListedCodexModels covers the catalog path: a listed model at a supported effort, and inherit, which needs no listing.
func TestSetAcceptsListedCodexModels(t *testing.T) {
	listingPath(t)
	path := filepath.Join(t.TempDir(), "state.json")
	for _, args := range [][]string{
		{"reviewer", "--client", "codex", "--model", "gpt-6.1-sol", "--effort", "ultra"},
		{"executor", "--client", "codex", "--model", "inherit"},
	} {
		if err := run(path, append([]string{"set"}, args...), &strings.Builder{}); err != nil {
			t.Errorf("set %v: %v", args, err)
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

// A review lens follows the reviewer's assignment until it is set on its own, and a reset returns it to the reviewer's.
func TestLensFollowsTheReviewerUntilSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	show := func() string {
		var out strings.Builder
		if err := run(path, []string{"show"}, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if got := show(); !strings.Contains(got, "review-risk client=claude model=opus effort=low (reviewer)") {
		t.Errorf("an unset lens does not follow the reviewer:\n%s", got)
	}
	for _, args := range [][]string{
		{"set", "reviewer", "--client", "claude", "--model", "sonnet"},
		{"set", "review-spec", "--client", "claude", "--model", "haiku"},
	} {
		if err := run(path, args, &strings.Builder{}); err != nil {
			t.Fatal(err)
		}
	}
	got := show()
	for _, want := range []string{"review-risk client=claude model=sonnet effort=inherit (reviewer)", "review-spec client=claude model=haiku effort=inherit (override)"} {
		if !strings.Contains(got, want) {
			t.Errorf("show lacks %q:\n%s", want, got)
		}
	}
	if err := run(path, []string{"reset", "review-spec"}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	if got := show(); !strings.Contains(got, "review-spec client=claude model=sonnet effort=inherit (reviewer)") {
		t.Errorf("a reset lens does not return to the reviewer:\n%s", got)
	}
}
