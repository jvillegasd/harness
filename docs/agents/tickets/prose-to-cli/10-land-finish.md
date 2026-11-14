# 10 Sync the default branch and clean up after a landing

## What to build

Two commands take over the mechanical end of a landing.

`harness land sync` fetches from origin with pruning and fast-forwards the local default branch: in the worktree that holds it, or by moving the branch when no worktree does. A default branch that cannot fast-forward is reported and left as it is.

`harness land finish <pr>... [--allow-ignored] [--keep-branch]` cleans up after merged pull requests, in the order that keeps the last step from removing the directory the session runs in. It runs the sync first, then for each pull request:

- removes its review directory, reporting and continuing when a recorded review worktree holds it;
- deletes the remote head branch only when the pull request is the forge user's, is not from a fork, and no open pull request is still based on that branch;
- retires this checkout and its branch when this checkout is the pull request's head, retires the branch by name when it is not, and does nothing more for a fork.

Retiring this checkout happens once, after every other pull request in the run. Each action is printed as done or kept, with the reason for a kept one. A pull request that is not merged is refused before anything is removed.

`land` steps 5 and 6 call these commands, with tracker reconciliation between them, and keep the judgment about when `--allow-ignored` and `--keep-branch` are safe. `local-tracker` and `land`'s tracker reference name `land sync` for updating the default branch.

## Acceptance criteria

- `sync` fast-forwards a default branch held by another worktree in that worktree, and one held by none without checking it out.
- `sync` with a diverged local default branch changes nothing, reports it, and exits non-zero.
- `finish` on an open pull request is refused with nothing removed.
- For a merged pull request that is the forge user's with no dependents, `finish` removes the review directory, the remote branch, the worktree, and the local branch.
- A remote branch another open pull request is based on is kept and reported.
- Someone else's remote branch is kept and reported.
- A fork's pull request loses only its review directory.
- A review directory that still records a worktree is kept and reported, and the rest of the cleanup still runs.
- A refused worktree retirement leaves the checkout and exits non-zero after printing every action already taken.
- With two pull requests named, this checkout is retired last.
- Running `finish` a second time changes nothing and exits 0.
- `land`, its tracker reference, and `local-tracker` name the commands, and the hand-run sequence is gone from them.

## Seam

Proposed, awaiting the human's confirmation: the landing package's tests over throwaway repositories with a bare origin and its fake forge settle the order and the conditions; one command test in `cmd/harness` proves the wiring. The retirement rules stay proven by the worktree package's existing tests.

## Blocked by

none

## Out of scope

Merging. Repairing dependents (ticket 08). Tracker transitions.
