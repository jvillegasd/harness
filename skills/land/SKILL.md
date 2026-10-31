---
name: land
description: "Land a verified pull request: squash merge it when it is still open, then reconcile the tracker and retire the worktree. Invoke only by name, never by routing: \"merge it\", \"the PR merged, clean up\"."
argument-hint: "[pull request number or ticket id; omit to detect. Invoking authorizes squash merge if open.]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Close the lifecycle for work that is ready to land: squash merge the pull request when it is still open, then reconcile the tracker and retire the task worktree.

Pull request or ticket: $ARGUMENTS

## Why

Merging publishes to the shared default branch and is hard to undo, so only a human's explicit `/land` authorizes it. That invocation is the whole approval for squash-merging the resolved open pull request and closing its lifecycle; no argument or second confirmation is added. If this skill is reached any other way, stop and tell the human to invoke `/land <n>`. Cleanup phrasing and a ready branch are not approval.

The pull request's state selects the lane (step 2). Mergeability and conformance are checked before merging; after someone else's merge, conformance can only report a mismatch and stop cleanup.

This capability merges, reconciles an authorized tracker binding, and retires a clean retained task worktree. It needs no prior delivery workflow and changes no application code.

## Steps

1. Resolve and name the pull request before proceeding. Resolve a number directly or a ticket id through its closing reference. Without input, use the current worktree's head-branch pull request; this is the only open-request autodetection. If none exists, use the repository's most recently merged request whose head still has a worktree. Do not search for another open request. Land only the PR this step names; someone else's PR needs the human to have named it. Read closing references or linked tracker metadata; a ticket is optional.

2. Pick the lane from the state. Run `gh pr view <n> --json state,isDraft,mergedAt,headRefName`.

   `MERGED` runs the merged lane: skip to step 7. There is nothing left to merge, and steps 9 and 10 condition the remote-branch deletion and the dependents report on this run having done the merging.

   `OPEN`, not a draft, runs the open lane: steps 3 through 6, then step 7.

   A draft stops and is reported as a finding, not an error. Any state outside the lanes above also stops as a finding. Check `isDraft` because drafts also have state `OPEN`. Closed unmerged requests belong to `/abandon`.

3. Gate before merging. Run `gh pr view <n> --json state,mergeable,mergeStateStatus,statusCheckRollup,reviewDecision`. Stop and report, as a finding rather than an error, on any of: a non-`MERGEABLE` mergeable state, a `DRAFT` merge state, a required check that is failing, pending, or unavailable, a missing required forge approval, or an unresolved review thread. `DRAFT` is step 2's stop reached a second way, and cheap to keep: `mergeStateStatus` knows it here without a second field. A branch behind its base needs an update only when the forge requires it. Shared history stays as it is: no rebase or force-push without authority. A red required check is never merged past.

   List and name direct dependents with `gh pr list --state open --base <headRefName> --json number,headRefName,baseRefName,headRefOid`, and record each dependent's `headRefOid` as its lease. They do not block merging but require step 6. Record the head's current tip SHA now; dependent rebases need this pre-squash boundary.

4. Verify the result against the available request, PR description, and linked ticket when present. An existing inline `## Contract` may supply requirements. Inspect relevant changes and verification evidence at the current head. A review the human requested, or that a named risk requires, must be verified before merging unless the human explicitly accepted reduced assurance; routine work needs none. Ask only about consequential missing requirements. A blocking mismatch stops before merging, or stops cleanup when the merge already happened. Fixes remain maintenance work on the existing branch while open; do not restart a lifecycle.

5. Squash merge, then confirm it landed. `gh pr merge <n> --squash`. The subject is the pull request title, which is already Conventional; the body carries the resulting behavior and material assumptions from the pull request description rather than a list of the squashed commits. Omit `--delete-branch`: its post-merge local checkout fails on a repository whose default branch is checked out in another worktree, which reports an error after the merge has already succeeded and reads as a failed merge. Step 9 deletes the remote branch instead. Then re-read `gh pr view <n> --json state,mergedAt,mergeCommit`. Anything but `MERGED` stops here and is reported, including when the merge command exited non-zero for the reason above; check the state, do not assume either outcome.

