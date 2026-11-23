package review

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/jvillegasd/harness/internal/run"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/jvillegasd/harness/internal/ui"
)

const usage = `Usage: harness review <command> [arguments]

Post a review's verdict on its pull request and keep a review run's scratch.
Findings and verdicts live on the pull request as one verdict comment per
review, and one per run-as-user check, which harness land check and harness
pr status read; nothing about either result is kept on this machine.

Commands:
  verdict   Post a review's or a run-as-user check's verdict comment on its
            pull request
  level     Print or set the review level: off, medium (default), or high
  target    Resolve what a review reads: its base, commit, and key
  snapshot  Save owned uncommitted work as a patch and print its digest
  worktree  Add or retire the one worktree a review may create
  remove    Delete a key's review directory, its worktree record and patches
  prune     Delete the review directories of branches that exist nowhere
`

const verdictUsage = `Usage: harness review verdict <pr> --result <PASS|PASS+NOTES|FAIL> --base <base> --sha <sha> --level <medium|high> [--notes <file>] [--dry-run]
       harness review verdict <pr> --run --result <PASS|PASS+NOTES|FAIL> --sha <sha> [--notes <file>] [--dry-run]

Post one verdict on pull request <pr> as one comment, and print it. A review's
verdict names the reviewed <base>..<sha> and the level. With --run it is a
run-as-user verdict: an agent that did not write the change drove it at <sha>,
and it takes no --base or --level. The comment starts with a hidden marker
naming the kind, the result, and the full commit ids, which harness land check
parses; only a verdict by someone with write access to the repository counts
there. --sha must be the pull request's current head, so a verdict always
names a commit the pull request holds.

  PASS        nothing left to report
  PASS+NOTES  the change may land; the notes say what the human should read
  FAIL        a confirmed finding is unfixed, part of the change went
              unreviewed, or a driven feature missed its end state

  --run      Post a run-as-user verdict instead of a review's
  --notes    A Markdown file, required for PASS+NOTES and FAIL. For a review:
             the confirmed findings, each with its path, lines, severity, and
             the run that proved it, plus coverage and dropped counts. For a
             run: each feature driven with its result and evidence, and the
             gaps
  --dry-run  Check everything, print the comment, and post nothing
`

const worktreeUsage = `Usage: harness review worktree add [--branch <key>] [--patch <file>] <sha>
       harness review worktree retire [--branch <key>] [--allow-ignored]

Own the one worktree a review may create, recorded in worktree.txt in the
key's review directory (default key: the current branch). While the worktree
it names exists, harness review remove and prune leave the directory alone.

add picks a fresh sibling directory outside the checkout, records it before
anything else, creates a detached worktree there at <sha> with Git hooks
disabled, applies --patch when given, and prints the path. Run again while
that worktree exists at the same commit, it prints the same path and changes
nothing.

retire removes the recorded worktree without losing work: whatever it holds
is first saved as <worktree name>.patch in the review directory, and the
record is deleted only once the worktree is gone. It refuses over ignored
paths unless --allow-ignored, which deletes them; pass it only once each is
regenerable build output. A refusal leaves the worktree and the record and
exits 1. With no record it does nothing.
`

const targetUsage = `Usage: harness review target [<pr number or URL> | <branch> | <base>..<sha>]

Print, as JSON, what a review of the target reads. It fetches what it reads
from origin without tags, and creates no branch or tag.

  (none)          the current task branch; a detached head is refused
  <pr>            a pull request by number or URL; its head is fetched and
                  must be the head the forge reports. base is its merge-base
                  with the pull request's base branch, and the key its head
                  branch, or pr-<n> for a fork
  <branch>        a local branch, or origin/<name> (fetched first). base is
                  its merge-base with its open pull request's base, or with
                  the default branch when it has none; the key is the name
                  without the remote
  <base>..<sha>   a range; the key is range-<base>-<sha>, each cut to 12

Fields: kind (branch, pr, or range), base and sha as full commit ids, key
(the review directory's name),
foreign (anything but the current task branch: the review is report-only),
and fork (a fork's pull request: nothing from it is run).
`

