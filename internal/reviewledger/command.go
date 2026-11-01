package reviewledger

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const usage = `Usage: harness review <command> [arguments]

Record and read this repository's local review ledger, kept at
$XDG_DATA_HOME/harness/reviews/<host>_<owner>_<repo>/ledger.jsonl (default
~/.local/share). It stays outside the repository and outside ~/.harness.

Commands:
  record  Append a new finding, or a new disposition for an existing one
  list    Print the latest line of each finding on a branch as JSON
  path    Print the ledger file's path
`

const recordUsage = `Usage: harness review record [flags]

Without --id, append a new line and print its id. A finding needs --depth,
--disposition, --severity, --path, and --summary; a whole-review line
(--disposition clean or no-review) takes no --path.

With --id, copy that finding's latest line, override it with the flags given
(usually --disposition and --evidence), and append the result.

  --id           Existing finding to update
  --branch       Branch (default: the current branch)
  --base         Base revision
  --sha          Reviewed commit (default: HEAD)
  --digest       Snapshot patch hash when the reviewed work is uncommitted
  --round        1 or 2 (default 1)
  --depth        none, quick, standard, or deep
  --axis         Hunter axis, such as bugs or spec
  --reviewer     Client and model, such as claude/opus
  --path         File the finding cites
  --lines        A line or range, such as 40 or 40-52
  --severity     bug, risk, q, pre-existing, or nit
  --class        Short kebab-case defect class
  --summary      One-line description
  --disposition  confirmed, unverified, fixed, refuted, intentional,
                 wont-matter, pre-existing, nit, clean, or no-review
  --evidence     Fixing commit and test, verifier output, or the human's words
`

const listUsage = `Usage: harness review list [--branch <name>] [--head <rev>]

Print, as a JSON array, the latest line of each finding on the branch (default:
the current branch). "open" is true for a latest disposition of confirmed or
unverified. "rewritten" marks a finding whose reviewed commit is no longer an
ancestor of --head (default: HEAD), as after a rebase; it still counts.
`

const pathUsage = `Usage: harness review path

Print this repository's ledger file path.
`

// Command runs `harness review` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "record":
		return record(args[1:], stdout, stderr)
	case "list":
		return list(args[1:], stdout, stderr)
	case "path":
		return printPath(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness review: unknown command %q; run harness review --help\n", args[0])
	return 2
}

// parse handles help and usage errors before any git call, returning -1 to continue.
func parse(flags *flag.FlagSet, args []string, text string, stdout, stderr io.Writer) int {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, text)
			return 0
		}
		fmt.Fprintf(stderr, "harness review %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "harness review %s: unexpected argument %q; run harness review %s --help\n", flags.Name(), flags.Arg(0), flags.Name())
		return 2
	}
	return -1
}

// ledger resolves the repository root and its ledger file.
func ledger(name string, stderr io.Writer) (root, file string, code int) {
	cwd, err := os.Getwd()
	if err == nil {
		root, err = git(cwd, "rev-parse", "--show-toplevel")
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: not inside a git repository\n", name)
		return "", "", 2
	}
	dir, err := Dir(root)
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return "", "", 1
	}
	return root, filepath.Join(dir, "ledger.jsonl"), -1
}

