package worktree

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

const usage = `Usage: harness worktree <command> [arguments]

Audit and retire this repository's linked worktrees. Nothing here fetches, so
run git fetch --prune first when a branch may have merged on the remote.

Commands:
  audit   List every linked worktree and whether it is safe to retire
  retire  Remove a worktree and its landed branch, refusing to lose work
`

const auditUsage = `Usage: harness worktree audit [--into <ref>]

Print one line per linked worktree: its state, how many changed and ignored
paths it holds, its branch, its head, and its path. It removes nothing.

  safe      clean, and its branch landed (or its detached head is on a ref)
  dirty     tracked or untracked changes
  loose     detached at a commit no branch, remote branch, or tag holds
  unlanded  its branch is not proven to be on --into
  ignored   only ignored paths stand in the way; removal deletes them
  locked    locked with git worktree lock
  missing   its directory is gone; git worktree prune clears it

  --into  Ref a branch must have landed on (default: origin/HEAD, then
          origin/main, then origin/master). A branch has landed when it is an
          ancestor of the ref or merging it into the ref changes nothing, as
          after a squash merge.
`

const retireUsage = `Usage: harness worktree retire <path> [flags]
       harness worktree retire --branch <name> [--into <ref>]

Remove the linked worktree at <path> and delete its branch, and print what was
done. It refuses, changing nothing, when that would lose work: uncommitted
changes, ignored files, a branch not proven landed on --into, or a detached
head no ref holds. The main worktree is never removed.

With --branch and no path, delete a local branch that no worktree holds, once
it is proven landed; a branch that does not exist prints nothing and exits 0.

  --into           Ref the branch must have landed on (default as in audit)
  --keep-branch    Remove only the worktree and leave its branch
  --allow-ignored  Delete the ignored files with the worktree; pass it only
                   after checking each one is regenerable build output
  --at             Commit a detached worktree was created at; a head no ref
                   holds is accepted when it still equals this commit
  --save-patch     Before removal, write the worktree's uncommitted work, and
                   with --at its commits since then, to this file as a binary
                   patch; the worktree is then removed even though it is dirty
`

// Command runs `harness worktree` and returns the process exit code.
func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "audit":
		return audit(args[1:], stdout, stderr)
	case "retire":
		return retire(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness worktree: unknown command %q; run harness worktree --help\n", args[0])
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
		fmt.Fprintf(stderr, "harness worktree %s: %v\n%s", flags.Name(), err, text)
		return 2
	}
	return -1
}

// repository returns the working directory when it is inside a git repository.
func repository(name string, stderr io.Writer) (string, bool) {
	cwd, err := os.Getwd()
	if err == nil {
		_, err = git(cwd, "rev-parse", "--git-dir")
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness worktree %s: not inside a git repository\n", name)
		return "", false
	}
	return cwd, true
}

func audit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	into := flags.String("into", "", "")
	if code := parse(flags, args, auditUsage, stdout, stderr); code >= 0 {
		return code
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "harness worktree audit: unexpected argument %q; run harness worktree audit --help\n", flags.Arg(0))
		return 2
	}
	dir, ok := repository("audit", stderr)
	if !ok {
		return 2
	}
	if *into == "" {
		ref, err := DefaultInto(dir)
		if err != nil {
			fmt.Fprintf(stderr, "harness worktree audit: %v\n", err)
			return 1
		}
		*into = ref
	}
	trees, err := List(dir)
	if err != nil {
		fmt.Fprintf(stderr, "harness worktree audit: %v\n", err)
		return 1
	}
	table := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(table, "STATE\tCHANGES\tIGNORED\tBRANCH\tHEAD\tPATH\n")
	for _, t := range trees {
		if t.Main {
			continue
		}
		s, err := Inspect(t, *into)
		if err != nil {
			fmt.Fprintf(stderr, "harness worktree audit: %s: %v\n", t.Path, err)
			continue
		}
		branch, head := s.Branch, s.Head
		if branch == "" {
			branch = "(detached)"
		}
		if len(head) > 12 {
			head = head[:12]
		}
		fmt.Fprintf(table, "%s\t%d\t%d\t%s\t%s\t%s\n", s.State, len(s.Changes), len(s.Ignored), branch, head, s.Path)
	}
	table.Flush()
	return 0
}

func retire(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("retire", flag.ContinueOnError)
	var o Options
	var branch string
	flags.StringVar(&branch, "branch", "", "")
	flags.StringVar(&o.Into, "into", "", "")
	flags.StringVar(&o.At, "at", "", "")
	flags.StringVar(&o.SavePatch, "save-patch", "", "")
	flags.BoolVar(&o.AllowIgnored, "allow-ignored", false, "")
	flags.BoolVar(&o.KeepBranch, "keep-branch", false, "")
	// The path may come before the flags, as in `retire <path> --into <ref>`.
	path := ""
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		path, args = args[0], args[1:]
	}
	if code := parse(flags, args, retireUsage, stdout, stderr); code >= 0 {
		return code
	}
	rest := flags.Args()
	if path == "" && len(rest) > 0 {
		path, rest = rest[0], rest[1:]
	}
	switch {
	case len(rest) != 0:
		fmt.Fprintf(stderr, "harness worktree retire: unexpected argument %q; run harness worktree retire --help\n", rest[0])
		return 2
	case (path == "") == (branch == ""):
		fmt.Fprintln(stderr, "harness worktree retire: name a worktree path, or a branch with --branch, not both; run harness worktree retire --help")
		return 2
	case branch != "" && (o.At != "" || o.SavePatch != "" || o.AllowIgnored || o.KeepBranch):
		fmt.Fprintln(stderr, "harness worktree retire: --branch takes only --into")
		return 2
	}
	dir, ok := repository("retire", stderr)
	if !ok {
		return 2
	}
	if o.Into == "" {
		// A detached worktree, or one whose branch is kept, needs no landed check, so a missing default is only reported where it matters.
		o.Into, _ = DefaultInto(dir)
	}
	if o.At != "" {
		sha, err := git(dir, "rev-parse", "--verify", "--quiet", "--end-of-options", o.At+"^{commit}")
		if err != nil {
			fmt.Fprintf(stderr, "harness worktree retire: --at %q does not name a commit\n", o.At)
			return 2
		}
		o.At = sha
	}
	var done []string
	var err error
	if branch != "" {
		if o.Into == "" {
			fmt.Fprintf(stderr, "harness worktree retire: no ref to prove branch %s landed on; pass --into <ref>\n", branch)
			return 1
		}
		done, err = RetireBranch(dir, branch, o.Into)
	} else {
		done, err = Retire(dir, path, o)
	}
	for _, line := range done {
		fmt.Fprintln(stdout, line)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness worktree retire: %v\n", err)
		return 1
	}
	return 0
}
