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
