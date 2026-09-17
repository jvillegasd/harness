package dash

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jvillegasd/harness/internal/skills"
)

func (m Model) Init() tea.Cmd {
	// Order is fixed: local, skills, tracker, then the periodic tick.
	return tea.Batch(m.refreshLocal(), m.refreshSkills(), m.refreshTracker(), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		next, cmd := m.press(msg.String())
		return next, cmd
	case localMsg:
		m.localRunning = false
		m.snap.Worktrees = msg.worktrees
		m.snap.WorktreesErr = msg.worktreesErr
		return m, nil
	case skillsMsg:
		m.skillsRunning = false
		m.snap.Skills = msg.results
		m.snap.SkillsErr = msg.err
		m.snap.SkillsAt = msg.at
		return m, nil
	case benchMsg:
		m.benchRunning = false
		m.benchErr = msg.err
		if msg.report != nil {
			m.snap.Bench = msg.report
		}
		return m, nil
	case trackerMsg:
		m.trackerRunning = false
		m.snap.Tracker = msg.tracker
		return m, nil
	case tickMsg:
		if m.localRunning {
			return m, tick()
		}
		m.localRunning = true
		return m, tea.Batch(m.refreshLocal(), tick())
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// press handles one key by name. It is separate from Update so tests assert key handling without building terminal key messages.
func (m Model) press(key string) (Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "r":
		if m.benchRunning {
			return m, nil
		}
		m.benchRunning = true
		m.benchErr = ""
		return m, m.refreshBench()
	case "s":
		if m.skillsRunning {
			return m, nil
		}
		m.skillsRunning = true
		return m, m.refreshSkills()
	case "t":
		if m.trackerRunning {
			return m, nil
		}
		m.trackerRunning = true
		m.snap.Tracker = Tracker{}
		return m, m.refreshTracker()
	}
	return m, nil
}

// --- messages and commands -----------------------------------------------------

type tickMsg time.Time
type localMsg struct {
	worktrees    []Worktree
	worktreesErr string
}
type skillsMsg struct {
	results []skills.LockResult
	err     string
	at      time.Time
}
type benchMsg struct {
	report *BenchReport
	err    string
}
type trackerMsg struct{ tracker Tracker }

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// refreshLocal lists the worktrees and resolves every displayed branch from one
// branch-state configuration read.
func (m Model) refreshLocal() tea.Cmd {
	dir, cols := m.dir, m.cols
	return func() tea.Msg {
		trees, loadErr := cols.worktrees(dir)
		if loadErr != "" {
			return localMsg{worktreesErr: loadErr}
		}
		branches := make([]string, len(trees))
		for i := range trees {
			branches[i] = trees[i].Branch
		}
		states, err := cols.states(dir, branches)
		if err != nil {
			if len(trees) == 0 {
				return localMsg{worktreesErr: err.Error()}
			}
			for i := range trees {
				trees[i].Err = firstLine(err.Error())
			}
			return localMsg{worktrees: trees}
		}
		for i := range trees {
			trees[i].State = states[trees[i].Branch]
		}
		return localMsg{worktrees: trees}
	}
}

func (m Model) refreshSkills() tea.Cmd {
	dir, cols := m.dir, m.cols
	return func() tea.Msg {
		results, err := cols.skills(dir)
		msg := skillsMsg{results: results, at: time.Now()}
		if err != nil {
			msg.err = err.Error()
		}
		return msg
	}
}

func (m Model) refreshBench() tea.Cmd {
	dir, cols := m.dir, m.cols
	return func() tea.Msg {
		report, err := cols.bench(dir)
		if err != nil {
			return benchMsg{err: "bench: " + firstLine(err.Error())}
		}
		return benchMsg{report: &BenchReport{Report: report, At: time.Now()}}
	}
}

func (m Model) refreshTracker() tea.Cmd {
	dir, cols := m.dir, m.cols
	return func() tea.Msg { return trackerMsg{tracker: cols.tracker(dir)} }
}
