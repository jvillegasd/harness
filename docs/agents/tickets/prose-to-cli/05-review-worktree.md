# 05 Review worktree lifecycle

## What to build

Two commands own the one worktree a review may create, so the record that protects it is never written or cleared by hand.

`harness review worktree add --branch <key> <sha> [--patch <file>]` picks a fresh sibling directory outside the checkout, records it in the key's review directory before anything else, creates a detached worktree there at the commit with Git hooks disabled, applies the saved patch when one is given, and prints the path. Run again while that worktree exists at the same commit, it prints the same path and changes nothing.

`harness review worktree retire --branch <key> [--allow-ignored]` retires the recorded worktree losslessly: whatever work it holds is saved as a patch in the review directory first, and the record is deleted only once the worktree is gone. A refusal leaves the worktree and the record in place and names its reason.

`review`'s Target and Verify sections, and `verification`'s Verdict step that borrows the same recipe, call these commands.

## Acceptance criteria

- `add` creates a detached worktree at the commit outside the checkout and prints its path, and the review directory records that path.
- The record exists before the worktree does: when worktree creation fails, the record still names the intended path.
- A second `add` for the same key and commit prints the same path and creates nothing.
- A repository hook that would fail on checkout does not run during `add`.
- `add --patch` leaves the patch's changes in the new worktree.
- `retire` on a clean worktree removes it and the record. On a worktree holding changes it writes them to a patch in the review directory, then removes both.
- `retire` refused over ignored paths leaves the worktree and the record, and exits non-zero. With `--allow-ignored` it proceeds.
- While a record exists, `harness review remove` and `harness review prune` still refuse that directory, as today.
- `review` and `verification` name the commands and no longer spell out the record, the `git worktree add`, or the record's deletion.

## Seam

Proposed, awaiting the human's confirmation: the review ledger package's tests over throwaway repositories settle the lifecycle; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Moving the worktree away from a sibling path. Launching the executor that works in it.
