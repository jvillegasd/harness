package land

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

const usage = `Usage: harness land <command> [arguments]

Commands:
  check   Report whether pull requests can land, and how
  sync    Fetch from origin and fast-forward the local default branch
  finish  Clean up after merged pull requests
  repair  Move a stacked dependent onto the default branch after a squash merge
`

const repairUsage = `Usage: harness land repair <dependent pr> --landed <pr>
       harness land repair <dependent pr> --push

After a squash merge, an open pull request based on the landed head still
carries the absorbed commits. Repair moves that one dependent onto the
default branch in two steps, and refuses whenever it cannot prove the
dependent's own change survives intact.

With --landed, it reads, decides, and rebases locally. It refuses, changing
nothing, when the dependent is not the forge user's, comes from a fork, or
holds a merge commit; when no commit it was built on can be found (the
landed head when that is an ancestor, else the last of its commits whose
patch the landed head holds); and when anything it holds up to that boundary
is not already on the default branch. Only then does it retarget the
dependent to the default branch and rebase its commits past the boundary, in
the worktree holding its branch or a temporary detached one with hooks
disabled. A conflict, or an own change that is empty or differs after the
rebase, puts the branch back and exits 1. It prints the boundary, the
recorded head, the rebased head, and the worktree where the branch's checks
can run.

With --push, it publishes the rebased head with a lease pinned to the
recorded head, so a push by anyone since fails instead of being overwritten,
and removes any temporary worktree.
`

const syncUsage = `Usage: harness land sync

Fetch from origin with pruning and fast-forward the local default branch: in
the worktree that holds it, or by moving the branch when no worktree does.
A default branch with commits origin lacks is reported, left as it is, and
exits 1.
`

const finishUsage = `Usage: harness land finish <pr>... [--allow-ignored] [--keep-branch]

Clean up after merged pull requests, and print each action as done or kept,
with the reason for a kept one. A pull request that is not merged is refused
before anything is removed. It runs harness land sync first, then for each
pull request:

  - removes its review directory, keeping one that still records a review
    worktree;
  - deletes the remote head branch only when the pull request is the forge
    user's, is not from a fork, and no open pull request is based on it;
  - retires this checkout and its branch when this checkout holds the head
    branch, or the local branch by name when it does not, once proven landed
    on origin/<default>. A fork's pull request loses only its review
    directory.

Retiring this checkout comes once, after every other pull request. In the
main worktree, which is never removed, the checkout switches to the default
branch and the branch is retired by name. A refused retirement leaves the
checkout and exits 1 after printing every action taken. Running it again
changes nothing.

  --allow-ignored  Delete this checkout's ignored files with it; pass it only
                   once each is regenerable build output
  --keep-branch    Retire this checkout but keep its branch
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
	case "sync":
		return syncCommand(args[1:], stdout, stderr)
	case "finish":
		return finish(args[1:], stdout, stderr)
	case "repair":
		return repair(args[1:], stdout, stderr)
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
	e, err := cwdEnv()
	if err != nil {
		return fail(err)
	}
	checks, err := Run(e, numbers, stack, merge)
	if err != nil {
		return fail(err)
	}
	out, _ := json.MarshalIndent(checks, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}

func syncCommand(args []string, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Fprint(stdout, syncUsage)
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(stderr, "harness land sync: unexpected argument %q; run harness land sync --help\n", args[0])
		return 2
	}
	e, err := cwdEnv()
	if err != nil {
		fmt.Fprintf(stderr, "harness land sync: %v\n", err)
		return 1
	}
	a, err := Sync(e)
	if a.Text != "" {
		fmt.Fprintln(stdout, a)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness land sync: %v\n", err)
		return 1
	}
	return 0
}

func finish(args []string, stdout, stderr io.Writer) int {
	var numbers []int
	var o FinishOptions
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprint(stdout, finishUsage)
			return 0
		case "--allow-ignored":
			o.AllowIgnored = true
		case "--keep-branch":
			o.KeepBranch = true
		default:
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "harness land finish: %q is not a pull request number or a flag; run harness land finish --help\n", a)
				return 2
			}
			numbers = append(numbers, n)
		}
	}
	if len(numbers) == 0 {
		fmt.Fprintln(stderr, "harness land finish: name the merged pull requests; run harness land finish --help")
		return 2
	}
	e, err := cwdEnv()
	if err != nil {
		fmt.Fprintf(stderr, "harness land finish: %v\n", err)
		return 1
	}
	actions, err := Finish(e, numbers, o)
	for _, a := range actions {
		fmt.Fprintln(stdout, a)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness land finish: %v\n", err)
		return 1
	}
	return 0
}

func repair(args []string, stdout, stderr io.Writer) int {
	usage := func(format string, a ...any) int {
		fmt.Fprintf(stderr, "harness land repair: "+format+"; run harness land repair --help\n", a...)
		return 2
	}
	dependent, landed, push := 0, 0, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-h", "--help":
			fmt.Fprint(stdout, repairUsage)
			return 0
		case "--push":
			push = true
		case "--landed":
			if i+1 < len(args) {
				i++
				landed, _ = strconv.Atoi(args[i])
			}
			if landed <= 0 {
				return usage("--landed takes a pull request number")
			}
		default:
			n, err := strconv.Atoi(a)
			if err != nil || n <= 0 || dependent != 0 {
				return usage("%q is not the one dependent pull request number or a flag", a)
			}
			dependent = n
		}
	}
	if dependent == 0 || push == (landed != 0) {
		return usage("give the dependent pull request and either --landed <pr> or --push")
	}
	e, err := cwdEnv()
	if err != nil {
		fmt.Fprintf(stderr, "harness land repair: %v\n", err)
		return 1
	}
	var r Repair
	if push {
		r, err = PushRepair(e, dependent)
	} else {
		r, err = RepairDependent(e, dependent, landed)
	}
	if err != nil {
		fmt.Fprintf(stderr, "harness land repair: %v\n", err)
		return 1
	}
	out, _ := json.MarshalIndent(r, "", "  ")
	fmt.Fprintln(stdout, string(out))
	return 0
}
