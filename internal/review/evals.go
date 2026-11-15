package review

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Case is one review eval case in evals.jsonl: a commit range and the findings a review of it should raise, or none for a clean case.
type Case struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"` // bug or clean
	Base        string     `json:"base"`
	Head        string     `json:"head"`
	Expected    []Expected `json:"expected,omitempty"`
	Paths       []string   `json:"paths,omitempty"`
	Source      string     `json:"source,omitempty"`
	ConfirmedBy string     `json:"confirmed_by"`
}

// Expected is where a bug case's defect sits at the case's head.
type Expected struct {
	Path  string `json:"path"`
	Lines string `json:"lines"`
	Class string `json:"class,omitempty"`
}

// RunFinding is one finding a run raised on a case, as a run file line.
type RunFinding struct {
	Case        string `json:"case"`
	Path        string `json:"path"`
	Lines       string `json:"lines,omitempty"`
	Severity    string `json:"severity"`
	Disposition string `json:"disposition"` // confirmed or dropped
}

// Summary is the line a scored run appends to its run file.
type Summary struct {
	Summary        bool              `json:"summary"`
	Date           string            `json:"date"`
	Revision       string            `json:"revision,omitempty"`
	Depth          string            `json:"depth,omitempty"`
	Models         map[string]string `json:"models,omitempty"`
	Cases          []string          `json:"cases"`
	BugCases       int               `json:"bug_cases"`
	Caught         int               `json:"caught"`
	Recall         float64           `json:"recall"`
	FalsePositives int               `json:"false_positives"`
	Dropped        int               `json:"dropped"`
	DroppedCatches int               `json:"dropped_catches"`
}

// Comparison is a scored run beside the last run on the same cases.
type Comparison struct {
	Summary  Summary  `json:"summary"`
	Previous *Summary `json:"previous"`
	// Regression is recall dropping or false positives rising against Previous: the review change is not kept.
	Regression bool `json:"regression"`
}

// EvalsDir is where the review evals live: inside the Git directory, beside and outside the branch directories land deletes.
func EvalsDir(root string) (string, error) {
	common, err := git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(common, "harness-review", "evals"), nil
}

// readLines hands each non-blank line of file to each, naming the line in any error.
func readLines(file string, each func(line []byte) error) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for n := 1; s.Scan(); n++ {
		if strings.TrimSpace(s.Text()) == "" {
			continue
		}
		if err := each(s.Bytes()); err != nil {
			return fmt.Errorf("%s:%d: %w", file, n, err)
		}
	}
	return s.Err()
}

// ReadCases reads the confirmed cases of evals.jsonl; a case no human confirmed does not count.
func ReadCases(file string) ([]Case, error) {
	var cases []Case
	err := readLines(file, func(line []byte) error {
		var c Case
		if err := json.Unmarshal(line, &c); err != nil {
			return err
		}
		if c.Kind != "bug" && c.Kind != "clean" {
			return fmt.Errorf("%w: case %q has kind %q, not bug or clean", ErrInvalid, c.ID, c.Kind)
		}
		if c.ConfirmedBy != "" {
			cases = append(cases, c)
		}
		return nil
	})
	return cases, err
}

// readRun splits a run file into its findings and its summary lines.
func readRun(file string) ([]RunFinding, []Summary, error) {
	var findings []RunFinding
	var summaries []Summary
	err := readLines(file, func(line []byte) error {
		var probe struct{ Summary bool }
		if err := json.Unmarshal(line, &probe); err != nil {
			return err
		}
		if probe.Summary {
			var s Summary
			if err := json.Unmarshal(line, &s); err != nil {
				return err
			}
			summaries = append(summaries, s)
			return nil
		}
		var f RunFinding
		if err := json.Unmarshal(line, &f); err != nil {
			return err
		}
		findings = append(findings, f)
		return nil
	})
	return findings, summaries, err
}

// span parses a line or a range such as 40-52; ok is false for anything else.
func span(lines string) (lo, hi int, ok bool) {
	a, b, ranged := strings.Cut(lines, "-")
	lo, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, false
	}
	hi = lo
	if ranged {
		if hi, err = strconv.Atoi(b); err != nil || hi < lo {
			return 0, 0, false
		}
	}
	return lo, hi, true
}

