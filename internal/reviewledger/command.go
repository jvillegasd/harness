package reviewledger

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

const usage = `Usage: harness review <command> [arguments]

Record and read a branch's local review ledger, kept at
<git-common-dir>/harness-review/branches/<branch>/ledger.jsonl. Git never
tracks it, and /land deletes the branch directory with the branch.

Commands:
  record  Append a new finding, or a new disposition for an existing one
  list    Print the latest line of each finding on a branch as JSON
  path    Print the ledger file's path
  level   Print or set the review level: off, medium (default), or high
`

const recordUsage = `Usage: harness review record [flags]

Without --id, append a new line and print its id. A finding needs --depth,
--disposition, --severity, --path, and --summary; a whole-review line
(--disposition clean or no-review) takes no --path.

With --id, append a new state for that finding: only --disposition,
--evidence, and --commit are accepted, and the rest is copied from its latest
line. A fixed, dropped, refuted, intentional, or wont-matter disposition needs
--evidence; fixed also needs --commit. A nit or pre-existing disposition fits
only a finding of that severity.

  --id           Existing finding to update
  --branch       Branch (default: the current branch)
  --base         Base revision
  --sha          Reviewed commit (default: HEAD)
  --digest       Snapshot patch hash when the reviewed work is uncommitted
  --round        1 or 2 (default 1)
  --depth        none, medium, or high (deep, and quick and standard, the
                 earlier names for medium and high, are translated)
  --axis         Hunter axis, such as bugs or spec
  --reviewer     Client and model, such as claude/opus
  --path         File the finding cites
  --lines        A line or range, such as 40 or 40-52
  --severity     bug, risk, q, pre-existing, or nit
  --class        Short kebab-case defect class
  --summary      One-line description
  --disposition  confirmed, fixed, dropped, intentional, wont-matter,
                 pre-existing, nit, clean, or no-review; unverified and
                 refuted are earlier dispositions new reviews do not
                 write. A confirmed or fixed finding cannot be dropped.
  --evidence     The run's output, what a dropped finding's run showed, or
                 the human's words
  --commit       The fixing commit, for --disposition fixed
`

const listUsage = `Usage: harness review list [--branch <name>] [--head <rev>]

Print, as a JSON array, the latest line of each finding and whole-review line
on the branch (default: the current branch); [] means nothing is recorded for
it here. "open" is true for confirmed and unverified, and for fixed until the
fixing commit is in --head (default: HEAD). "rewritten" marks a reviewed commit
no longer in --head's history, as after a rebase; the finding still counts.
`

const pathUsage = `Usage: harness review path [--branch <name>]

Print the ledger file path for the branch (default: the current branch).
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
	case "level":
		return level(args[1:], stdout, stderr)
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

// ledger resolves the repository root, the branch (default: the current one), and the branch's ledger file.
func ledger(name, branch string, stderr io.Writer) (root, resolved, file string, code int) {
	cwd, err := os.Getwd()
	if err == nil {
		root, err = git(cwd, "rev-parse", "--show-toplevel")
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: not inside a git repository\n", name)
		return "", "", "", 2
	}
	if branch == "" {
		branch, _ = git(root, "branch", "--show-current")
	}
	file, err = File(root, branch)
	if errors.Is(err, ErrInvalid) {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return "", "", "", 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return "", "", "", 1
	}
	return root, branch, file, -1
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
	flags.StringVar(&e.Commit, "commit", "", "")
	if code := parse(flags, args, recordUsage, stdout, stderr); code >= 0 {
		return code
	}
	if id != "" {
		// An update changes a finding's state, never what the finding says, so its severity cannot be relabelled to close it.
		var other []string
		flags.Visit(func(f *flag.Flag) {
			if f.Name != "id" && f.Name != "branch" && f.Name != "disposition" && f.Name != "evidence" && f.Name != "commit" {
				other = append(other, "--"+f.Name)
			}
		})
		if len(other) > 0 {
			fmt.Fprintf(stderr, "harness review record: an update with --id takes only --branch, --disposition, --evidence, and --commit, not %s\n", strings.Join(other, ", "))
			return 2
		}
	}
	root, branch, file, code := ledger("record", e.Branch, stderr)
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
		// A proven finding stays proven: dropping is for findings no run proved, so it cannot close a confirmed or fixed one on an agent's word.
		if e.Disposition == "dropped" && (prior.Disposition == "confirmed" || prior.Disposition == "fixed") {
			fmt.Fprintf(stderr, "harness review record: finding %s is %s, and a proven finding cannot be dropped; record the human's decision as intentional or wont-matter instead\n", id, prior.Disposition)
			return 2
		}
		prior.Disposition, prior.Evidence, prior.Commit = e.Disposition, e.Evidence, e.Commit
		e = prior
	} else {
		e.Digest, e.Path = optional(digest), optional(path)
		e.Branch, e.Depth = branch, CanonicalDepth(e.Depth)
		if e.SHA == "" {
			e.SHA, _ = git(root, "rev-parse", "HEAD")
		}
	}
	if err := Validate(e); err != nil {
		fmt.Fprintf(stderr, "harness review record: %v\n", err)
		return 2
	}
	if e.Disposition == "fixed" {
		// Store the fixing commit as a full SHA after the reviewed one, so a name like HEAD or the reviewed commit itself cannot close a finding.
		commit, err := git(root, "rev-parse", "--verify", "--end-of-options", e.Commit+"^{commit}")
		if err != nil {
			fmt.Fprintf(stderr, "harness review record: --commit %q does not name a commit\n", e.Commit)
			return 2
		}
		if _, err := git(root, "merge-base", "--is-ancestor", e.SHA, commit); err != nil || strings.HasPrefix(commit, e.SHA) {
			fmt.Fprintf(stderr, "harness review record: --commit %s does not come after the reviewed commit %s\n", commit, e.SHA)
			return 2
		}
		e.Commit = commit
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
	root, branch, file, code := ledger("list", branch, stderr)
	if code >= 0 {
		return code
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
		latest = []Finding{}
	}
	out, _ := json.MarshalIndent(latest, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func printPath(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("path", flag.ContinueOnError)
	var branch string
	flags.StringVar(&branch, "branch", "", "")
	if code := parse(flags, args, pathUsage, stdout, stderr); code >= 0 {
		return code
	}
	_, _, file, code := ledger("path", branch, stderr)
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
