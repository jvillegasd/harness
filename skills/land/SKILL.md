---
name: land
description: "Land a verified pull request: squash merge it when it is still open, then reconcile the tracker and retire the worktree. Invoke only by name, never by routing: \"merge it\", \"the PR merged, clean up\"."
argument-hint: "[optional pull request number or ticket id, which must resolve to the current branch. Invoking authorizes squash merge if open.]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Close the lifecycle for work that is ready to land: squash merge the pull request when it is still open, then reconcile the tracker and retire the task worktree.

Pull request or ticket: $ARGUMENTS

## Why

Merging publishes to the shared default branch and is hard to undo, so only a human's explicit `/land` authorizes it. That invocation is the whole approval for squash-merging the resolved open pull request and closing its lifecycle; no argument or second confirmation is added. If this skill is reached any other way, stop and tell the human to invoke `/land` from the worktree to land. Cleanup phrasing and a ready branch are not approval.

The pull request's state selects the lane (step 2). Mergeability and conformance are checked before merging; after someone else's merge, conformance can only report a mismatch and stop cleanup.

This capability merges, reconciles an authorized tracker binding, and retires a clean retained task worktree. It needs no prior delivery workflow and changes no application code. It acts only on the branch checked out where it runs, so a stack lands one `/land` per worktree, bottom up.

## Steps

1. Resolve the pull request from the current worktree. Land only the pull request whose head is the branch checked out here (`git branch --show-current`, then `gh pr view --json number,headRefName,baseRefName`). A number or ticket id in the arguments is a cross-check: resolve it, a ticket through its closing reference, and when it names a different branch, stop and name the worktree holding that branch from `git worktree list --porcelain`, or say that none does, so the human runs `/land` there. A detached HEAD, the default branch, or a branch with no pull request stops as a finding. Read closing references or linked tracker metadata; a ticket is optional.

2. Pick the lane from the state. Run `gh pr view <n> --json state,isDraft,mergedAt,headRefName`.

   `MERGED` runs the merged lane: skip to step 7. There is nothing left to merge, and steps 9 and 10 condition the remote-branch deletion and the dependents report on this run having done the merging.

   `OPEN`, not a draft, runs the open lane: steps 3 through 6, then step 7.

   A draft stops and is reported as a finding, not an error. Any state outside the lanes above also stops as a finding. Check `isDraft` because drafts also have state `OPEN`. Closed unmerged requests belong to `/abandon`.

3. Repair this branch, then gate before merging.

   Repair first when the branch still carries work an earlier `/land` already squash-merged, which happens when it was stacked on a branch that landed and was retargeted to the default branch by that run's step 6. Fetch, then walk `git rev-list --reverse origin/<default>..HEAD` from the oldest commit and keep the longest leading run of commits whose content is already on the default branch: a commit qualifies when `git merge-tree --write-tree origin/<default> <commit>` prints `git rev-parse origin/<default>^{tree}` as its first line. The last commit of that run is the boundary; with none, the branch needs no repair. A merge commit in that range (`git rev-list --merges origin/<default>..HEAD` prints anything) is reported, not rebased, because it can hide absorbed commits under a clean-looking tip. Otherwise record the old head, run `git rebase --onto origin/<default> <boundary>` here, and confirm this branch's own change survived: `git diff <boundary> <old head> | git patch-id --verbatim` must be non-empty and equal `git diff origin/<default> HEAD | git patch-id --verbatim`. An empty id or a difference stops and is reported, since two empty ids compare equal and a difference means the boundary still held this branch's own work. Run the repository's local rungs, push with `git push --force-with-lease=refs/heads/<branch>:<old head> origin <branch>`, which fails if someone else pushed meanwhile, and wait for the required checks on the new head with `gh pr checks <n> --required --watch` run in the background before the gate. Keep the boundary for step 4.

   Gate. Run `gh pr view <n> --json state,mergeable,mergeStateStatus,statusCheckRollup,reviewDecision`. Stop and report, as a finding rather than an error, on any of: a non-`MERGEABLE` mergeable state, a `DRAFT` merge state, a required check that is failing, pending, or unavailable, a missing required forge approval, or an unresolved review thread. `DRAFT` is step 2's stop reached a second way, and cheap to keep: `mergeStateStatus` knows it here without a second field. A branch behind its base needs an update only when the forge requires it. Shared history stays as it is: no rebase or force-push without authority. A red required check is never merged past.

   List and name direct dependents with `gh pr list --state open --base <headRefName> --json number,headRefName`. They do not block merging but need step 6. Record the head's current tip SHA now for step 5.