// within reports whether a finding falls inside an expected path and line range: the same path and overlapping lines.
func within(f RunFinding, e Expected) bool {
	flo, fhi, ok1 := span(f.Lines)
	elo, ehi, ok2 := span(e.Lines)
	return f.Path == e.Path && ok1 && ok2 && flo <= ehi && elo <= fhi
}

// Score scores a run's findings against the cases. A bug case is caught when a confirmed bug or risk falls within one of its expected ranges; each confirmed bug or risk on a clean case is a false positive; a dropped finding within a bug case's expected range is a dropped catch. Findings on a case that is not among the cases are an error.
func Score(cases []Case, findings []RunFinding) (Summary, error) {
	s := Summary{Summary: true, Cases: []string{}}
	byID := map[string]Case{}
	for _, c := range cases {
		byID[c.ID] = c
		s.Cases = append(s.Cases, c.ID)
		if c.Kind == "bug" {
			s.BugCases++
		}
	}
	sort.Strings(s.Cases)
	caught := map[string]bool{}
	for _, f := range findings {
		c, ok := byID[f.Case]
		if !ok {
			return s, fmt.Errorf("%w: a finding names case %q, which is not a confirmed case", ErrInvalid, f.Case)
		}
		hit := slices.ContainsFunc(c.Expected, func(e Expected) bool { return within(f, e) })
		counted := f.Severity == "bug" || f.Severity == "risk"
		switch {
		case f.Disposition == "dropped":
			s.Dropped++
			if c.Kind == "bug" && hit {
				s.DroppedCatches++
			}
		case f.Disposition != "confirmed" || !counted:
		case c.Kind == "clean":
			s.FalsePositives++
		case hit:
			caught[c.ID] = true
		}
	}
	s.Caught = len(caught)
	if s.BugCases > 0 {
		s.Recall = float64(s.Caught) / float64(s.BugCases)
	}
	return s, nil
}

// Previous is the latest summary on the same cases in the run files under dir, in file-name order, or nil when there is none.
func Previous(dir string, cases []string) (*Summary, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var last *Summary
	for _, file := range files {
		_, summaries, err := readRun(file)
		if err != nil {
			return nil, err
		}
		for i := range summaries {
			if slices.Equal(summaries[i].Cases, cases) {
				last = &summaries[i]
			}
		}
	}
	return last, nil
}

// ScoreRun scores the run file against evals.jsonl beside its runs directory, compares it with the last run on the same cases, and appends the summary to the run file. A run file already holding a summary is refused, so a run is scored once.
func ScoreRun(casesFile, runFile string, meta Summary) (Comparison, error) {
	cases, err := ReadCases(casesFile)
	if err != nil {
		return Comparison{}, err
	}
	findings, summaries, err := readRun(runFile)
	if err != nil {
		return Comparison{}, err
	}
	if len(summaries) > 0 {
		return Comparison{}, fmt.Errorf("%w: %s is already scored", ErrInvalid, runFile)
	}
	s, err := Score(cases, findings)
	if err != nil {
		return Comparison{}, err
	}
	s.Date, s.Revision, s.Depth, s.Models = meta.Date, meta.Revision, meta.Depth, meta.Models
	prev, err := Previous(filepath.Dir(runFile), s.Cases)
	if err != nil {
		return Comparison{}, err
	}
	c := Comparison{Summary: s, Previous: prev}
	c.Regression = prev != nil && (s.Recall < prev.Recall || s.FalsePositives > prev.FalsePositives)
	line, err := json.Marshal(s)
	if err != nil {
		return c, err
	}
	f, err := os.OpenFile(runFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return c, err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return c, err
	}
	return c, f.Close()
}