func record(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("record", flag.ContinueOnError)
	var e Entry
	var id, digest, path string
	flags.StringVar(&id, "id", "", "")
	flags.StringVar(&e.Branch, "branch", "", "")
	flags.StringVar(&e.Base, "base", "", "")
	flags.StringVar(&e.SHA, "sha", "", "")
	flags.StringVar(&digest, "digest", "", "")
	flags.IntVar(&e.Round, "round", 1, "")
	flags.StringVar(&e.Depth, "depth", "", "")
	flags.StringVar(&e.Axis, "axis", "", "")
	flags.StringVar(&e.Reviewer, "reviewer", "", "")
	flags.StringVar(&path, "path", "", "")
	flags.StringVar(&e.Lines, "lines", "", "")
	flags.StringVar(&e.Severity, "severity", "", "")
	flags.StringVar(&e.Class, "class", "", "")
	flags.StringVar(&e.Summary, "summary", "", "")
	flags.StringVar(&e.Disposition, "disposition", "", "")
	flags.StringVar(&e.Evidence, "evidence", "", "")
	if code := parse(flags, args, recordUsage, stdout, stderr); code >= 0 {
		return code
	}
	root, file, code := ledger("record", stderr)
	if code >= 0 {
		return code
	}
	if id != "" {
		entries, err := Read(file)
		if err != nil {
			fmt.Fprintf(stderr, "harness review record: %v\n", err)
			return 1
		}
		prior, ok := Find(entries, id)
		if !ok {
			fmt.Fprintf(stderr, "harness review record: no finding %q in %s\n", id, file)
			return 2
		}
		given := map[string]bool{}
		flags.Visit(func(f *flag.Flag) { given[f.Name] = true })
		merged := prior
		override := map[string]func(){
			"branch": func() { merged.Branch = e.Branch }, "base": func() { merged.Base = e.Base },
			"sha": func() { merged.SHA = e.SHA }, "digest": func() { merged.Digest = optional(digest) },
			"round": func() { merged.Round = e.Round }, "depth": func() { merged.Depth = e.Depth },
			"axis": func() { merged.Axis = e.Axis }, "reviewer": func() { merged.Reviewer = e.Reviewer },
			"path": func() { merged.Path = optional(path) }, "lines": func() { merged.Lines = e.Lines },
			"severity": func() { merged.Severity = e.Severity }, "class": func() { merged.Class = e.Class },
			"summary": func() { merged.Summary = e.Summary }, "disposition": func() { merged.Disposition = e.Disposition },
			"evidence": func() { merged.Evidence = e.Evidence },
		}
		for name, apply := range override {
			if given[name] {
				apply()
			}
		}
		e = merged
	} else {
		e.Digest, e.Path = optional(digest), optional(path)
		if e.Branch == "" {
			e.Branch, _ = git(root, "branch", "--show-current")
		}
		if e.SHA == "" {
			e.SHA, _ = git(root, "rev-parse", "HEAD")
		}
	}
	if err := Validate(e); err != nil {
		fmt.Fprintf(stderr, "harness review record: %v\n", err)
		return 2
	}
	if id == "" {
		short := e.SHA
		if len(short) > 7 {
			short = short[:7]
		}
		parts := []string{short}
		if e.Axis != "" {
			parts = append(parts, e.Axis)
		}
		id = strings.Join(append(parts, strconv.FormatInt(time.Now().UnixNano(), 36)), "-")
	}
	e.ID, e.TS = id, time.Now().UTC().Format(time.RFC3339)
	if err := Append(file, e); err != nil {
		fmt.Fprintf(stderr, "harness review record: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, id)
	return 0
}

func list(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("list", flag.ContinueOnError)
	var branch, head string
	flags.StringVar(&branch, "branch", "", "")
	flags.StringVar(&head, "head", "HEAD", "")
	if code := parse(flags, args, listUsage, stdout, stderr); code >= 0 {
		return code
	}
	root, file, code := ledger("list", stderr)
	if code >= 0 {
		return code
	}
	if branch == "" {
		branch, _ = git(root, "branch", "--show-current")
	}
	entries, err := Read(file)
	if err != nil {
		fmt.Fprintf(stderr, "harness review list: %v\n", err)
		return 1
	}
	latest := Latest(entries, branch, func(sha string) bool {
		_, err := git(root, "merge-base", "--is-ancestor", sha, head)
		return err == nil
	})
	if latest == nil {
		latest = []Entry{}
	}
	out, _ := json.MarshalIndent(latest, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func printPath(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("path", flag.ContinueOnError)
	if code := parse(flags, args, pathUsage, stdout, stderr); code >= 0 {
		return code
	}
	_, file, code := ledger("path", stderr)
	if code >= 0 {
		return code
	}
	fmt.Fprintln(stdout, file)
	return 0
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
