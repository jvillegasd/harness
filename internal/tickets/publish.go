package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Forge runs one gh command and returns its stdout.
type Forge func(args ...string) ([]byte, error)

// Config is the part of docs/agents/harness.json publishing reads.
type Config struct {
	Tracker string            `json:"tracker"`
	Labels  map[string]string `json:"labels"`
}

// ConfigPath is the repository's tracker configuration.
var ConfigPath = filepath.Join("docs", "agents", "harness.json")

// ReadConfig reads the tracker configuration at root.
func ReadConfig(root string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(filepath.Join(root, ConfigPath))
	if errors.Is(err, fs.ErrNotExist) {
		return c, fmt.Errorf("no tracker configuration at %s", ConfigPath)
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("%s: %w", ConfigPath, err)
	}
	return c, nil
}

func (c Config) label(key, fallback string) string {
	if v := c.Labels[key]; v != "" {
		return v
	}
	return fallback
}

// Order returns the tickets blockers first, ties by number. The tickets have passed Check, so the edges form no cycle.
func Order(ts []Ticket) []Ticket {
	byNumber := map[int]Ticket{}
	for _, t := range ts {
		byNumber[t.Number] = t
	}
	numbers := make([]int, 0, len(ts))
	for _, t := range ts {
		numbers = append(numbers, t.Number)
	}
	sort.Ints(numbers)
	placed := map[int]bool{}
	var out []Ticket
	var place func(int)
	place = func(n int) {
		if placed[n] {
			return
		}
		placed[n] = true
		for _, b := range byNumber[n].BlockedBy {
			place(b)
		}
		out = append(out, byNumber[n])
	}
	for _, n := range numbers {
		place(n)
	}
	return out
}

// Body renders a draft's five sections as an issue body, with Blocked by naming the published issues.
func Body(t Ticket, issues map[int]int) string {
	var parts []string
	for _, name := range Sections {
		text := t.Sections[name].Body
		if name == "Blocked by" {
			text = "none"
			if len(t.BlockedBy) > 0 {
				refs := make([]string, len(t.BlockedBy))
				for i, b := range t.BlockedBy {
					refs[i] = "#" + strconv.Itoa(issues[b])
				}
				text = strings.Join(refs, ", ")
			}
		}
		parts = append(parts, "## "+name+"\n\n"+text)
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// Label is the mapped label a new issue takes: ready only with no blocker, since every blocker is a ticket this run publishes open.
func Label(c Config, t Ticket) string {
	if len(t.BlockedBy) == 0 {
		return c.label("ready_for_agent", "ready-for-agent")
	}
	return c.label("blocked", "blocked")
}

// Plan prints the exact write set: each issue to create with its label, and each blocking edge.
func Plan(c Config, ts []Ticket, out io.Writer) {
	for _, t := range Order(ts) {
		if t.Issue != "" {
			fmt.Fprintf(out, "exists %02d %s\n", t.Number, t.Issue)
			continue
		}
		fmt.Fprintf(out, "create %02d %q label %s\n", t.Number, t.Title, Label(c, t))
	}
	for _, t := range Order(ts) {
		for _, b := range t.BlockedBy {
			fmt.Fprintf(out, "edge   %02d blocked by %02d\n", t.Number, b)
		}
	}
}

var issueURLRe = regexp.MustCompile(`/issues/([0-9]+)\s*$`)

func issueNumber(url string) (int, error) {
	m := issueURLRe.FindStringSubmatch(url)
	if m == nil {
		return 0, fmt.Errorf("%q is not an issue URL", url)
	}
	return strconv.Atoi(m[1])
}

// Published is what a publish run did.
type Published struct {
	Created  []string // issue URLs this run created
	Issues   []string // every ticket's issue URL, in publish order
	Kept     []string // draft paths whose issue did not match on read-back
	Removed  []string // draft paths removed once their issue matched
	Remains  []string // draft paths not yet published when the run stopped
	Mismatch []string // why each kept draft did not match
}

// Publish creates each draft's issue blockers first, records it in the draft at once, adds each missing blocking edge, reads every issue back, and removes the drafts whose issue matched. It stops at the first forge failure, leaving every draft in place, and a rerun skips the issues the drafts already record.
func Publish(c Config, root string, ts []Ticket, forge Forge) (Published, error) {
	var p Published
	issues := map[int]int{}
	order := Order(ts)
	matched := map[int]bool{}
	for i, t := range order {
		stop := func(err error) (Published, error) {
			for _, rest := range order[i:] {
				p.Remains = append(p.Remains, rest.Path)
			}
			return p, fmt.Errorf("%s: %w", t.Path, err)
		}
		url := t.Issue
		if url == "" {
			out, err := forge("issue", "create", "--title", t.Title, "--body", Body(t, issues), "--label", Label(c, t))
			if err != nil {
				return stop(err)
			}
			url = strings.TrimSpace(string(out))
			if _, err := issueNumber(url); err != nil {
				return stop(err)
			}
			if err := record(filepath.Join(root, t.Path), url); err != nil {
				return stop(fmt.Errorf("created %s but could not record it: %w", url, err))
			}
			p.Created = append(p.Created, url)
		}
		n, err := issueNumber(url)
		if err != nil {
			return stop(err)
		}
		issues[t.Number] = n
		p.Issues = append(p.Issues, url)
		if err := edges(forge, n, t.BlockedBy, issues); err != nil {
			return stop(err)
		}
		why, err := readBack(forge, n, t.Title, Body(t, issues), Label(c, t), t.BlockedBy, issues)
		if err != nil {
			return stop(err)
		}
		if why != "" {
			p.Kept = append(p.Kept, t.Path)
			p.Mismatch = append(p.Mismatch, fmt.Sprintf("%s: %s: %s", t.Path, url, why))
			continue
		}
		matched[t.Number] = true
	}
	// A kept draft still names its blockers by draft number, so their drafts stay until it is published too.
	needed := map[int]bool{}
	for _, t := range order {
		if !matched[t.Number] {
			for _, b := range t.BlockedBy {
				needed[b] = true
			}
		}
	}
	for _, t := range order {
		if matched[t.Number] && !needed[t.Number] {
			if err := os.Remove(filepath.Join(root, t.Path)); err != nil {
				return p, err
			}
			p.Removed = append(p.Removed, t.Path)
		}
	}
	if len(order) > 0 && len(p.Removed) == len(order) {
		os.Remove(filepath.Dir(filepath.Join(root, order[0].Path))) // only when now empty
	}
	return p, nil
}

// record writes the issue URL into the draft under its title line.
func record(path, url string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.SplitAfter(string(raw), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			if !strings.HasSuffix(line, "\n") {
				lines[i] += "\n"
			}
			lines = append(lines[:i+1], append([]string{"\nIssue: " + url + "\n"}, lines[i+1:]...)...)
			break
		}
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "")), 0o644)
}

