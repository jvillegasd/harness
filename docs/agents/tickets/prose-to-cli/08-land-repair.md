# 08 Repair a stacked dependent after a squash merge

## What to build

After a pull request is squash-merged, each open pull request based on its head still carries the absorbed commits. `harness land repair <dependent pr> --landed <pr>` moves one such dependent onto the default branch, and refuses whenever it cannot prove the dependent's own change survives intact. It is the one place the harness rewrites a pushed branch, so it runs in two steps.

The first step reads, decides, and rebases locally:

- It refuses, changing nothing, when the dependent is not the forge user's, comes from a fork, or holds a merge commit.
- It finds the boundary, the commit the dependent was built on: the landed head when that is an ancestor of the dependent, otherwise the last of the dependent's commits whose patch the landed head already holds. With no such commit it refuses.
- It verifies the boundary by content: everything the dependent holds up to it must already be on the default branch. Otherwise it refuses.
- Only then does it retarget the dependent to the default branch and rebase the dependent's commits past the boundary onto it, in the worktree that holds the dependent's branch or, when none does, in a temporary detached worktree with hooks disabled.
- It confirms the dependent's own change is non-empty and identical before and after the rebase, by patch identity. A difference stops the repair with the differing files named.
- It prints the boundary, the dependent's recorded head, the rebased head, and where the result sits so the branch's own checks can run there.

The second step, `--push`, publishes the rebased head with a lease pinned to the recorded head, so a push by anyone since then fails instead of being overwritten, and removes any temporary worktree.

`land`'s stacked reference keeps what to do with a refusal and calls this command for the rest.

## Acceptance criteria

- A dependent built directly on the landed head ends with only its own commits on top of the default branch, and its own patch identity unchanged.
- A dependent still sitting on the landed branch's pre-rewrite commits is repaired from the boundary found by patch comparison.
- A dependent holding a merge commit is refused before any write.
- A dependent whose candidate boundary would drop one of its own commits (its own commit matches a landed one) is refused by the content check, before any write.
- A rebase conflict leaves the dependent's branch and worktree exactly as they were and exits non-zero.
- Every refusal that can be decided by reading happens before the retarget.
- `--push` succeeds when the remote branch is still at the recorded head, and fails without changing the remote when it has moved.
- A temporary worktree is gone after `--push`, and none is left behind by a refusal.
- A dependent that is not the forge user's is refused.
- `land`'s stacked reference names the command and no longer spells out the boundary search, the content check, the patch comparison, or the leased push.

## Seam

Proposed, awaiting the human's confirmation: the landing package's tests over throwaway repositories with a bare origin and its fake forge settle every rule above; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Dependents deeper than direct ones. Resolving a conflict. Running the dependent's checks between the two steps, which stays with the agent. Deleting the landed branch.
