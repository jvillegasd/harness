# 11 Forge state in the worktree audit and the review prune list

## What to build

`harness worktree audit --forge` adds, for each worktree's branch, its pull request's number and state. For a branch the audit calls `unlanded` whose pull request merged, it fetches that pull request's head and prints the ref to pass as `--into` when retiring, which proves the branch holds nothing the merged pull request lacked. A branch with an open pull request is marked as held by it.

`harness review prune --dry-run --forge` marks each listed `pr-<n>` directory whose pull request is still open, since that ledger is what `harness land check` reads.

`cleanup` steps 2 and 5 call these flags and keep the recommendation and the wait for the human.

## Acceptance criteria

- Without `--forge`, both commands print exactly what they print today and call the forge zero times.
- With `--forge`, a worktree whose branch has an open pull request shows the number and `OPEN`.
- An `unlanded` row whose pull request is `MERGED` shows the merged head as its `--into`, and `harness worktree retire <path> --into <that ref>` then succeeds for a branch equal to the merged head.
- A branch with no pull request shows none, and the row's state is unchanged.
- A forge read that fails marks that row's forge state unavailable and still prints every row.
- `review prune --dry-run --forge` marks an open pull request's `pr-<n>` directory and leaves a merged or closed one unmarked.
- Neither flag removes anything.
- `cleanup` names the flags and no longer spells out the per-row `gh` calls.

## Seam

Proposed, awaiting the human's confirmation: the worktree package's and the review ledger package's tests, each with a fake forge and throwaway repositories, settle the added columns; one command test apiece in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Retiring anything on the strength of the forge state. Deleting remote branches.