const snapshotUsage = `Usage: harness review snapshot --base <base> [--branch <key>] -- <owned paths>...

Save tracked and untracked changes under the owned paths, given relative to
the repository root, as one binary patch against base in the key's review
directory (default key: the current branch). Print the patch path and its
sha256 digest on separate lines: the digest is the content identity of the
reviewed work. The index and working tree are left as they were. With
no change under the owned paths it writes nothing and exits 1.
`

const removeUsage = `Usage: harness review remove [--branch <key>]

Delete the key's review directory, its worktree record and saved patches
(default: the current branch), and print the directory removed. A key with no
directory prints nothing and exits 0. A directory whose worktree.txt, review's
record of a worktree it could not retire, names a directory that still exists
is refused untouched; one whose recorded directory is gone is removed.
`

const pruneUsage = `Usage: harness review prune [--dry-run]

Delete the review directory of every branch that exists neither as a local
branch nor as a remote-tracking branch, and print each directory removed. Run
git fetch --prune first so a branch deleted on the remote counts as gone. A
directory whose worktree.txt names a directory that still exists is kept and
named on stderr; once that worktree is retired, by path or by harness review
worktree retire, the directory goes like any other. A review keyed pr-<n> (a
fork's pull request) or range-<base>-<sha> has no branch and is always
pruned; a review directory holds only scratch, so nothing a landing reads is
lost.

  --dry-run  Print what would be removed, and remove nothing
`

func Command(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	case "snapshot":
		return snapshot(args[1:], stdout, stderr)
	case "remove":
		return remove(args[1:], stdout, stderr)
	case "prune":
		return prune(args[1:], stdout, stderr)
	case "verdict":
		return verdict(args[1:], stdout, stderr)
	case "level":
		return level(args[1:], stdout, stderr)
	case "target":
		return target(args[1:], stdout, stderr)
	case "worktree":
		return worktreeCommand(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness review: unknown command %q; run harness review --help\n", args[0])
	return 2
}

func snapshot(args []string, stdout, stderr io.Writer) int {
	separator := slices.Index(args, "--")
	head := args
	if separator >= 0 {
		head = args[:separator]
	}
	if slices.ContainsFunc(head, func(a string) bool { return a == "-h" || a == "--help" }) {
		fmt.Fprint(stdout, snapshotUsage)
		return 0
	}
	if separator < 0 || separator == len(args)-1 {
		fmt.Fprintf(stderr, "harness review snapshot: provide -- followed by at least one owned path\n%s", snapshotUsage)
		return 2
	}
	flags := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	var base, branch string
	flags.StringVar(&base, "base", "", "")
	flags.StringVar(&branch, "branch", "", "")
	if code := parse(flags, args[:separator], snapshotUsage, stdout, stderr); code >= 0 {
		return code
	}
	if base == "" {
		fmt.Fprintf(stderr, "harness review snapshot: --base is required\n%s", snapshotUsage)
		return 2
	}
	root, branch, _, code := reviewDir("snapshot", branch, stderr)
	if code >= 0 {
		return code
	}
	path, digest, err := Snapshot(root, base, branch, args[separator+1:])
	if err != nil {
		fmt.Fprintf(stderr, "harness review snapshot: %v\n", err)
		if errors.Is(err, ErrInvalid) {
			return 2
		}
		return 1
	}
	fmt.Fprintf(stdout, "%s\n%s\n", path, digest)
	return 0
}

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

func repoRoot(name string, stderr io.Writer) (string, int) {
	cwd, err := os.Getwd()
	root := ""
	if err == nil {
		root, err = run.Git(cwd, "rev-parse", "--show-toplevel")
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: not inside a git repository\n", name)
		return "", 2
	}
	return root, 0
}

func reviewDir(name, key string, stderr io.Writer) (root, resolved, dir string, code int) {
	root, code = repoRoot(name, stderr)
	if code != 0 {
		return "", "", "", code
	}
	if key == "" {
		key, _ = run.Git(root, "branch", "--show-current")
	}
	dir, err := Dir(root, key)
	if errors.Is(err, ErrInvalid) {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return "", "", "", 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return "", "", "", 1
	}
	return root, key, dir, -1
}

func remove(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	var branch string
	flags.StringVar(&branch, "branch", "", "")
	if code := parse(flags, args, removeUsage, stdout, stderr); code >= 0 {
		return code
	}
	_, _, dir, code := reviewDir("remove", branch, stderr)
	if code >= 0 {
		return code
	}
	removed, err := RemoveBranch(dir)
	if err != nil {
		fmt.Fprintf(stderr, "harness review remove: %v\n", err)
		return 1
	}
	if removed {
		if terminal(stdout) {
			fmt.Fprintln(stdout, ui.For(stdout).OK("removed "+dir))
		} else {
			fmt.Fprintln(stdout, dir)
		}
	}
	return 0
}

func prune(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "")
	if code := parse(flags, args, pruneUsage, stdout, stderr); code >= 0 {
		return code
	}
	root, code := repoRoot("prune", stderr)
	if code != 0 {
		return code
	}
	live := map[string]bool{}

	for strip, refs := range map[string]string{"2": "refs/heads", "3": "refs/remotes"} {
		out, err := run.Git(root, "for-each-ref", "--format=%(refname:lstrip="+strip+")", refs)
		if err != nil {
			fmt.Fprintf(stderr, "harness review prune: %v\n", err)
			return 1
		}
		for _, name := range strings.Split(out, "\n") {
			live[name] = true
		}
	}
	dir, err := BranchesDir(root)
	if err != nil {
		fmt.Fprintf(stderr, "harness review prune: %v\n", err)
		return 1
	}
	removed, held, err := Prune(dir, func(b string) bool { return live[b] }, *dryRun)
	for _, d := range removed {
		if terminal(stdout) {
			s := ui.For(stdout)
			if *dryRun {
				fmt.Fprintln(stdout, s.Dim("would remove "+d))
			} else {
				fmt.Fprintln(stdout, s.OK("removed "+d))
			}
		} else {
			fmt.Fprintln(stdout, d)
		}
	}
	for _, d := range held {
		fmt.Fprintf(stderr, "harness review prune: kept %s: worktree.txt records a review worktree that still exists\n", d)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review prune: %v\n", err)
		return 1
	}
	return 0
}

