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
	"github.com/jvillegasd/harness/internal/skills"
	"github.com/jvillegasd/harness/internal/state"
)

// --- collectors ----------------------------------------------------------------

// realCollectors is the subprocess-backed set the dashboard runs in production.
func realCollectors() collectors {
	return collectors{
		worktrees: loadWorktrees,
		states:    loadStates,
		skills:    loadSkills,
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

// harnessConfig is the file setup-harness writes, which marks a repository the dashboard can show.
const harnessConfig = "docs/agents/harness.json"

// errHarnessOnly fills the panels whose data exists only in the harness checkout.
var errHarnessOnly = errors.New("harness checkout only")

// projectRoot is the top of the git repository enclosing dir, which has to carry the setup-harness configuration.
func projectRoot(dir string) (string, error) {
	out, err := capture(dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("run harness dash inside a git repository: %v", err)
	}
	root := strings.TrimSpace(string(out))
	if _, err := os.Stat(filepath.Join(root, harnessConfig)); err != nil {
		return "", fmt.Errorf("%s has no %s; run setup-harness there first", root, harnessConfig)
	}
	return root, nil
}

// isHarness reports whether the dashboard's repository is the harness checkout itself.
func isHarness(dir checkout.Checkout) bool {
	c, err := checkout.Find(dir.Root)
	return err == nil && c.Root == dir.Root
}

func loadSkills(dir checkout.Checkout) ([]skills.LockResult, error) {
	if !isHarness(dir) {
		return nil, errHarnessOnly
	}
	return skills.Verify(dir)
}

// loadBench reports for the dashboard's resolved checkout through the shared
// reporting operation, so no report from another checkout can be shown.
func loadBench(dir checkout.Checkout) (string, error) {
	if !isHarness(dir) {
		return "", errHarnessOnly
	}
	split, err := headDate(dir.Root)
	if err != nil {
		return "", err
	}
	return bench.ReportFor(dir, split)
}

func loadTracker(dir checkout.Checkout) Tracker {
	config, err := readConfig(filepath.Join(dir.Root, harnessConfig))
	if err != nil {
		return Tracker{Err: err.Error()}
	}
	var statuses []string
	switch config.Tracker {
	case "github":
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
		for _, issue := range issues {
			for _, l := range issue.Labels {
				statuses = append(statuses, l.Name)
			}
		}
	case "local-markdown":
		statuses, err = localStatuses(filepath.Join(dir.Root, config.SpecHome))
		if err != nil {
			return Tracker{Err: err.Error()}
		}
	default:
		return Tracker{Err: fmt.Sprintf("tracker %q is not one the dashboard reads", config.Tracker)}
	}
	counts := map[string]int{}
	for _, status := range statuses {
		for _, key := range labelOrder {
			if status == config.Labels[key] {
				counts[key]++
			}
		}
	}
	tracker := Tracker{}
	for _, key := range labelOrder {
		tracker.Labels = append(tracker.Labels, LabelCount{Label: config.Labels[key], Count: counts[key]})
	}
	return tracker
}

// localStatuses reads the Status: line of every local-markdown ticket and spec under specHome, the whole lifecycle state that tracker keeps.
func localStatuses(specHome string) ([]string, error) {
	var files []string
	for _, pattern := range []string{"*/issues/*.md", "*/spec.md"} {
		matches, err := filepath.Glob(filepath.Join(specHome, pattern))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	var statuses []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if status, ok := strings.CutPrefix(line, "Status:"); ok {
				statuses = append(statuses, strings.TrimSpace(status))
				break
			}
		}
	}
	return statuses, nil
}

// harnessSettings is the part of docs/agents/harness.json the tracker panel reads.
type harnessSettings struct {
	Tracker  string            `json:"tracker"`
	SpecHome string            `json:"spec_home"`
	Labels   map[string]string `json:"labels"`
}

// readConfig reads the repository's tracker mode and lifecycle label mapping, requiring every key the tracker panel renders.
func readConfig(path string) (harnessSettings, error) {
	var doc harnessSettings
	data, err := os.ReadFile(path)
	if err != nil {
		return doc, err
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return doc, err
	}
	for _, key := range labelOrder {
		if doc.Labels[key] == "" {
			return doc, fmt.Errorf("%s: no label mapped for %q", path, key)
		}
	}
	return doc, nil
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
