// Package dash renders the live terminal dashboard over the harness state, lock,
// bench, and tracker data the CLI already reads. It writes no harness state:
// only the bench refresh persists its own report under the user cache directory.
package dash

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jvillegasd/harness/internal/bench"
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
	root           string
	snap           Snapshot
	benchErr       string
	localRunning   bool
	benchRunning   bool
	trackerRunning bool
}

// New builds the model over one snapshot.
func New(root string, snap Snapshot) Model {
	return Model{root: root, snap: snap}
}

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

func (m Model) render() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("harness dash"))
	b.WriteString("\n\n")
	worktreesPanel(&b, m.snap.Worktrees, m.snap.WorktreesErr)
	locksPanel(&b, m.snap.Locks, m.snap.LocksErr)
	benchPanel(&b, m.snap.Bench, m.benchRunning, m.benchErr)
	trackerPanel(&b, m.snap.Tracker, m.trackerRunning)
	b.WriteString("r refresh bench   t refresh tracker   q quit\n")
	return b.String()
}

func panel(b *strings.Builder, name string, lines []string) {
	b.WriteString(panelStyle.Render(name))
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
}

func worktreesPanel(b *strings.Builder, trees []Worktree, loadErr string) {
	if loadErr != "" {
		panel(b, "worktrees", []string{loadErr})
		return
	}
	if len(trees) == 0 {
		panel(b, "worktrees", []string{"(none)"})
		return
	}
	sorted := append([]Worktree(nil), trees...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ei, ai := ranks(sorted[i])
		ej, aj := ranks(sorted[j])
		if ei != ej {
			return ei > ej
		}
		if ai != aj {
			return ai > aj
		}
		return sorted[i].Branch < sorted[j].Branch
	})
	lines := []string{fmt.Sprintf("%-28s %-5s %-16s %-9s %-16s %s", "branch", "dirty", "escalations", "attempts", "ladder", "reason")}
	for _, w := range sorted {
		ladder, reason := w.State.Ladder, w.State.Reason
		if ladder == "" {
			ladder = "-"
		}
		if reason == "" {
			reason = "-"
		}
		if w.Err != "" {
			reason = "state error: " + w.Err
		}
		lines = append(lines, fmt.Sprintf("%-28s %-5s %-16s %-9s %-16s %s",
			clip(w.Branch, 28), yesNo(w.Dirty),
			counterCell(w.State.Escalations, w.State.Legacy["escalations"]),
			counterCell(w.State.Attempts, w.State.Legacy["attempts"]),
			clip(ladder, 16), reason))
	}
	panel(b, "worktrees", lines)
}

// ranks orders worktrees by escalations then attempts, highest first. A legacy or absent counter is unknown: legacy sorts below zero, absent counts as zero.
func ranks(w Worktree) (escalations, attempts int) {
	return rank(w.State.Escalations, w.State.Legacy["escalations"]), rank(w.State.Attempts, w.State.Legacy["attempts"])
}

func rank(n *int, legacy string) int {
	if n != nil {
		return *n
	}
	if legacy != "" {
		return -1
	}
	return 0
}

// counterCell renders a typed counter, or marks a legacy value in place so an uninterpretable number is visibly not a number. It stays plain text so the row's byte padding holds.
func counterCell(n *int, legacy string) string {
	if legacy != "" {
		return "legacy:" + clip(legacy, 8)
	}
	if n == nil {
		return "-"
	}
	return strconv.Itoa(*n)
}

// clip bounds one cell so a long branch name or legacy text cannot push the rest of the row out of alignment.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "~"
}

func locksPanel(b *strings.Builder, results []locks.LockResult, loadErr string) {
	if loadErr != "" {
		panel(b, "locks", []string{loadErr})
		return
	}
	if len(results) == 0 {
		panel(b, "locks", []string{"(none)"})
		return
	}
	lines := make([]string, 0, len(results))
	for _, r := range results {
		line := r.Name + ": " + r.Status
		if r.Detail != "" {
			line += " (" + r.Detail + ")"
		}
		lines = append(lines, line)
	}
	panel(b, "locks", lines)
}

func benchPanel(b *strings.Builder, cache *BenchCache, running bool, loadErr string) {
	if running {
		panel(b, "bench", []string{"refreshing..."})
		return
	}
	var lines []string
	if loadErr != "" {
		lines = append(lines, loadErr)
	}
	if cache != nil {
		lines = append(lines, "age "+time.Since(cache.At).Round(time.Second).String())
		for _, line := range strings.Split(strings.TrimRight(cache.Report, "\n"), "\n") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{"no cached result; press r to compute"}
	}
	panel(b, "bench", lines)
}

func trackerPanel(b *strings.Builder, t Tracker, running bool) {
	if t.Err != "" {
		panel(b, "tracker", []string{t.Err})
		return
	}
	if running || t.Labels == nil {
		panel(b, "tracker", []string{"querying..."})
		return
	}
	cells := make([]string, 0, len(t.Labels))
	for _, l := range t.Labels {
		cells = append(cells, fmt.Sprintf("%s: %d", l.Label, l.Count))
	}
	panel(b, "tracker", []string{strings.Join(cells, "   ")})
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
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
	root := m.root
	return func() tea.Msg { return localMsg(localData(root)) }
}

func (m Model) refreshBench() tea.Cmd {
	root := m.root
	return func() tea.Msg { return computeBench(root) }
}

func (m Model) refreshTracker() tea.Cmd {
	root := m.root
	return func() tea.Msg { return loadTracker(root) }
}

// --- loaders -------------------------------------------------------------------

func loadLocal(root string) Snapshot {
	snap := localData(root)
	snap.Bench = loadBenchCache()
	return snap
}

