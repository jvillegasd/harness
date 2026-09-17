// Package dash renders the live terminal dashboard over the harness state, skills,
// bench, and tracker data the CLI already reads. It writes no harness state:
// every result lives only for the dashboard session.
package dash

import (
	"fmt"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/skills"
	"github.com/jvillegasd/harness/internal/state"
)

const usage = `Usage: harness dash

A live terminal dashboard for the repository you are in, once setup-harness has
written its docs/agents/harness.json: worktrees and their typed state, and the
tracker's lifecycle frontier from GitHub issues or local-markdown tickets. Skills
verification and the bench result show inside the harness checkout only. It
writes no harness state; every result lives only for the dashboard session.

Keys: r refresh bench, s refresh skills, t refresh tracker, q quit.
`

// refreshEvery is how often the local panels reload from git config and wt list.
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
	Skills       []skills.LockResult
	SkillsErr    string
	SkillsAt     time.Time
	Bench        *BenchReport
	Tracker      Tracker
}

// Worktree is one entry from wt list with the branch's typed state. Err carries a state read failure so the row still renders.
type Worktree struct {
	Branch string
	Dirty  bool
	State  state.State
	Err    string
}

// BenchReport is this session's bench result. At is when it was computed; the age derives from it at render time.
type BenchReport struct {
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

// collectors are the dashboard's outside reads, so a recording test can assert
// the model's work without spawning git, wt, or gh.
type collectors struct {
	worktrees func(checkout.Checkout) ([]Worktree, string)
	states    func(checkout.Checkout, []string) (map[string]state.State, error)
	skills    func(checkout.Checkout) ([]skills.LockResult, error)
	bench     func(checkout.Checkout) (string, error)
	tracker   func(checkout.Checkout) Tracker
}

// Model is the dashboard's whole state. Its rendering and key handling are pure, so seam-3 tests never touch terminal I/O.
type Model struct {
	dir            checkout.Checkout
	cols           collectors
	snap           Snapshot
	benchErr       string
	localRunning   bool
	skillsRunning  bool
	benchRunning   bool
	trackerRunning bool
}

// New builds the model over one snapshot of a checkout rooted at root. The model
// starts with the startup loads in flight, so Init must run for the local, skills,
// and tracker panels to fill.
func New(root string, snap Snapshot) Model {
	return newModel(root, snap, realCollectors())
}

// newModel is the seam tests use to record the dashboard's outside reads. The
// three in-flight guards are the startup loads Init issues: until a load's
// result arrives its key is ignored, and the skills and tracker panels show the
// load in progress.
func newModel(root string, snap Snapshot, cols collectors) Model {
	return Model{
		dir:            checkout.Checkout{Root: root},
		cols:           cols,
		snap:           snap,
		localRunning:   true,
		skillsRunning:  true,
		trackerRunning: true,
	}
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
	root, err := projectRoot(".")
	if err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	program := tea.NewProgram(New(root, Snapshot{}), tea.WithOutput(stdout), tea.WithInput(os.Stdin))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	return 0
}