6. Repair direct dependents from step 3 before deleting anything. Squash merging replaces the original commits, so dependents otherwise repeat absorbed work and risk unintended retargeting or closure.

   Fetch first and rebase onto `origin/<default>`; the local default branch lacks the squash until step 7.

   For each direct dependent: retarget it with `gh pr edit <n> --base <default branch>` while the old base still exists, so a failure surfaces here rather than after the deletion has already made it unrecoverable. Then drop the absorbed commits from its head with `git rebase --onto origin/<default> <the tip SHA recorded in step 3> <dependent head>`, run that branch's own local rungs, and push with `git push --force-with-lease=refs/heads/<dependent head>:<its headRefOid from step 3> origin <dependent head>`. The explicit lease fails if the dependent's owner pushed since step 3, even when a background fetch has moved the remote-tracking ref; report that dependent instead of retrying. Do the rebase in the dependent's own worktree where it has one, and never rebase a branch that is checked out somewhere else.

   Repair only direct dependents. A deeper chain re-forms correctly as each one lands, so rebasing it now would be guessing at a base that is about to change again. A conflict that is more than mechanical stops here: leave the head branch undeleted, report which dependents were repaired and which were not, and treat the rest as `/address-review` work. Nothing below runs until every dependent is repaired or reported.

7. Verify the merge and default-branch health. Fetch and prune, locate `<default>`'s worktree with `git worktree list`, then run `git -C <that path> merge --ff-only origin/<default>`. Run the repository-required local checks on the updated default branch and report failures before cleanup. Pass `-C` so the right branch is updated, and use it in place of `git checkout <default>`, which fails when another worktree owns the branch.

   When recovery material exists, read `continuity` and preserve it before retirement. Record the confirmed merge and remaining cleanup in the retained copy of the task document outside a checkout slated for retirement; mirror if available. Keep the merged checkout clean of this bookkeeping, create no checkpoint merely to land, and leave legacy records as they are.

   Reaching this step straight from step 2, run step 4's conformance check here and resume at step 8 rather than step 5, because steps 5 and 6 belong to the run that does the merging. Someone else merged this pull request, so a blocking mismatch is reported rather than prevented, and the landing stops here.

8. Reconcile the tracker only for ticket-bound work. Where `setup-harness` installed the GitHub lifecycle workflow, confirm it already ran: the ticket closed, the parent closed once every child had merge evidence, and each unblocked sibling moved to mapped `ready_for_agent`. Where it did not run, or the tracker is not GitHub, apply those transitions by hand and say which ones you applied. Where the tracker is local markdown, read the `local-tracker` skill: the transitions are file edits that must be committed and pushed on the default branch, which step 7 has already fast-forwarded past the merge, and before step 9 retires the worktree. The explicit landing request covers these scoped reconciliation writes; name and verify their files and transitions without a repeated confirmation. Preserve unrelated default-branch work. A protected branch or transition outside that authority stops only dependent cleanup. A parent closes only when every child has merge evidence. For trackerless work, skip tracker transitions.

9. Retire only a clean worktree. Remove the authorized checkout and the merged local branch (POLICY's floor row on worktrees; only `land` and `abandon` retire a task worktree). Delete the remote head branch as well when this run did the merging, and only once step 6 reported every dependent repaired; an unrepaired dependent keeps it alive. If the worktree holds uncommitted changes or unpushed commits, refuse and report instead: this skill discards nothing, and `/abandon` is what discards unmerged work.

10. Report the pull request, whether this run merged it and the merge commit when it did, the requirements source, the remote branch deleted, and the worktree removed. Where this run merged, report every dependent pull request that was retargeted and rebased, and every one that was left for a human. The merged lane skips steps 3 and 6, so it reports that dependents were neither enumerated nor repaired, which is not the same as reporting none. For ticket-bound work, also report the ticket and parent states, labels moved, and next ready frontier. For trackerless work, report that tracker reconciliation was skipped.

## Skills this loads

- `continuity`, read from its path only when recovery material exists
- `local-tracker`, read from its path, only when ticket-bound and `docs/agents/harness.json` sets `"tracker": "local-markdown"`
