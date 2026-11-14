// Package attempts keeps the attempt log of a measured loop for `harness attempts`: one tab-separated row per attempt, with the id, time, and delta computed rather than typed.
package attempts

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usage = `Usage: harness attempts <command> [arguments]

Keep the attempt log for measured loops.

Commands:
  add   Append a measured attempt to the log
  list  Print logged attempts
`

const addUsage = `Usage: harness attempts add --log <file> --hypothesis <text> --change <text> --before <number> --after <number> --gate <pass|fail> --outcome <kept|reverted> [--note <text>]

Append one row to the tab-separated attempt log, writing its header on first
use. The id, the UTC time, and the delta (after minus before, and as a percent
of before, empty when before is 0) are computed. A tab or newline in a text
value is stored as a space, and a value a spreadsheet would read as a formula
is stored with a leading '. The log must sit outside the current checkout. A
rejected value exits 2 and writes nothing.
`

const listUsage = `Usage: harness attempts list --log <file> [--json]

Print the attempt log as stored, or with --json as an array of rows whose id,
before, after, delta, and percent are numbers (percent null when empty).
`

var columns = []string{"id", "time", "hypothesis", "change", "before", "after", "delta", "percent", "gate", "outcome", "note"}

type row struct {
	ID         int      `json:"id"`
	Time       string   `json:"time"`
	Hypothesis string   `json:"hypothesis"`
	Change     string   `json:"change"`
	Before     float64  `json:"before"`
	After      float64  `json:"after"`
	Delta      float64  `json:"delta"`
	Percent    *float64 `json:"percent"`
	Gate       string   `json:"gate"`
	Outcome    string   `json:"outcome"`
	Note       string   `json:"note"`
}

// Command runs `harness attempts` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "--help", "-h", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "add":
		return add(args[1:], stdout, stderr)
	case "list":
		return list(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "harness attempts: unknown command %q; run harness attempts --help\n", args[0])
		return 2
	}
}

func flags(name string, args []string, help string, stdout, stderr io.Writer, register func(*flag.FlagSet)) (*flag.FlagSet, int) {
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	register(f)
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, help)
			return nil, 0
		}
		fmt.Fprintf(stderr, "harness attempts %s: %v\n%s", name, err, help)
		return nil, 2
	}
	return f, -1
}

func add(args []string, stdout, stderr io.Writer) int {
	var path, hypothesis, change, gate, outcome, note, beforeText, afterText string
	f, code := flags("add", args, addUsage, stdout, stderr, func(f *flag.FlagSet) {
		f.StringVar(&path, "log", "", "")
		f.StringVar(&hypothesis, "hypothesis", "", "")
		f.StringVar(&change, "change", "", "")
		f.StringVar(&beforeText, "before", "", "")
		f.StringVar(&afterText, "after", "", "")
		f.StringVar(&gate, "gate", "", "")
		f.StringVar(&outcome, "outcome", "", "")
		f.StringVar(&note, "note", "", "")
	})
	if code >= 0 {
		return code
	}
	if f.NArg() != 0 || path == "" || hypothesis == "" || change == "" || beforeText == "" || afterText == "" || gate == "" || outcome == "" {
		fmt.Fprintf(stderr, "harness attempts add: missing or unexpected argument\n%s", addUsage)
		return 2
	}
	if gate != "pass" && gate != "fail" {
		return invalid(stderr, "add", "--gate must be pass or fail")
	}
	if outcome != "kept" && outcome != "reverted" {
		return invalid(stderr, "add", "--outcome must be kept or reverted")
	}
	before, err := strconv.ParseFloat(beforeText, 64)
	if err != nil || math.IsNaN(before) || math.IsInf(before, 0) {
		return invalid(stderr, "add", "--before must be a finite number")
	}
	after, err := strconv.ParseFloat(afterText, 64)
	if err != nil || math.IsNaN(after) || math.IsInf(after, 0) {
		return invalid(stderr, "add", "--after must be a finite number")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return operational(stderr, "add", err)
	}
	if inside, err := inCheckout(abs); err != nil {
		return operational(stderr, "add", err)
	} else if inside {
		return invalid(stderr, "add", "log path must be outside the current checkout")
	}
	parent := filepath.Dir(abs)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return operational(stderr, "add", err)
	}
	abs = filepath.Join(resolvedParent, filepath.Base(abs))
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return operational(stderr, "add", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return operational(stderr, "add", err)
	}
	if info.Size() == 0 {
		if err := writeRecord(file, columns); err != nil {
			return operational(stderr, "add", err)
		}
	}
	id, err := nextID(abs)
	if err != nil {
		return operational(stderr, "add", err)
	}
	delta := after - before
	var percent *float64
	if before != 0 {
		v := math.Round((delta/before)*10000) / 100
		percent = &v
	}
	r := row{ID: id, Time: time.Now().UTC().Format(time.RFC3339), Hypothesis: clean(hypothesis), Change: clean(change), Before: before, After: after, Delta: delta, Percent: percent, Gate: gate, Outcome: outcome, Note: clean(note)}
	values := []string{strconv.Itoa(r.ID), r.Time, r.Hypothesis, r.Change, number(r.Before), number(r.After), number(r.Delta), "", r.Gate, r.Outcome, r.Note}
	if r.Percent != nil {
		values[7] = number(*r.Percent)
	}
	if err := writeRecord(file, values); err != nil {
		return operational(stderr, "add", err)
	}
	return 0
}