const repoPath = "repos/{owner}/{repo}/issues/"

// blockedBy reads the issue numbers an issue is natively blocked by.
func blockedBy(forge Forge, n int) (map[int]bool, error) {
	out, err := forge("api", repoPath+strconv.Itoa(n)+"/dependencies/blocked_by")
	if err != nil {
		return nil, err
	}
	var list []struct{ Number int }
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("reading #%d's blockers: %w", n, err)
	}
	have := map[int]bool{}
	for _, b := range list {
		have[b.Number] = true
	}
	return have, nil
}

// edges adds each blocking edge the issue lacks, sending the blocker's database id.
func edges(forge Forge, n int, blockers []int, issues map[int]int) error {
	if len(blockers) == 0 {
		return nil
	}
	have, err := blockedBy(forge, n)
	if err != nil {
		return err
	}
	for _, b := range blockers {
		issue := issues[b]
		if have[issue] {
			continue
		}
		out, err := forge("api", repoPath+strconv.Itoa(issue))
		if err != nil {
			return err
		}
		var blocker struct{ ID int64 }
		if err := json.Unmarshal(out, &blocker); err != nil || blocker.ID == 0 {
			return fmt.Errorf("reading #%d's database id: %v", issue, err)
		}
		if _, err := forge("api", "-X", "POST", repoPath+strconv.Itoa(n)+"/dependencies/blocked_by", "-F", "issue_id="+strconv.FormatInt(blocker.ID, 10)); err != nil {
			return err
		}
	}
	return nil
}

// readBack compares the issue on the forge with what was written, returning why it differs or "" when it matches.
func readBack(forge Forge, n int, title, body, label string, blockers []int, issues map[int]int) (string, error) {
	out, err := forge("issue", "view", strconv.Itoa(n), "--json", "title,body,labels")
	if err != nil {
		return "", err
	}
	var got struct {
		Title  string
		Body   string
		Labels []struct{ Name string }
	}
	if err := json.Unmarshal(out, &got); err != nil {
		return "", fmt.Errorf("reading #%d back: %w", n, err)
	}
	switch {
	case got.Title != title:
		return fmt.Sprintf("title is %q", got.Title), nil
	case strings.TrimSpace(strings.ReplaceAll(got.Body, "\r\n", "\n")) != strings.TrimSpace(body):
		return "body differs from the draft", nil
	}
	hasLabel := false
	for _, l := range got.Labels {
		hasLabel = hasLabel || l.Name == label
	}
	if !hasLabel {
		return "label " + label + " is missing", nil
	}
	if len(blockers) > 0 {
		have, err := blockedBy(forge, n)
		if err != nil {
			return "", err
		}
		for _, b := range blockers {
			if !have[issues[b]] {
				return fmt.Sprintf("blocked-by edge to #%d is missing", issues[b]), nil
			}
		}
	}
	return "", nil
}