// localData loads the panels that reload on the tick. The bench cache is not among them: its age is derived from its timestamp at render time.
func localData(root string) Snapshot {
	snap := Snapshot{}
	snap.Worktrees, snap.WorktreesErr = loadWorktrees(root)
	snap.Locks, snap.LocksErr = loadLocks(root)
	return snap
}

func loadWorktrees(root string) ([]Worktree, string) {
	out, err := capture(root, "wt", "list", "--format=json", "--config-set", "list.json-schema=2")
	if err != nil {
		return nil, err.Error()
	}
	trees, err := parseWorktrees(out)
	if err != nil {
		return nil, err.Error()
	}
	for i := range trees {
		// listing the branch in wt list is what makes Worktrunk its state store; a legacy harness.* value still reads through the fallback.
		br := &state.Branch{Dir: root, Name: trees[i].Branch, Store: "worktrunk"}
		if st, err := br.Get(); err != nil {
			trees[i].Err = err.Error()
		} else {
			trees[i].State = st
		}
	}
	return trees, ""
}

// parseWorktrees reads a `wt list --format=json --config-set list.json-schema=2` listing. The dirty flags come from the worktree.changes object.
func parseWorktrees(data []byte) ([]Worktree, error) {
	var listing struct {
		Items []struct {
			Branch   string `json:"branch"`
			Worktree struct {
				Changes struct {
					Staged     bool `json:"staged"`
					Modified   bool `json:"modified"`
					Untracked  bool `json:"untracked"`
					Renamed    bool `json:"renamed"`
					Deleted    bool `json:"deleted"`
					Conflicted bool `json:"conflicted"`
				} `json:"changes"`
			} `json:"worktree"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, err
	}
	trees := make([]Worktree, 0, len(listing.Items))
	for _, item := range listing.Items {
		c := item.Worktree.Changes
		trees = append(trees, Worktree{
			Branch: item.Branch,
			Dirty:  c.Staged || c.Modified || c.Untracked || c.Renamed || c.Deleted || c.Conflicted,
		})
	}
	return trees, nil
}

func loadLocks(root string) ([]locks.LockResult, string) {
	results, err := locks.Verify(root)
	if err != nil {
		return nil, err.Error()
	}
	return results, ""
}

func computeBench(root string) benchMsg {
	split, err := headDate(root)
	if err != nil {
		return benchMsg{err: "bench: " + err.Error()}
	}
	var out, errText bytes.Buffer
	if code := bench.Command([]string{"--split", split}, &out, &errText); code != 0 {
		msg := strings.TrimSpace(errText.String())
		if msg == "" {
			msg = "bench exited non-zero"
		}
		return benchMsg{err: "bench: " + firstLine(msg)}
	}
	now := time.Now()
	report := out.String()
	if err := writeBenchCache(now, report); err != nil {
		return benchMsg{cache: &BenchCache{Report: report, At: now}, err: "bench cache write failed: " + err.Error()}
	}
	return benchMsg{cache: &BenchCache{Report: report, At: now}}
}

func loadTracker(root string) trackerMsg {
	labels, err := readLabels(filepath.Join(root, "docs", "agents", "harness.json"))
	if err != nil {
		return trackerMsg{tracker: Tracker{Err: err.Error()}}
	}
	out, err := capture(root, "gh", "issue", "list", "--state", "open", "--limit", "1000", "--json", "number,labels")
	if err != nil {
		return trackerMsg{tracker: Tracker{Err: "gh unavailable: " + err.Error()}}
	}
	var issues []struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if json.Unmarshal(out, &issues) != nil {
		return trackerMsg{tracker: Tracker{Err: "gh: cannot parse issue list"}}
	}
	counts := map[string]int{}
	for _, issue := range issues {
		for _, l := range issue.Labels {
			for _, key := range labelOrder {
				if l.Name == labels[key] {
					counts[key]++
				}
			}
		}
	}
	tracker := Tracker{}
	for _, key := range labelOrder {
		tracker.Labels = append(tracker.Labels, LabelCount{Label: labels[key], Count: counts[key]})
	}
	return trackerMsg{tracker: tracker}
}

// readLabels reads the repository's lifecycle label mapping and requires every key the tracker panel renders.
func readLabels(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	for _, key := range labelOrder {
		if doc.Labels[key] == "" {
			return nil, fmt.Errorf("%s: no label mapped for %q", path, key)
		}
	}
	return doc.Labels, nil
}

// --- bench cache ---------------------------------------------------------------

type cacheDoc struct {
	At     time.Time `json:"at"`
	Report string    `json:"report"`
}

func benchCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "harness", "dash-bench.json"), nil
}

func loadBenchCache() *BenchCache {
	path, err := benchCachePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc cacheDoc
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	return &BenchCache{Report: doc.Report, At: doc.At}
}

func writeBenchCache(at time.Time, report string) error {
	path, err := benchCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cacheDoc{At: at, Report: report})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// --- process and checkout helpers ----------------------------------------------

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
	root, err := findRoot()
	if err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	program := tea.NewProgram(New(root, loadLocal(root)), tea.WithOutput(stdout), tea.WithInput(os.Stdin))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(stderr, "harness dash: %v\n", err)
		return 1
	}
	return 0
}

// findRoot walks up to the checkout holding harness/POLICY.md, so the dashboard reads the working directory's checkout.
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "harness", "POLICY.md")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("run harness dash from inside the harness checkout")
		}
		dir = parent
	}
}

func headDate(root string) (string, error) {
	out, err := capture(root, "git", "log", "-1", "--format=%cs")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// capture runs a helper and returns its stdout, folding stderr's first line into any error so a panel shows one actionable line.
func capture(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(firstLine(msg))
	}
	return out, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
