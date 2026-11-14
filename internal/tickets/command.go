package tickets

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const usage = `Usage: harness tickets <command> [arguments]

Work with the ticket drafts under docs/agents/tickets/<feature-slug>/.

Commands:
  check     Validate the drafts of one feature, or of every feature
  frontier  Print which tickets of a feature are ready, done, and blocked
  publish   Publish a feature's drafts as GitHub issues
`

const checkUsage = `Usage: harness tickets check [<feature-slug>]

Validate the ticket drafts in docs/agents/tickets/<feature-slug>/, or in every
feature directory when none is named, and print each problem as
path:line: message. Exit 0 when there is none and 1 otherwise.

A draft is a file named <NN>-<slug>.md holding a title line (# <NN> <title>)
and five non-empty sections: ## What to build, ## Acceptance criteria,
## Seam, ## Blocked by, and ## Out of scope. Blocked by holds none or the
numbers of tickets in the same feature, numbers are unique within a feature,
and the blocking edges form no cycle.
`

const frontierUsage = `Usage: harness tickets frontier <feature-slug> [--done <numbers>]
       harness tickets frontier --dir <feature-dir> [--done <numbers>]

Print, as JSON, which tickets of a feature are ready to start, which are done,
and which are blocked with the blockers each still waits on. A ticket is done
when --done names it or its draft carries a Status: done line, and ready when
it is not done and every ticket it is blocked by is done. A feature whose
drafts fail harness tickets check is refused with the check's problems.

  --done  Comma-separated ticket numbers already done, such as the slices
          build has integrated; a number that is not a ticket exits 2
  --dir   The feature directory to read instead of
          docs/agents/tickets/<feature-slug>, such as a local-markdown
          tracker's <spec_home>/<feature-slug>/issues
`

const publishUsage = `Usage: harness tickets publish <feature-slug> [--dry-run]

Publish the drafts in docs/agents/tickets/<feature-slug>/ as GitHub issues. It
writes to the forge, so run it only when the human asked to publish; it grants
no authority. It refuses, before any write, a repository whose
docs/agents/harness.json does not select the github tracker and drafts that
fail harness tickets check.

Blockers are created before the tickets they block. Each issue takes the
draft's title and its five sections, with Blocked by naming the published
issues, and the mapped ready_for_agent label when it has no blocker or the
mapped blocked label otherwise. Each blocking edge is a native dependency sent
with the blocker's database id. The issue URL is recorded in the draft as an
Issue: line the moment it is created, so a rerun after a failure skips it.
Every write is read back; a draft whose issue matches is removed, and one that
does not is kept and reported. On a forge failure it stops, reports what was
created and what remains, and leaves the remaining drafts as they are.

  --dry-run  Print each issue to create, each edge, and each label, and write
             nothing
`

// Command runs `harness tickets` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "check":
		return check(args[1:], stdout, stderr)
	case "frontier":
		return frontier(args[1:], stdout, stderr)
	case "publish":
		return publish(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness tickets: unknown command %q; run harness tickets --help\n", args[0])
	return 2
}

// parse handles help and usage errors before any read, returning -1 to continue.
func parse(flags *flag.FlagSet, args []string, text string, stdout, stderr io.Writer) int {
	flags.SetOutput(io.Discard)
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, text)
			return 0
		}
		fmt.Fprintf(stderr, "harness tickets %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	return -1
}

// root returns the top of the checkout holding the working directory.
func root(name string, stderr io.Writer) (string, bool) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets %s: not inside a git repository\n", name)
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// slug validates a feature slug so it names one directory under the draft directory.
func slug(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid feature slug %q", name)
	}
	return nil
}

func check(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	if code := parse(flags, args, checkUsage, stdout, stderr); code >= 0 {
		return code
	}
	if flags.NArg() > 1 {
		fmt.Fprintf(stderr, "harness tickets check: unexpected argument %q; run harness tickets check --help\n", flags.Arg(1))
		return 2
	}
	if flags.NArg() == 1 {
		if err := slug(flags.Arg(0)); err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 2
		}
	}
	top, ok := root("check", stderr)
	if !ok {
		return 2
	}
	features := []string{flags.Arg(0)}
	if flags.NArg() == 0 {
		var err error
		if features, err = Features(top); err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 1
		}
	}
	found := false
	for _, f := range features {
		_, problems, err := Check(top, filepath.Join(DraftDir, f))
		if err != nil {
			fmt.Fprintf(stderr, "harness tickets check: %v\n", err)
			return 1
		}
		for _, p := range problems {
			fmt.Fprintln(stdout, p)
			found = true
		}
	}
	if found {
		return 1
	}
	return 0
}

// featureDir resolves one feature's directory from its slug, or from dir, as the root and path Check takes.
func featureDir(top, name, dir string) (string, string, error) {
	if dir == "" {
		return top, filepath.Join(DraftDir, name), nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	if rel, err := filepath.Rel(top, abs); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return top, rel, nil
	}
	return "", abs, nil
}

