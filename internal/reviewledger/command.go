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

	"github.com/jvillegasd/harness/internal/statefile"
)

const usage = `Usage: harness review <command> [arguments]

Record and read a branch's local review ledger, kept at
<git-common-dir>/harness-review/branches/<branch>/ledger.jsonl. Git never
tracks it, and /land removes the branch directory with the branch.

Commands:
  record  Append a new finding, or a new disposition for an existing one
  snapshot  Save owned uncommitted work as a patch and print its digest
  list    Print the latest line of each finding on a branch as JSON
  path    Print the ledger file's path
  remove  Delete a branch's ledger and patches, once the branch has merged
  prune   Delete the review directories of branches that exist nowhere
  level   Print or set the review level: off, medium (default), or high
  covers  Say whether a recorded review already covers a patch
  stamp   Print the review line a pull request body carries
  target  Resolve what a review reads: its base, commit, and ledger key
  worktree  Add or retire the one worktree a review may create
`

const worktreeUsage = `Usage: harness review worktree add [--branch <key>] [--patch <file>] <sha>
       harness review worktree retire [--branch <key>] [--allow-ignored]

Own the one worktree a review may create, recorded in worktree.txt in the
key's review directory (default key: the current branch). While that record
exists, harness review remove and prune leave the directory alone.

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

Fields: kind (branch, pr, or range), base and sha as full commit ids, key,
foreign (anything but the current task branch: the review is report-only),
and fork (a fork's pull request: nothing from it is run).
`

const stampUsage = `Usage: harness review stamp [--branch <key>] [--head <rev>] [--base <rev>]

Print the one review line a pull request body carries for --head (default:
HEAD), taken from the branch's ledger, so the line harness land check parses
is never composed by hand:

  Reviewed: <base>..<sha>          the latest clean record still covers the
                                   head, compared file by file by patch, and
                                   no finding is open
  Review: not run (level <level>)  the latest whole-review record is
                                   no-review

Otherwise it prints nothing on stdout, names the reason on stderr (a finding
still open, no clean record, a change the review never read, or no review
recorded), and exits 1.

  --base  The pull request's base the change is compared against (default:
          origin/HEAD, then origin/main, then origin/master, then the
          record's own base)
`

const coversUsage = `Usage: harness review covers --base <base> --sha <sha> --level <medium|high> [--branch <key>] [--digest <hash>]

Print, as JSON, whether a recorded review already covers the change
<base>..<sha>, and write nothing. A whole-review clean record is the one
marker of a complete review. The branch's latest clean record at --level or
deeper covers the change when it is the same change file by file, compared by
patch as harness land check compares a reviewed commit with a head, so a
rebase that changes no file's patch keeps its review. A record made with
--digest covers only a request with that same --digest, and one made without
covers only committed work.

  covered  true when the record covers the change
  depth    the covering review's depth
  base     the covering review's recorded base
  sha      its reviewed commit
  open     each finding still open at --sha
`

const recordUsage = `Usage: harness review record [flags]

Without --id, append a new line and print its id. A finding needs --depth,
--disposition, --severity, --path, and --summary; a whole-review line
(--disposition clean or no-review) takes no --path.

With --id, append a new state for that finding: only --disposition,
--evidence, and --commit are accepted, and the rest is copied from its latest
line. A fixed, dropped, or accepted disposition needs --evidence; fixed also
needs --commit. The nit disposition fits only a nit or pre-existing finding.

  --id           Existing finding to update
  --branch       Branch (default: the current branch)
  --base         Base revision
  --sha          Reviewed commit (default: HEAD)
  --digest       Snapshot patch hash when the reviewed work is uncommitted
  --depth        none, medium, or high
  --path         File the finding cites
  --lines        A line or range, such as 40 or 40-52
  --severity     bug, risk, q, pre-existing, or nit
  --summary      One-line description
  --disposition  confirmed, fixed, dropped, accepted, nit, clean, or
                 no-review. A confirmed or fixed finding cannot be dropped.
  --evidence     The run's output, what a dropped finding's run showed, or
                 the human's words
  --commit       The fixing commit, for --disposition fixed
`

