package dash

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshTracker(), tick())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		next, cmd := m.press(msg.String())
		return next, cmd
	case localMsg:
		m.localRunning = false
		m.snap.Worktrees = msg.Worktrees
		m.snap.WorktreesErr = msg.WorktreesErr
		m.snap.Locks = msg.Locks
		m.snap.LocksErr = msg.LocksErr
		return m, nil
	case benchMsg:
		m.benchRunning = false
		m.benchErr = msg.err
		if msg.cache != nil {
			m.snap.Bench = msg.cache
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
type localMsg Snapshot
type benchMsg struct {
	cache *BenchCache
	err   string
}
type trackerMsg struct{ tracker Tracker }

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m Model) refreshLocal() tea.Cmd {
	dir := m.dir
	return func() tea.Msg { return localMsg(localData(dir)) }
}

func (m Model) refreshBench() tea.Cmd {
	dir := m.dir
	return func() tea.Msg { return computeBench(dir) }
}

func (m Model) refreshTracker() tea.Cmd {
	dir := m.dir
	return func() tea.Msg { return loadTracker(dir) }
}
