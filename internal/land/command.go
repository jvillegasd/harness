package land

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/jvillegasd/harness/internal/statefile"
)

const usage = `Usage: harness land <command> [arguments]

Commands:
  check  Report whether pull requests can land, and how
`

const checkUsage = `Usage: harness land check [--merge] [--stack] [<pr>...]

Print, as a JSON array in landing order, the verdict for each pull request:
the numbers given, or with none the pull request of the branch checked out
here. It reads the forge through gh, fetches each pull request's head and
base from origin, and changes nothing else.

  --merge  The human asked for the merge. Without it an open pull request
           stops, since only a merged one is cleaned up.
  --stack  The one number is a stack's tip: every open pull request under it
           joins the run. A base with no open pull request is an error.

Each verdict carries:

  lane        open (merge it), merged (clean up after it), or stop
  mode        local when this checkout is the pull request's head, fork when
              it comes from another repository, otherwise remote
  mine        whether the forge user authored it. A landing deletes the
              remote branch, and rebases a dependent, only when it is theirs
  head        the head commit every check read; merge with
              --match-head-commit <head>
  after       the earlier pull request in the run this one is based on. Check
              it again alone when its turn comes
  key         its review directory: the head branch, or pr-<n> for a fork
  worktrees   other checkouts holding the head branch, which stay
  dependents  open pull requests based on its head, each with its own mine
  holds       every reason not to merge. Empty means the gates passed. On the
              merged lane these are what the merge skipped
  notes       findings that hold nothing

The gates are the forge's: a mergeable, non-blocked state, every required
check passing, no missing approval or requested change, no unresolved review
thread, and a base that is the default branch.

review.required is true when the review level is not off and the change
reaches above the tests-docs tier, by the changed files or by the Tiers line
of the pull request body. review.source is where the reviewed commit came
from: the body's "Reviewed:" line, else the ledger's latest clean line, else,
for a pull request someone else wrote, a person's approval with no person's
change request standing. review.status compares that commit's patch with the
head's, file by file:

  current     the head carries the reviewed patch
  tests-docs  only the test or documentation files in review.changed differ
  stale       another file differs, or the reviewed commit is gone
  missing     no source exists

A required review that is not current is a hold. findings lists the ledger's
open findings and its recorded dismissals, each a hold; dropped counts the
rest.
`

// Command runs `harness land` and returns the process exit code.
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
	}
	fmt.Fprintf(stderr, "harness land: unknown command %q; run harness land --help\n", args[0])
	return 2
}

func check(args []string, stdout, stderr io.Writer) int {
	var numbers []int
	merge, stack := false, false
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprint(stdout, checkUsage)
			return 0
		case "--merge":
			merge = true
		case "--stack":
			stack = true
		default:
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "harness land check: %q is not a pull request number or a flag; run harness land check --help\n", a)
				return 2
			}
			numbers = append(numbers, n)
		}
	}
	if stack && len(numbers) != 1 {
		fmt.Fprintln(stderr, "harness land check: --stack takes exactly one pull request number, the stack's tip")
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(stderr, "harness land check: %v\n", err)
		return 1
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fail(err)
	}
	checks, err := Run(Env{Dir: cwd, Forge: Gh(cwd), State: statefile.Path(home)}, numbers, stack, merge)
	if err != nil {
		return fail(err)
	}
	out, _ := json.MarshalIndent(checks, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}