func target(args []string, stdout, stderr io.Writer) int {
	arg := ""
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprint(stdout, targetUsage)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "harness review target: unknown flag %q; run harness review target --help\n", a)
			return 2
		case arg != "":
			fmt.Fprintf(stderr, "harness review target: one target only, not %q and %q\n", arg, a)
			return 2
		default:
			arg = a
		}
	}
	root, code := repoRoot("target", stderr)
	if code != 0 {
		return code
	}
	t, err := ResolveTarget(root, run.Gh(root), arg)
	if errors.Is(err, ErrInvalid) {
		fmt.Fprintf(stderr, "harness review target: %v\n", err)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review target: %v\n", err)
		return 1
	}
	out, _ := json.MarshalIndent(t, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func worktreeCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, worktreeUsage)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, worktreeUsage)
		return 0
	case "add", "retire":
	default:
		fmt.Fprintf(stderr, "harness review worktree: unknown command %q; run harness review worktree --help\n", args[0])
		return 2
	}
	name := "worktree " + args[0]
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	var branch, patch string
	var allowIgnored bool
	flags.StringVar(&branch, "branch", "", "")
	if args[0] == "add" {
		flags.StringVar(&patch, "patch", "", "")
	} else {
		flags.BoolVar(&allowIgnored, "allow-ignored", false, "")
	}

	var positional []string
	rest := args[1:]
	for {
		if err := flags.Parse(rest); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				fmt.Fprint(stdout, worktreeUsage)
				return 0
			}
			fmt.Fprintf(stderr, "harness review %s: %v\n%s", name, err, worktreeUsage)
			return 2
		}
		if flags.NArg() == 0 {
			break
		}
		positional = append(positional, flags.Arg(0))
		rest = flags.Args()[1:]
	}
	want := 0
	if args[0] == "add" {
		want = 1
	}
	if len(positional) != want {
		fmt.Fprintf(stderr, "harness review %s: takes %d argument(s), got %d; run harness review worktree --help\n", name, want, len(positional))
		return 2
	}
	root, key, _, code := reviewDir(name, branch, stderr)
	if code >= 0 {
		return code
	}
	if args[0] == "add" {
		path, err := AddWorktree(root, key, positional[0], patch)
		if err != nil {
			fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
			if errors.Is(err, ErrInvalid) {
				return 2
			}
			return 1
		}
		fmt.Fprintln(stdout, path)
		return 0
	}
	done, err := RetireWorktree(root, key, allowIgnored)
	for _, line := range done {
		fmt.Fprintln(stdout, ui.For(stdout).OK(line))
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return 1
	}
	return 0
}

