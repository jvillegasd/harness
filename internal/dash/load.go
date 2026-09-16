package dash

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvillegasd/harness/internal/bench"
	"github.com/jvillegasd/harness/internal/checkout"
	"github.com/jvillegasd/harness/internal/locks"
	"github.com/jvillegasd/harness/internal/state"
)

// --- collectors ----------------------------------------------------------------

// realCollectors is the subprocess-backed set the dashboard runs in production.
func realCollectors() collectors {
	return collectors{
		worktrees: loadWorktrees,
		states:    loadStates,
		locks:     loadLocks,
		bench:     loadBench,
		tracker:   loadTracker,
	}
}

func loadWorktrees(dir checkout.Checkout) ([]Worktree, string) {
	out, err := capture(dir.Root, "wt", "list", "--format=json", "--config-set", "list.json-schema=2")
	if err != nil {
		return nil, err.Error()
	}
	trees, err := parseWorktrees(out)
	if err != nil {
		return nil, err.Error()
	}
	return trees, ""
}

// loadStates resolves every displayed branch from one branch-state configuration
// read: listing the branch in wt list is what makes Worktrunk its state store.
func loadStates(dir checkout.Checkout, branches []string) (map[string]state.State, error) {
	snap, err := state.ReadSnapshot(dir.Root)
	if err != nil {
		return nil, err
	}
	resolved := make(map[string]state.State, len(branches))
	for _, name := range branches {
		resolved[name] = snap.State("worktrunk", name)
	}
	return resolved, nil
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

func loadLocks(dir checkout.Checkout) ([]locks.LockResult, error) {
	return locks.Verify(dir)
}

// loadBench reports for the dashboard's resolved checkout through the shared
// reporting operation, so no report from another checkout can be shown.
func loadBench(dir checkout.Checkout) (string, error) {
	split, err := headDate(dir.Root)
	if err != nil {
		return "", err
	}
	return bench.ReportFor(dir, split)
}

func loadTracker(dir checkout.Checkout) Tracker {
	labels, err := readLabels(filepath.Join(dir.Root, "docs", "agents", "harness.json"))
	if err != nil {
		return Tracker{Err: err.Error()}
	}
	out, err := capture(dir.Root, "gh", "issue", "list", "--state", "open", "--limit", "1000", "--json", "number,labels")
	if err != nil {
		return Tracker{Err: "gh unavailable: " + err.Error()}
	}
	var issues []struct {
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	if json.Unmarshal(out, &issues) != nil {
		return Tracker{Err: "gh: cannot parse issue list"}
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
	return tracker
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

func headDate(root string) (time.Time, error) {
	out, err := capture(root, "git", "log", "-1", "--format=%cs")
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse("2006-01-02", strings.TrimSpace(string(out)))
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
