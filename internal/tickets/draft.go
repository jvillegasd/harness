// Package tickets reads and validates the ticket drafts `tickets` writes, and computes their ready frontier and publication.
package tickets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DraftDir is where a repository keeps its ticket drafts, one directory per feature.
var DraftDir = filepath.Join("docs", "agents", "tickets")

// Sections are the parts every draft carries, in order.
var Sections = []string{"What to build", "Acceptance criteria", "Seam", "Blocked by", "Out of scope"}

// Section is one `## ` part of a draft: the line its heading sits on and its trimmed text.
type Section struct {
	Line int
	Body string
}

// Ticket is one draft file.
type Ticket struct {
	Number   int
	Path     string // as reported: the feature directory joined with the file name
	Title    string // the title line without its leading number
	Sections map[string]Section
	// BlockedBy holds the numbers the Blocked by section names, in order.
	BlockedBy   []int
	blockedLine int
	// Done is set by a `Status: done` line.
	Done bool
	// Issue is the URL the draft records once publish created its issue.
	Issue string
}

// Problem is one rule a draft breaks, printed as path:line: message.
type Problem struct {
	Path    string
	Line    int
	Message string
}

func (p Problem) String() string { return fmt.Sprintf("%s:%d: %s", p.Path, p.Line, p.Message) }

var (
	nameRe   = regexp.MustCompile(`^([0-9]{2,})-[a-z0-9][a-z0-9-]*\.md$`)
	titleRe  = regexp.MustCompile(`^#\s+(?:[0-9]+\s+)?(\S.*)$`)
	statusRe = regexp.MustCompile(`(?i)^status:\s*done\s*$`)
	issueRe  = regexp.MustCompile(`^Issue:\s*(\S+)\s*$`)
	refRe    = regexp.MustCompile(`^#?([0-9]+)$`)
	splitRe  = regexp.MustCompile(`[\s,]+`)
)

// Features lists the feature directories under root's draft directory, sorted; none when it does not exist.
func Features(root string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, DraftDir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Check reads the drafts in root/rel and returns them sorted by number, with every rule they break. A missing directory holds no drafts and no problems.
func Check(root, rel string) ([]Ticket, []Problem, error) {
	entries, err := os.ReadDir(filepath.Join(root, rel))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var ts []Ticket
	var problems []Problem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(rel, e.Name())
		m := nameRe.FindStringSubmatch(e.Name())
		if m == nil {
			problems = append(problems, Problem{path, 1, "not a draft; name it <NN>-<slug>.md"})
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, nil, err
		}
		n, _ := strconv.Atoi(m[1])
		t, ps := parseDraft(path, n, string(raw))
		ts = append(ts, t)
		problems = append(problems, ps...)
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].Number < ts[j].Number })
	problems = append(problems, graph(ts)...)
	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].Path != problems[j].Path {
			return problems[i].Path < problems[j].Path
		}
		return problems[i].Line < problems[j].Line
	})
	return ts, problems, nil
}