func list(args []string, stdout, stderr io.Writer) int {
	var path string
	var asJSON bool
	f, code := flags("list", args, listUsage, stdout, stderr, func(f *flag.FlagSet) { f.StringVar(&path, "log", "", ""); f.BoolVar(&asJSON, "json", false, "") })
	if code >= 0 {
		return code
	}
	if f.NArg() != 0 || path == "" {
		fmt.Fprintf(stderr, "harness attempts list: --log is required and positional arguments are not accepted\n%s", listUsage)
		return 2
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return operational(stderr, "list", err)
	}
	if !asJSON {
		if _, err := stdout.Write(data); err != nil {
			return operational(stderr, "list", err)
		}
		return 0
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	all, err := r.ReadAll()
	if err != nil {
		return operational(stderr, "list", err)
	}
	if len(all) == 0 {
		all = [][]string{columns}
	}
	if !same(all[0], columns) {
		return invalid(stderr, "list", "log header is invalid")
	}
	rows := make([]row, 0, len(all)-1)
	for _, v := range all[1:] {
		if len(v) != len(columns) {
			return invalid(stderr, "list", "log row has the wrong number of columns")
		}
		id, e1 := strconv.Atoi(v[0])
		before, e2 := strconv.ParseFloat(v[4], 64)
		after, e3 := strconv.ParseFloat(v[5], 64)
		delta, e4 := strconv.ParseFloat(v[6], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return invalid(stderr, "list", "log row contains invalid numbers")
		}
		var pct *float64
		if v[7] != "" {
			n, e := strconv.ParseFloat(v[7], 64)
			if e != nil {
				return invalid(stderr, "list", "log row contains invalid percent")
			}
			pct = &n
		}
		rows = append(rows, row{ID: id, Time: v[1], Hypothesis: v[2], Change: v[3], Before: before, After: after, Delta: delta, Percent: pct, Gate: v[8], Outcome: v[9], Note: v[10]})
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rows); err != nil {
		return operational(stderr, "list", err)
	}
	return 0
}

func inCheckout(path string) (bool, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return false, err
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return false, nil
	}
	root := strings.TrimSpace(string(out))
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return false, err
	}
	resolved := path
	parent := filepath.Dir(path)
	for {
		resolved, err = filepath.EvalSymlinks(parent)
		if err == nil {
			resolved = filepath.Join(resolved, filepath.Base(path))
			break
		}
		if !os.IsNotExist(err) {
			return false, err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return false, err
		}
		parent = next
	}
	rel, err := filepath.Rel(root, resolved)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)), err
}

func nextID(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = '\t'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func writeRecord(w io.Writer, values []string) error {
	c := csv.NewWriter(w)
	c.Comma = '\t'
	if err := c.Write(values); err != nil {
		return err
	}
	c.Flush()
	return c.Error()
}
func clean(s string) string {
	s = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(s)
	if strings.HasPrefix(s, "=") || strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "@") {
		s = "'" + s
	}
	return s
}
func number(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func invalid(stderr io.Writer, name, message string) int {
	fmt.Fprintf(stderr, "harness attempts %s: %s\n", name, message)
	return 2
}
func operational(stderr io.Writer, name string, err error) int {
	fmt.Fprintf(stderr, "harness attempts %s: %v\n", name, err)
	return 1
}