4. Verify the result against the available request, PR description, and linked ticket when present. An existing inline `## Contract` may supply requirements. Inspect relevant changes and verification evidence at the current head. A review the human requested, or that a named risk requires, must be verified before merging unless the human explicitly accepted reduced assurance; routine work needs none.

   In the open lane, check that a required review still covers the head being merged. When the review was required (the human asked for it, or the PR body's change map lists `critical` items) and the PR body has no `Reviewed: <base>..<sha>` line (written by `open-pr`), treat the review as unverified. Otherwise fetch, then confirm both `git rev-parse --verify origin/<baseRefName>^{commit}` and `git rev-parse --verify <sha>^{commit}` resolve; if either does not, the review is stale, because a base or reviewed commit that is gone cannot be compared. When step 3 repaired this branch and its boundary is an ancestor of `<sha>`, the reviewed commit still sits on the absorbed work, so use `git diff <boundary> <sha>` as the reviewed side in place of the three-dot diff from `<sha>` below. Diff both sides against the PR's current base, `origin/<baseRefName>`, rather than the base recorded in the line, so commits the base gained before a rebase never count as changes. Compare `git diff origin/<baseRefName>...<sha> | git patch-id --verbatim` with `git diff origin/<baseRefName>...<head> | git patch-id --verbatim`, where `<head>` is the tip SHA recorded in step 3; `--verbatim` keeps whitespace, which changes behavior in Python, YAML, or Makefiles. Equal, non-empty ids mean the head carries the reviewed patch and the review stands. When they differ, find the files whose own three-dot diffs differ (`git diff origin/<baseRefName>...<sha> -- <file>` against `git diff origin/<baseRefName>...<head> -- <file>` for each file either diff touches). The review still covers the head only when every such file is a test or documentation file whose changed hunks you have read and found to change no shipped behavior; a lockfile, a generated file, or any other file changed after the review makes it stale. A stale or unverified required review is a finding like a failing check: report it, and hold the merge unless the human, who invoked `/land`, accepts the reduced assurance in this session. A stale review that was never required is reported without holding the merge.

   Ask only about consequential missing requirements. A blocking mismatch stops before merging, or stops cleanup when the merge already happened. Fixes remain maintenance work on the existing branch while open; do not restart a lifecycle.

5. Squash merge, then confirm it landed. `gh pr merge <n> --squash --match-head-commit <head recorded in step 3>`, so a push after the checks above fails the merge instead of landing unchecked code. The subject is the pull request title, which is already Conventional; the body carries the resulting behavior and material assumptions from the pull request description rather than a list of the squashed commits. Omit `--delete-branch`: its post-merge local checkout fails on a repository whose default branch is checked out in another worktree, which reports an error after the merge has already succeeded and reads as a failed merge. Step 9 deletes the remote branch instead. Then re-read `gh pr view <n> --json state,mergedAt,mergeCommit`. Anything but `MERGED` stops here and is reported, including when the merge command exited non-zero for the reason above; check the state, do not assume either outcome.

6. Retarget direct dependents before deleting anything. For each dependent from step 3, run `gh pr edit <n> --base <default branch>` while the old base still exists, then read back its base. A failed retarget is reported and keeps the head branch alive in step 9. Rebase and push no other branch: each dependent still carries the commits this run squashed, and its own `/land`, run from its worktree, removes them in step 3.

7. Verify the merge and default-branch health. Fetch and prune, locate `<default>`'s worktree with `git worktree list`, then run `git -C <that path> merge --ff-only origin/<default>`. Run the repository-required local checks on the updated default branch and report failures before cleanup. Pass `-C` so the right branch is updated, and use it in place of `git checkout <default>`, which fails when another worktree owns the branch.

   When recovery material exists, read `continuity` and preserve it before retirement. Record the confirmed merge and remaining cleanup in the retained copy of the task document outside a checkout slated for retirement; mirror if available. Keep the merged checkout clean of this bookkeeping, create no checkpoint merely to land, and leave legacy records as they are.

   Reaching this step straight from step 2, run step 4's conformance check here and resume at step 8 rather than step 5, because steps 5 and 6 belong to the run that does the merging. Someone else merged this pull request, so a blocking mismatch is reported rather than prevented, and the landing stops here.

8. Reconcile the tracker only for ticket-bound work. Where `setup-harness` installed the GitHub lifecycle workflow, confirm it already ran: the ticket closed, the parent closed once every child had merge evidence, and each unblocked sibling moved to mapped `ready_for_agent`. Where it did not run, or the tracker is not GitHub, apply those transitions by hand and say which ones you applied. Where the tracker is local markdown, read the `local-tracker` skill: the transitions are file edits that must be committed and pushed on the default branch, which step 7 has already fast-forwarded past the merge, and before step 9 retires the worktree. The explicit landing request covers these scoped reconciliation writes; name and verify their files and transitions without a repeated confirmation. Preserve unrelated default-branch work. A protected branch or transition outside that authority stops only dependent cleanup. A parent closes only when every child has merge evidence. For trackerless work, skip tracker transitions.

9. Retire only this run's checkout, and only when clean. When it is a linked worktree, remove it and the merged local branch from the default branch's worktree (`git -C <that path> worktree remove <this path>`), since a checkout cannot remove itself; when it is the main checkout, switch it to the default branch and delete the merged local branch (POLICY's floor row on worktrees; only `land` and `abandon` retire a task worktree). Delete the remote head branch as well when this run did the merging, and only once step 6 retargeted every dependent; a dependent still based on it keeps it alive. If the checkout holds uncommitted changes or unpushed commits, refuse and report instead: this skill discards nothing, and `/abandon` is what discards unmerged work.

10. Report the pull request, whether this run merged it and the merge commit when it did, the requirements source, the review status (current, covered with later tests or docs, stale and accepted, or none required), the remote branch deleted, and the worktree removed. Report whether step 3 repaired this branch, with its boundary. Where this run merged, report every dependent pull request it retargeted, and that each one repairs itself when `/land` runs from its own worktree. The merged lane skips steps 3 and 6, so it reports that dependents were neither enumerated nor retargeted, which is not the same as reporting none. For ticket-bound work, also report the ticket and parent states, labels moved, and next ready frontier. For trackerless work, report that tracker reconciliation was skipped.

## Skills this loads

- `continuity`, read from its path only when recovery material exists
- `local-tracker`, read from its path, only when ticket-bound and `docs/agents/harness.json` sets `"tracker": "local-markdown"`