// parseDraft reads one draft's title, sections, blockers, status, and recorded issue.
func parseDraft(path string, number int, text string) (Ticket, []Problem) {
	t := Ticket{Number: number, Path: path, Sections: map[string]Section{}}
	var problems []Problem
	lines := strings.Split(text, "\n")
	current, start := "", 0
	var body []string
	flush := func() {
		if current != "" {
			if _, seen := t.Sections[current]; !seen {
				t.Sections[current] = Section{start, strings.TrimSpace(strings.Join(body, "\n"))}
			}
		}
	}
	first := true
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if first && trimmed != "" {
			first = false
			if m := titleRe.FindStringSubmatch(trimmed); m != nil && !strings.HasPrefix(trimmed, "##") {
				t.Title = strings.TrimSpace(m[1])
				continue
			}
			problems = append(problems, Problem{path, 1, "missing title; start with # <NN> <title>"})
		}
		if name, ok := strings.CutPrefix(trimmed, "## "); ok {
			flush()
			current, start, body = strings.TrimSpace(name), i+1, nil
			continue
		}
		if statusRe.MatchString(trimmed) {
			t.Done = true
		}
		if current == "" {
			if m := issueRe.FindStringSubmatch(trimmed); m != nil {
				t.Issue = m[1]
			}
			continue
		}
		body = append(body, line)
	}
	flush()
	if first {
		problems = append(problems, Problem{path, 1, "missing title; start with # <NN> <title>"})
	}
	for _, name := range Sections {
		s, ok := t.Sections[name]
		switch {
		case !ok:
			problems = append(problems, Problem{path, 1, "missing section " + name})
		case s.Body == "":
			problems = append(problems, Problem{path, s.Line, "empty section " + name})
		}
	}
	if s, ok := t.Sections["Blocked by"]; ok && s.Body != "" {
		t.blockedLine = s.Line + firstText(lines[s.Line:])
		if !strings.EqualFold(s.Body, "none") {
			for _, field := range splitRe.Split(s.Body, -1) {
				m := refRe.FindStringSubmatch(field)
				if m == nil {
					problems = append(problems, Problem{path, t.blockedLine, "Blocked by must be none or ticket numbers, not " + strconv.Quote(s.Body)})
					t.BlockedBy = nil
					break
				}
				n, _ := strconv.Atoi(m[1])
				t.BlockedBy = append(t.BlockedBy, n)
			}
		}
	}
	return t, problems
}

// firstText counts the lines before the first non-blank one, plus one for the heading.
func firstText(lines []string) int {
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			return i + 1
		}
	}
	return 0
}

// graph reports shared numbers, blockers that are not tickets, and each blocking cycle once.
func graph(ts []Ticket) []Problem {
	var problems []Problem
	byNumber := map[int][]Ticket{}
	for _, t := range ts {
		byNumber[t.Number] = append(byNumber[t.Number], t)
	}
	for _, t := range ts {
		if len(byNumber[t.Number]) > 1 {
			problems = append(problems, Problem{t.Path, 1, fmt.Sprintf("ticket number %02d is shared with another draft", t.Number)})
		}
		for _, b := range t.BlockedBy {
			if len(byNumber[b]) == 0 {
				problems = append(problems, Problem{t.Path, t.blockedLine, fmt.Sprintf("blocked by %02d, which is not a ticket of this feature", b)})
			}
		}
	}
	for _, cycle := range cycles(ts) {
		names := make([]string, len(cycle))
		for i, n := range cycle {
			names[i] = fmt.Sprintf("%02d", n)
		}
		t := byNumber[cycle[0]][0]
		problems = append(problems, Problem{t.Path, t.blockedLine, "blocking cycle among " + strings.Join(names, ", ")})
	}
	return problems
}

// cycles returns each strongly connected group of tickets that blocks itself, its numbers sorted, by Tarjan's algorithm.
func cycles(ts []Ticket) [][]int {
	edges := map[int][]int{}
	for _, t := range ts {
		if _, seen := edges[t.Number]; !seen {
			edges[t.Number] = t.BlockedBy
		}
	}
	index, low, on := map[int]int{}, map[int]int{}, map[int]bool{}
	var stack []int
	var found [][]int
	var visit func(int)
	visit = func(v int) {
		index[v], low[v] = len(index), len(index)
		stack = append(stack, v)
		on[v] = true
		for _, w := range edges[v] {
			if _, ok := edges[w]; !ok {
				continue
			}
			if _, seen := index[w]; !seen {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if on[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] != index[v] {
			return
		}
		var group []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			on[w] = false
			group = append(group, w)
			if w == v {
				break
			}
		}
		if len(group) > 1 || blocksItself(v, edges[v]) {
			sort.Ints(group)
			found = append(found, group)
		}
	}
	var order []int
	for n := range edges {
		order = append(order, n)
	}
	sort.Ints(order)
	for _, n := range order {
		if _, seen := index[n]; !seen {
			visit(n)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i][0] < found[j][0] })
	return found
}

func blocksItself(n int, blockers []int) bool {
	for _, b := range blockers {
		if b == n {
			return true
		}
	}
	return false
}