func verdict(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("verdict", flag.ContinueOnError)
	var v Verdict
	var notesFile string
	flags.StringVar(&v.Result, "result", "", "")
	flags.StringVar(&v.Base, "base", "", "")
	flags.StringVar(&v.SHA, "sha", "", "")
	flags.StringVar(&v.Level, "level", "", "")
	flags.StringVar(&notesFile, "notes", "", "")
	flags.BoolVar(&v.Run, "run", false, "")
	dryRun := flags.Bool("dry-run", false, "")

	var rest, positional []string
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case strings.HasPrefix(a, "-") && !strings.Contains(a, "=") && a != "--dry-run" && a != "--run" && a != "-h" && a != "--help" && i+1 < len(args):
			rest = append(rest, a, args[i+1])
			i++
		case strings.HasPrefix(a, "-"):
			rest = append(rest, a)
		default:
			positional = append(positional, a)
		}
	}
	if code := parse(flags, rest, verdictUsage, stdout, stderr); code >= 0 {
		return code
	}
	number, err := 0, error(nil)
	if len(positional) == 1 {
		number, err = strconv.Atoi(positional[0])
	}
	if len(positional) != 1 || err != nil || number <= 0 || v.Result == "" || v.SHA == "" || !v.Run && (v.Base == "" || v.Level == "") {
		fmt.Fprintf(stderr, "harness review verdict: give one pull request number, --result, and --sha, with --base and --level for a review or --run for a run; run harness review verdict --help\n")
		return 2
	}
	notes := ""
	if notesFile != "" {
		raw, err := os.ReadFile(notesFile)
		if err != nil {
			fmt.Fprintf(stderr, "harness review verdict: %v\n", err)
			return 2
		}
		notes = string(raw)
	}
	root, code := repoRoot("verdict", stderr)
	if code != 0 {
		return code
	}
	for _, rev := range []*string{&v.Base, &v.SHA} {
		if *rev == "" {

			continue
		}
		full, err := run.Git(root, "rev-parse", "--verify", "--quiet", "--end-of-options", *rev+"^{commit}")
		if err != nil {
			fmt.Fprintf(stderr, "harness review verdict: %q does not name a commit here; fetch it first\n", *rev)
			return 2
		}
		*rev = full
	}
	if err := v.Validate(notes); err != nil {
		fmt.Fprintf(stderr, "harness review verdict: %v\n", err)
		return 2
	}
	forge := run.Gh(root)
	raw, err := forge("pr", "view", strconv.Itoa(number), "--json", "state,headRefOid")
	var pull struct{ State, HeadRefOid string }
	if err == nil {
		err = json.Unmarshal(raw, &pull)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review verdict: %v\n", err)
		return 1
	}
	switch {
	case pull.State != "OPEN":
		fmt.Fprintf(stderr, "harness review verdict: pull request #%d is %s, and a verdict is posted only on an open one\n", number, strings.ToLower(pull.State))
		return 1
	case pull.HeadRefOid != v.SHA:
		fmt.Fprintf(stderr, "harness review verdict: pull request #%d's head is %s, not %s, the commit this verdict covers; push that commit, or get a verdict for the new head\n", number, pull.HeadRefOid, v.SHA)
		return 1
	}
	body := Body(v, notes)
	fmt.Fprint(stdout, body)
	if *dryRun {
		return 0
	}
	if err := PostVerdict(forge, number, body); err != nil {
		fmt.Fprintf(stderr, "harness review verdict: %v\n", err)
		return 1
	}
	return 0
}

func terminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && term.IsTerminal(f.Fd())
}