var (
	fixSubjectRe = regexp.MustCompile(`^(fix(\([^)]*\))?!?:|[Rr]evert\b)`)
	hunkRe       = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? `)
)

// Seed lists candidate bug cases from the fix and revert commits in rev (newest first, at most max): each fix's changed lines are blamed at its parent, and each commit that introduced them becomes one case whose expected ranges are those lines as they stood in that commit. A pure insertion is placed by the lines around it. The candidates are unconfirmed: the human confirms each before it counts.
func Seed(root, rev string, max int) ([]Case, error) {
	out, err := git(root, "log", "--no-merges", "--format=%H%x09%s", rev)
	if err != nil {
		return nil, err
	}
	cases := []Case{}
	fixes := 0
	for _, line := range strings.Split(out, "\n") {
		sha, subject, ok := strings.Cut(line, "\t")
		if !ok || !fixSubjectRe.MatchString(subject) {
			continue
		}
		if fixes++; fixes > max {
			break
		}
		found, err := seedFix(root, sha)
		if err != nil {
			return nil, err
		}
		cases = append(cases, found...)
	}
	return cases, nil
}

// seedFix blames one fix commit's changed lines at its parent and groups them by the commit that introduced them.
func seedFix(root, fix string) ([]Case, error) {
	if _, err := git(root, "rev-parse", "--verify", "--quiet", fix+"^"); err != nil {
		return nil, nil
	}
	diff, err := git(root, "diff", "-U0", "--no-renames", "--diff-filter=MD", fix+"^", fix)
	if err != nil {
		return nil, err
	}
	type hit struct {
		path   string
		lo, hi int
	}
	byIntro := map[string]map[string][2]int{}
	var order []string
	add := func(intro, path string, n int) {
		if byIntro[intro] == nil {
			byIntro[intro] = map[string][2]int{}
			order = append(order, intro)
		}
		r, seen := byIntro[intro][path]
		if !seen {
			r = [2]int{n, n}
		}
		byIntro[intro][path] = [2]int{min(r[0], n), max(r[1], n)}
	}
	var path string
	var blames []hit
	for _, l := range strings.Split(diff, "\n") {
		if p, ok := strings.CutPrefix(l, "--- a/"); ok {
			path = p
			continue
		}
		m := hunkRe.FindStringSubmatch(l)
		if m == nil || path == "" {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		count := 1
		if m[2] != "" {
			count, _ = strconv.Atoi(m[2])
		}
		lo, hi := start, start+count-1
		if count == 0 {
			// A pure insertion after line start: the lines around it are where the defect was.
			lo, hi = max(start, 1), start+1
		}
		blames = append(blames, hit{path, lo, hi})
	}
	lengths := map[string]int{}
	for _, b := range blames {
		if _, ok := lengths[b.path]; !ok {
			text, err := rawGit(root, "show", fix+"^:"+b.path)
			if err != nil {
				return nil, err
			}
			lengths[b.path] = strings.Count(text, "\n")
			if text != "" && !strings.HasSuffix(text, "\n") {
				lengths[b.path]++
			}
		}
		hi := min(b.hi, lengths[b.path])
		if b.lo > hi {
			continue
		}
		raw, err := rawGit(root, "blame", "--line-porcelain", "-L", fmt.Sprintf("%d,%d", b.lo, hi), fix+"^", "--", b.path)
		if err != nil {
			return nil, err
		}
		intro, file := "", b.path
		var origLine int
		for _, pl := range strings.Split(raw, "\n") {
			fields := strings.Fields(pl)
			switch {
			case len(fields) >= 3 && len(fields[0]) == 40 && !strings.HasPrefix(pl, "\t"):
				intro = fields[0]
				origLine, _ = strconv.Atoi(fields[1])
			case strings.HasPrefix(pl, "filename "):
				file = strings.TrimPrefix(pl, "filename ")
			case strings.HasPrefix(pl, "\t") && intro != "":
				add(intro, file, origLine)
			}
		}
	}
	var cases []Case
	for _, intro := range order {
		// A root commit has no parent to diff against, so it makes no case.
		base, err := git(root, "rev-parse", "--verify", "--quiet", intro+"^")
		if err != nil {
			continue
		}
		c := Case{ID: "seed-" + intro[:12] + "-" + fix[:12], Kind: "bug", Base: base, Head: intro, Source: fix}
		var paths []string
		for p := range byIntro[intro] {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			r := byIntro[intro][p]
			lines := strconv.Itoa(r[0])
			if r[1] != r[0] {
				lines += "-" + strconv.Itoa(r[1])
			}
			c.Expected = append(c.Expected, Expected{Path: p, Lines: lines})
		}
		cases = append(cases, c)
	}
	return cases, nil
}

// rawGit runs git in dir and returns its output untrimmed: a blank last line of a file, or of blame's content, is whitespace that trimming would erase.
func rawGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