// load reads a feature's drafts and refuses them, printing the check's problems, when any rule fails.
func load(name, root, rel string, stderr io.Writer) ([]Ticket, int) {
	ts, problems, err := Check(root, rel)
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets %s: %v\n", name, err)
		return nil, 1
	}
	if len(problems) > 0 {
		fmt.Fprintf(stderr, "harness tickets %s: the drafts fail harness tickets check:\n", name)
		for _, p := range problems {
			fmt.Fprintln(stderr, p)
		}
		return nil, 1
	}
	return ts, 0
}

// numbers parses a comma-separated list of ticket numbers, each optionally prefixed with #.
func numbers(text string) ([]int, error) {
	var ns []int
	for _, field := range splitRe.Split(strings.TrimSpace(text), -1) {
		if field == "" {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(field, "#"))
		if err != nil || n < 0 {
			return nil, fmt.Errorf("--done takes ticket numbers, not %q", field)
		}
		ns = append(ns, n)
	}
	return ns, nil
}

// target reads the one feature a command names, by slug or --dir, refusing both or neither.
func target(name string, flags *flag.FlagSet, dir string, stderr io.Writer) (string, bool) {
	switch {
	case flags.NArg() > 1:
		fmt.Fprintf(stderr, "harness tickets %s: unexpected argument %q; run harness tickets %s --help\n", name, flags.Arg(1), name)
		return "", false
	case (flags.NArg() == 1) == (dir != ""):
		fmt.Fprintf(stderr, "harness tickets %s: name a feature slug or --dir, not both; run harness tickets %s --help\n", name, name)
		return "", false
	case flags.NArg() == 1:
		if err := slug(flags.Arg(0)); err != nil {
			fmt.Fprintf(stderr, "harness tickets %s: %v\n", name, err)
			return "", false
		}
		return flags.Arg(0), true
	}
	return dir, true
}

func frontier(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("frontier", flag.ContinueOnError)
	doneText := flags.String("done", "", "")
	dir := flags.String("dir", "", "")
	if code := parse(flags, interspersed(args), frontierUsage, stdout, stderr); code >= 0 {
		return code
	}
	name, ok := target("frontier", flags, *dir, stderr)
	if !ok {
		return 2
	}
	done, err := numbers(*doneText)
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets frontier: %v\n", err)
		return 2
	}
	top, ok := root("frontier", stderr)
	if !ok {
		return 2
	}
	slugName := name
	if *dir != "" {
		slugName = ""
	}
	r, rel, err := featureDir(top, slugName, *dir)
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets frontier: %v\n", err)
		return 1
	}
	ts, code := load("frontier", r, rel, stderr)
	if code != 0 {
		return code
	}
	if len(ts) == 0 {
		fmt.Fprintf(stderr, "harness tickets frontier: no drafts in %s\n", rel)
		return 1
	}
	f, err := Compute(ts, done)
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets frontier: %v\n", err)
		return 2
	}
	f.Feature = name
	out, _ := json.MarshalIndent(f, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

// interspersed moves a leading positional argument after the flags, so `frontier <slug> --done 1` parses as `frontier --done 1 <slug>`.
func interspersed(args []string) []string {
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		return append(append([]string{}, args[1:]...), args[0])
	}
	return args
}

// gh is the Forge that runs the gh CLI in dir.
func gh(dir string) Forge {
	return func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		cmd.Dir = dir
		var out, errOut bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errOut
		if err := cmd.Run(); err != nil {
			return out.Bytes(), fmt.Errorf("gh %s: %w: %s", args[0]+" "+args[1], err, strings.TrimSpace(errOut.String()))
		}
		return out.Bytes(), nil
	}
}

func publish(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "")
	if code := parse(flags, interspersed(args), publishUsage, stdout, stderr); code >= 0 {
		return code
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "harness tickets publish: name one feature slug; run harness tickets publish --help")
		return 2
	}
	name := flags.Arg(0)
	if err := slug(name); err != nil {
		fmt.Fprintf(stderr, "harness tickets publish: %v\n", err)
		return 2
	}
	top, ok := root("publish", stderr)
	if !ok {
		return 2
	}
	c, err := ReadConfig(top)
	if err == nil && c.Tracker != "github" {
		err = fmt.Errorf("%s selects the %q tracker; publish writes only to github", ConfigPath, c.Tracker)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets publish: %v\n", err)
		return 1
	}
	rel := filepath.Join(DraftDir, name)
	ts, code := load("publish", top, rel, stderr)
	if code != 0 {
		return code
	}
	if len(ts) == 0 {
		fmt.Fprintf(stderr, "harness tickets publish: no drafts in %s\n", rel)
		return 1
	}
	if *dryRun {
		Plan(c, ts, stdout)
		return 0
	}
	p, err := Publish(c, top, ts, gh(top))
	for _, url := range p.Issues {
		fmt.Fprintln(stdout, url)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness tickets publish: stopped: %v\n", err)
		fmt.Fprintf(stderr, "created: %s\n", list(p.Created))
		fmt.Fprintf(stderr, "remains: %s\n", list(p.Remains))
		return 1
	}
	if len(p.Mismatch) > 0 {
		for _, m := range p.Mismatch {
			fmt.Fprintf(stderr, "harness tickets publish: kept %s\n", m)
		}
		return 1
	}
	return 0
}

func list(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
