package worktree

import (
	"github.com/jvillegasd/harness/internal/ui"
	"testing"
)

func TestPlainState(t *testing.T) {
	for _, value := range []string{"safe", "dirty"} {
		if got := state(ui.Plain, value); got != value {
			t.Errorf("state %q: %q", value, got)
		}
	}
}
