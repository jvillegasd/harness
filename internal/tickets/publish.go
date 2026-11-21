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

	"github.com/jvillegasd/harness/internal/ui"
)

type Forge func(args ...string) ([]byte, error)

type Config struct {
	Tracker string `json:"tracker"`
}

var ConfigPath = filepath.Join("docs", "agents", "harness.json")

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

func Plan(ts []Ticket, out io.Writer) {
	s := ui.For(out)
	fmt.Fprintln(out, s.Title("harness tickets publish", "dry run"))
	var rows [][]string
	for _, t := range Order(ts) {
		if t.Issue != "" {
			rows = append(rows, []string{s.Good("exists"), fmt.Sprintf("%02d", t.Number), t.Issue})
			continue
		}
		rows = append(rows, []string{s.Accent("create"), fmt.Sprintf("%02d", t.Number), t.Title})
	}
	for _, t := range Order(ts) {
		for _, b := range t.BlockedBy {
			rows = append(rows, []string{"edge", fmt.Sprintf("%02d", t.Number), fmt.Sprintf("blocked by %02d", b)})
		}
	}
	for _, line := range s.Table([]string{"action", "ticket", "detail"}, rows) {
		fmt.Fprintln(out, line)
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

type Published struct {
	Created  []string
	Issues   []string
	Kept     []string
	Removed  []string
	Remains  []string
	Mismatch []string
}

func Publish(root string, ts []Ticket, forge Forge) (Published, error) {
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
			out, err := forge("issue", "create", "--title", t.Title, "--body", Body(t, issues))
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
		why, err := readBack(forge, n, t.Title, Body(t, issues), t.BlockedBy, issues)
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

	needed := map[int]bool{}
	byNumber := make(map[int]Ticket, len(order))
	for _, t := range order {
		byNumber[t.Number] = t
	}
	var needBlockers func(int)
	seen := map[int]bool{}
	needBlockers = func(number int) {
		if seen[number] {
			return
		}
		seen[number] = true
		for _, blocker := range byNumber[number].BlockedBy {
			needed[blocker] = true
			needBlockers(blocker)
		}
	}
	for _, t := range order {
		if !matched[t.Number] {
			needBlockers(t.Number)
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
		os.Remove(filepath.Dir(filepath.Join(root, order[0].Path)))
	}
	return p, nil
}

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

func readBack(forge Forge, n int, title, body string, blockers []int, issues map[int]int) (string, error) {
	out, err := forge("issue", "view", strconv.Itoa(n), "--json", "title,body")
	if err != nil {
		return "", err
	}
	var got struct {
		Title string
		Body  string
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
