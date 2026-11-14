# 01 Pull request status verdict

## What to build

`harness pr status [--stack] [<pr>...]` reports, in one pass and without merge authority, whether each pull request is ready to merge and what stands in the way. It prints one JSON verdict per pull request: the forge gates, the review coverage, the ledger's open findings, and every unresolved review thread with its id, path, line, author, the author's permission on the base repository, the first comment's text, and its URL. With no number it reads the pull request of the branch checked out here; `--stack` treats the one number as a stack's tip and reports bottom-up.

The gates and the review comparison are the ones `harness land check --merge` already applies, so the two commands can never disagree about the same head. `land check` keeps deciding whether a landing may proceed; `pr status` only reports.

The exit code names the first blocker in the order `address-review` works them: 0 ready, 3 merge conflict, 4 unresolved review thread, 5 failing required check, 6 pending required check, 7 any other hold (a draft, a missing approval, a required review that is not current, an open ledger finding). 1 stays an operational failure and 2 an invalid invocation.

`open-pr`'s readiness report, `address-review`'s `check` mode, and the status pass in `open-pr`'s stack reference call this command and stop listing the facts to gather by hand.

## Acceptance criteria

- An open pull request with every gate passing and a current review prints `ready: true` with empty `holds` and exits 0.
- A failing required check exits 5 and its hold names the check; a pending one exits 6.
- An unresolved review thread exits 4, and `threads` carries its id, path, line, author, author permission, first comment text, and URL. Resolved threads are not listed.
- A pull request with both a conflict and a failing check exits 3.
- For the same pull request and head, the `review` object and the gate holds equal what `harness land check --merge` prints.
- `--stack <tip>` prints one verdict per open pull request under the tip, bottom-up.
- The command writes nothing to the forge and creates no local branch.
- `harness land check` output is unchanged for every existing case.
- `open-pr`, `address-review`, and `open-pr`'s stack reference name the command, and the hand-gathered lists are gone from them.
- `AGENTS.md` lists the command and its validation boundary.

## Seam

The landing package's tests over its existing fake forge and throwaway repositories settle the verdict and exit-code rules; one command test in `cmd/harness` proves the CLI is wired to them.

## Blocked by

none

## Out of scope

Polling until checks settle (ticket 02). Replying to or resolving threads (ticket 09). Any forge other than GitHub.
