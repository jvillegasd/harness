package dash

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jvillegasd/harness/internal/locks"
)

func (m Model) render() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("harness dash"))
	b.WriteString("\n\n")
	worktreesPanel(&b, m.snap.Worktrees, m.snap.WorktreesErr)
	locksPanel(&b, m.snap.Locks, m.snap.LocksErr, m.snap.LocksAt, m.locksRunning)
	benchPanel(&b, m.snap.Bench, m.benchRunning, m.benchErr)
	trackerPanel(&b, m.snap.Tracker, m.trackerRunning)
	b.WriteString("r refresh bench   l refresh locks   t refresh tracker   q quit\n")
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

func locksPanel(b *strings.Builder, results []locks.LockResult, loadErr string, at time.Time, running bool) {
	if running {
		panel(b, "locks", []string{"verifying..."})
		return
	}
	if loadErr != "" {
		panel(b, "locks", []string{loadErr})
		return
	}
	if at.IsZero() {
		panel(b, "locks", []string{"(not verified)"})
		return
	}
	lines := make([]string, 1, len(results)+1)
	lines[0] = "verified " + time.Since(at).Round(time.Second).String() + " ago"
	if len(results) == 0 {
		lines = append(lines, "(none)")
	}
	for _, r := range results {
		line := r.Name + ": " + r.Status
		if r.Detail != "" {
			line += " (" + r.Detail + ")"
		}
		lines = append(lines, line)
	}
	panel(b, "locks", lines)
}

func benchPanel(b *strings.Builder, report *BenchReport, running bool, loadErr string) {
	if running {
		panel(b, "bench", []string{"refreshing..."})
		return
	}
	var lines []string
	if loadErr != "" {
		lines = append(lines, loadErr)
	}
	if report != nil {
		lines = append(lines, "age "+time.Since(report.At).Round(time.Second).String())
		for _, line := range strings.Split(strings.TrimRight(report.Report, "\n"), "\n") {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		lines = []string{"not computed; press r to compute"}
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
