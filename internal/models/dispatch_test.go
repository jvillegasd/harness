package models

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTerminalCreateTimeout proves the create bound fires even when a hung Orca leaves a child holding stdout, and that the reason says a terminal may be open instead of claiming a refusal.
func TestTerminalCreateTimeout(t *testing.T) {
	bin := t.TempDir()
	shim := "#!/bin/sh\ncase \"$1 $2\" in\n\"orchestration run-current\") echo '{\"ok\":true,\"result\":{\"run\":{\"id\":\"r-1\"}}}' ;;\n\"terminal create\") sleep 30 ;;\nesac\n"
	if err := os.WriteFile(filepath.Join(bin, "orca"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("ORCA_CLI_COMMAND", "orca")
	defer func(d time.Duration) { terminalCreateTimeout = d }(terminalCreateTimeout)
	terminalCreateTimeout = 100 * time.Millisecond

	start := time.Now()
	_, err := viaOrca(context.Background(), "executor", Assignment{"claude", "sonnet", ""}, "p", t.TempDir())
	var f failure
	if !errors.As(err, &f) || f.Reason != "timeout" || !strings.Contains(f.Detail, "terminal list --worktree path:") {
		t.Fatalf("got %v", err)
	}
	if took := time.Since(start); took > 10*time.Second {
		t.Fatalf("create bound took %s", took)
	}
}
