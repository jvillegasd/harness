package models

import (
	"reflect"
	"testing"
)

// TestClaudeExecutorArgvKeepsTheParentsActions pins the one role configuration no rendered-agent golden covers: the claude executor's denies live in argv, not in its agent file.
func TestClaudeExecutorArgvKeepsTheParentsActions(t *testing.T) {
	got := launchArgs("executor", Assignment{Client: "claude", Model: "inherit"})
	want := []string{
		"claude", "--agent", "executor", "--permission-mode", "acceptEdits", "--allowedTools", "Bash", "--disallowedTools",
		"Bash(git commit:*)", "Bash(git push:*)", "Bash(git merge:*)", "Bash(git worktree remove:*)",
		"Bash(orca worktree rm:*)", "Bash(orca-ide worktree rm:*)", "Bash(orca-dev worktree rm:*)",
		"Bash(wt remove:*)", "Bash(wt merge:*)", "Bash(wt step commit:*)", "Bash(gh:*)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %v, want %v", got, want)
	}
}

// A reviewer holds no shell, so nothing is subtracted from its launch.
func TestReviewerArgvCarriesNoDenies(t *testing.T) {
	if got := launchArgs("reviewer", Assignment{Client: "claude", Model: "inherit"}); !reflect.DeepEqual(got, []string{"claude", "--agent", "reviewer"}) {
		t.Fatalf("argv = %v", got)
	}
}

// A Codex role starts as no named agent, so its argv carries the instructions, model, and effort, and the sandbox that keeps commits out while Orca stays reachable.
func TestCodexArgvCarriesInstructionsAndSandbox(t *testing.T) {
	got := codexLaunch(Assignment{Client: "codex", Model: "gpt-6.1-sol", Effort: "high"}, "read-search", "Say \"hi\" & <stop>\x7f\nNext line.")
	want := []string{
		"codex", "-m", "gpt-6.1-sol", "-c", `model_reasoning_effort="high"`,
		"-c", "features.apps=false", "-c", "features.plugins=false",
		"-c", `developer_instructions="Say \"hi\" & <stop>\u007f\nNext line."`,
		"--ask-for-approval", "never", "--sandbox", "workspace-write", "-c", "sandbox_workspace_write.network_access=true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	if got := codexLaunch(Assignment{Client: "codex", Model: "inherit"}, "implement", "x"); got[1] != "-c" || got[2] != `developer_instructions="x"` {
		t.Fatalf("inherit argv = %q", got)
	}
}