const listUsage = `Usage: harness review list [--branch <name>] [--head <rev>]

Print, as a JSON array, the latest line of each finding and whole-review line
on the branch (default: the current branch); [] means nothing is recorded for
it here. "open" is true for confirmed, and for fixed until the
fixing commit is in --head (default: HEAD). "rewritten" marks a reviewed commit
no longer in --head's history, as after a rebase; the finding still counts.
`

const snapshotUsage = `Usage: harness review snapshot --base <base> [--branch <key>] -- <owned paths>...

Save tracked and untracked changes under the owned paths, given relative to
the repository root, as one binary patch against base in the key's review
directory (default key: the current branch). Print the patch path and its
sha256 digest on separate lines: the digest is the content identity review
records with --digest. The index and working tree are left as they were. With
no change under the owned paths it writes nothing and exits 1.
`

const pathUsage = `Usage: harness review path [--branch <name>]

Print the ledger file path for the branch (default: the current branch).
`

const removeUsage = `Usage: harness review remove [--branch <name>]

Delete the branch's review directory, its ledger and saved patches (default:
the current branch), and print the directory removed. A branch with nothing
recorded prints nothing and exits 0. A directory that still holds worktree.txt,
review's record of a worktree it could not retire, is refused untouched.
`

const pruneUsage = `Usage: harness review prune [--dry-run]

Delete the review directory of every branch that exists neither as a local
branch nor as a remote-tracking branch, and print each directory removed. Run
git fetch --prune first so a branch deleted on the remote counts as gone. A
directory that still holds worktree.txt is kept and named on stderr. A review
keyed pr-<n> (a fork's pull request) or range-<base>-<sha> has no branch and is
always pruned, even while that pull request is open, so run --dry-run first and
prune only once each listed directory is approved for removal.

  --dry-run  Print what would be removed, and remove nothing
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
	case "snapshot":
		return snapshot(args[1:], stdout, stderr)
	case "list":
		return list(args[1:], stdout, stderr)
	case "path":
		return printPath(args[1:], stdout, stderr)
	case "remove":
		return remove(args[1:], stdout, stderr)
	case "prune":
		return prune(args[1:], stdout, stderr)
	case "level":
		return level(args[1:], stdout, stderr)
	case "covers":
		return covers(args[1:], stdout, stderr)
	case "stamp":
		return stamp(args[1:], stdout, stderr)
	case "target":
		return target(args[1:], stdout, stderr)
	case "worktree":
		return worktreeCommand(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "harness review: unknown command %q; run harness review --help\n", args[0])
	return 2
}

func snapshot(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "-h" || a == "--help" {
			fmt.Fprint(stdout, snapshotUsage)
			return 0
		}
	}
	separator := -1
	for i, arg := range args {
		if arg == "--" {
			separator = i
			break
		}
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
	root, branch, _, code := ledger("snapshot", branch, stderr)
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
	flags.StringVar(&e.Depth, "depth", "", "")
	flags.StringVar(&path, "path", "", "")
	flags.StringVar(&e.Lines, "lines", "", "")
	flags.StringVar(&e.Severity, "severity", "", "")
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
			fmt.Fprintf(stderr, "harness review record: finding %s is %s, and a proven finding cannot be dropped; record the human's decision as accepted instead\n", id, prior.Disposition)
			return 2
		}
		prior.Disposition, prior.Evidence, prior.Commit = e.Disposition, e.Evidence, e.Commit
		e = prior
	} else {
		e.Digest, e.Path = optional(digest), optional(path)
		e.Branch = branch
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
		id = short + "-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	e.ID = id
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

func remove(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	var branch string
	flags.StringVar(&branch, "branch", "", "")
	if code := parse(flags, args, removeUsage, stdout, stderr); code >= 0 {
		return code
	}
	_, _, file, code := ledger("remove", branch, stderr)
	if code >= 0 {
		return code
	}
	removed, err := RemoveBranch(file)
	if err != nil {
		fmt.Fprintf(stderr, "harness review remove: %v\n", err)
		return 1
	}
	if removed {
		fmt.Fprintln(stdout, filepath.Dir(file))
	}
	return 0
}

func prune(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("prune", flag.ContinueOnError)
	dryRun := flags.Bool("dry-run", false, "")
	if code := parse(flags, args, pruneUsage, stdout, stderr); code >= 0 {
		return code
	}
	cwd, err := os.Getwd()
	root := ""
	if err == nil {
		root, err = git(cwd, "rev-parse", "--show-toplevel")
	}
	if err != nil {
		fmt.Fprintln(stderr, "harness review prune: not inside a git repository")
		return 2
	}
	live := map[string]bool{}
	// A local branch is refs/heads/<name>; a remote-tracking one is refs/remotes/<remote>/<name>.
	for strip, refs := range map[string]string{"2": "refs/heads", "3": "refs/remotes"} {
		out, err := git(root, "for-each-ref", "--format=%(refname:lstrip="+strip+")", refs)
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
		fmt.Fprintln(stdout, d)
	}
	for _, d := range held {
		fmt.Fprintf(stderr, "harness review prune: kept %s: worktree.txt still records a review worktree\n", d)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review prune: %v\n", err)
		return 1
	}
	return 0
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func covers(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("covers", flag.ContinueOnError)
	var branch, base, sha, depth, digest string
	flags.StringVar(&branch, "branch", "", "")
	flags.StringVar(&base, "base", "", "")
	flags.StringVar(&sha, "sha", "", "")
	flags.StringVar(&depth, "level", "", "")
	flags.StringVar(&digest, "digest", "", "")
	if code := parse(flags, args, coversUsage, stdout, stderr); code >= 0 {
		return code
	}
	if base == "" || sha == "" || depth == "" {
		fmt.Fprintf(stderr, "harness review covers: --base, --sha, and --level are required\n%s", coversUsage)
		return 2
	}
	root, branch, _, code := ledger("covers", branch, stderr)
	if code >= 0 {
		return code
	}
	c, err := Covers(root, branch, base, sha, depth, digest)
	if errors.Is(err, ErrInvalid) {
		fmt.Fprintf(stderr, "harness review covers: %v\n", err)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review covers: %v\n", err)
		return 1
	}
	out, _ := json.MarshalIndent(c, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func stamp(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stamp", flag.ContinueOnError)
	var branch, head, base string
	flags.StringVar(&branch, "branch", "", "")
	flags.StringVar(&head, "head", "HEAD", "")
	flags.StringVar(&base, "base", "", "")
	if code := parse(flags, args, stampUsage, stdout, stderr); code >= 0 {
		return code
	}
	root, branch, _, code := ledger("stamp", branch, stderr)
	if code >= 0 {
		return code
	}
	if base == "" {
		for _, ref := range []string{"refs/remotes/origin/HEAD", "refs/remotes/origin/main", "refs/remotes/origin/master"} {
			if _, err := git(root, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err == nil {
				base = ref
				break
			}
		}
	}
	level := "medium"
	if home, err := os.UserHomeDir(); err == nil {
		if l, _, err := LoadLevel(statefile.Path(home)); err == nil {
			level = l
		}
	}
	line, err := Stamp(root, branch, head, base, level)
	if errors.Is(err, ErrInvalid) {
		fmt.Fprintf(stderr, "harness review stamp: %v\n", err)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review stamp: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, line)
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
	cwd, err := os.Getwd()
	root := ""
	if err == nil {
		root, err = git(cwd, "rev-parse", "--show-toplevel")
	}
	if err != nil {
		fmt.Fprintln(stderr, "harness review target: not inside a git repository")
		return 2
	}
	t, err := ResolveTarget(root, Gh(root), arg)
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
	// Flags may follow the commit, so parse around the one positional argument.
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
	root, key, _, code := ledger(name, branch, stderr)
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
		fmt.Fprintln(stdout, line)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness review %s: %v\n", name, err)
		return 1
	}
	return 0
}
