// Package dash renders the live terminal dashboard over the harness state, lock,
// bench, and tracker data the CLI already reads. It writes no harness state:
// only the bench refresh persists its own report under the user cache directory.
package dash

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/locks"
	"github.com/jvillegasd/harness/internal/state"
)

const usage = `Usage: harness dash

A live terminal dashboard: worktrees and their typed state, lock verification,
the cached bench result, and the tracker's lifecycle frontier. It writes no
harness state; only the bench refresh caches its report under the user cache.

Keys: r refresh bench, t refresh tracker, q quit.
`

// refreshEvery is how often the local panels reload from git config, wt list, and the lock bodies.
const refreshEvery = 3 * time.Second

// labelOrder is the lifecycle label order the tracker panel renders, keyed by the mapping keys in docs/agents/harness.json.
var labelOrder = []string{"blocked", "ready_for_agent", "in_progress", "ready_for_human"}

var (
	titleStyle = lipgloss.NewStyle().Bold(true)
	panelStyle = lipgloss.NewStyle().Bold(true).Underline(true)
)

// Snapshot is one fixed observation of every panel. Tests feed it directly; loaders fill it from the same packages the CLI uses.
type Snapshot struct {
	Worktrees    []Worktree
	WorktreesErr string
	Locks        []locks.LockResult
	LocksErr     string
	Bench        *BenchCache
	Tracker      Tracker
}

// Worktree is one entry from wt list with the branch's typed state. Err carries a state read failure so the row still renders.
type Worktree struct {
	Branch string
	Dirty  bool
	State  state.State
	Err    string
}

// BenchCache is the last cached bench report. Age is derived from At at render time, so it advances without another read.
type BenchCache struct {
	Report string
	At     time.Time
}

// Tracker is the lifecycle frontier: a count per mapped label, in labelOrder, or Err when gh is unauthenticated or offline.
type Tracker struct {
	Labels []LabelCount
	Err    string
}

// LabelCount is one lifecycle label and its open-ticket count.
type LabelCount struct {
	Label string
	Count int
}

// Model is the dashboard's whole state. Its rendering and key handling are pure, so seam-3 tests never touch terminal I/O.
type Model struct {
	dir            checkout.Checkout
	snap           Snapshot
	benchErr       string
	localRunning   bool
	benchRunning   bool
	trackerRunning bool
}

// New builds the model over one snapshot of a checkout rooted at root.
func New(root string, snap Snapshot) Model {
	return Model{dir: checkout.Checkout{Root: root}, snap: snap}
}

// Command runs `harness dash` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "harness dash: unknown argument %q; run harness dash --help\n", args[0])
		return 2
	}
	dir, err := checkout.Find(".")
	if err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	program := tea.NewProgram(New(dir.Root, loadLocal(dir)), tea.WithOutput(stdout), tea.WithInput(os.Stdin))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	return 0
}
