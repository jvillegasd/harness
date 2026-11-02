---
name: abandon
description: "Release an abandoned attempt or reconcile explicitly cancelled scope, then retire its worktree. Invoke only by name: \"abandon this\", \"this spike is dead\", \"the PR was closed without merging\", \"release the ticket\"."
argument-hint: "[ticket id, branch, or pull request number; omit to detect]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Close the lifecycle for work that will not land: release stranded claims, reconcile explicitly cancelled scope, and retire the worktree.

Target: $ARGUMENTS

## Why

Discarded work cannot be recovered, so this is the only workflow that discards unmerged work. It names everything it will discard, gets one confirmation of that inventory from the human, and deletes nothing beyond it. A prototype uses it only after an explicit discard request. Open or merged pull requests are outside this workflow, and it changes no application code.

## Steps

1. Resolve the target. A pull request number resolves directly; a ticket id resolves to its branch, and to that branch's pull request when one exists; a branch name resolves directly. With no input, find the stranded work and name what you picked: a ticket at mapped `in_progress` whose branch has no open pull request, or a retained worktree whose branch has a closed unmerged pull request. Where the tracker is local markdown, read the `local-tracker` skill: it says how an id resolves to a ticket file and how a pull request resolves back to one. The ticket is optional, so a branch that never had one is a valid target. For a bound ticket, read its description and complete comment thread in order, then its parent's description and complete comment thread, and the live parent and dependency relationships. On a partial rerun, recover previously affected parents and dependents from the recorded cancellation history as well as live relationships. Distinguish abandoning an implementation attempt from cancelling the requirement: a closed unmerged pull request or a dead run alone means release for retry. Cancellation needs explicit human intent that the requirement is no longer needed. The request or the ticket thread may propose it; the live confirmation in step 3, which lists its exact scope changes, is what authorizes it. If intent is unclear, preserve the requirement and relationships, and leave a closed ticket and any recorded cancellation as they are.

2. Gate before writes. Stop and report an open pull request as a finding, not an error; its claim remains live. For a merged request, stop and tell the human to invoke `/land <n>`; merging and cleanup after a merge belong to that human invocation. Continue only for closed unmerged requests or no request.

3. Name the complete write set before any write. Report `git log origin/<default>..<branch> --oneline` and `git status --short` from the branch's worktree, listing commits and uncommitted changes rather than counting them. Record the local and remote branch tip SHAs alongside the discard inventory. If the worktree is gone but a local or remote branch remains, inspect and list its commits before proposing deletion. If either is absent, report that instead of recreating it. List the branch's review directory too when it exists (the directory of `harness review path --branch <branch>`), since it holds the branch's unmerged review findings and patches. Tracker reconciliation still runs when no worktree remains.

   For cancellation, inspect the parent's remaining children and each dependent's full contract and comments. List the cancellation reason, ticket state and label changes, parent scope edits, exact relationships to remove, affected dependent states, and whether the remaining parent scope has merge evidence. Remove a dependency only when its prerequisite is explicitly obsolete; a dependent that still needs the cancelled behavior stays blocked and is reported for a replacement contract, with its acceptance criteria left as written. If a retained requirement still depends on the cancelled behavior, stop before writes and request a scope or replacement decision in the same batch; detaching the cancelled prerequisite would only make the parent appear complete. If the tracker has no documented cancellation representation, stop before writes; `local-tracker` currently defines release and `done`, not cancellation, so report the gap instead of inventing a status or archive layout there.

   Confirm the combined discard and tracker write set once, including unresolved scope decisions. Nothing is written until the open decisions are resolved. Before applying, re-read affected contracts, relationships, states, and the local discard list. Changes require a revised confirmation; approval covers only the listed set.

4. Reconcile only ticket-bound work. For an abandoned attempt still needed, release without changing scope or relationships. Reopen a closed ticket only with explicit approval. Set an open ticket to mapped `blocked` or mapped `ready_for_agent` according to its current edges, including reopened blockers. For local markdown, follow `local-tracker`'s release commit. On GitHub, remove both active labels (`in_progress` and `ready_for_human`) before applying the release label, and report all three actions. Active and release labels must not coexist, and release emits no close/reopen event for the reconciler. Skip for trackerless work.

   For explicitly cancelled GitHub requirements, do not apply a retry label. Record the approved cancellation and exclusion from required scope on the ticket and parent, retaining historical references. Apply the approved parent scope edit, remove only the approved obsolete blocking edges, and detach the cancelled ticket's native sub-issue relationship so it is no longer a required child. Remove all mapped lifecycle labels from the cancelled ticket and close it as `not planned`, or retain that reason when it is already closed. Preserve unrelated labels, comments, and the ticket itself; cancellation is not merge evidence.

   Read every write back. Recompute affected dependents from their remaining blockers using the existing merge-evidence and active-claim rules; do not promote planning-only tickets or overwrite another run's active claim. Verify each remaining required child's merged pull request before closing the parent. A missing merge, retained dependency on cancelled behavior, or unresolved scope decision keeps the parent open and is reported. If no required children remain, report the parent as entirely cancelled, not delivered; close it as `not planned` only when that parent cancellation was in the approved write set, removing its mapped lifecycle labels too. Close an ancestor only after its own requirements were checked, and keep the merge-evidence rule intact.

   Reconcile explicitly; relationship edits and already-closed tickets may emit no lifecycle event. Verify final state after any triggered lifecycle run. On partial or failed writes, stop before retirement, report persisted and remaining changes, and resume from read-back state without duplicate history or writes.

5. Retire the local worktree and branches only after tracker reconciliation completes, or is skipped for trackerless work. Immediately before each deletion, re-read the surviving local and remote branch tips and their unpushed commits, and `git status --short` where a worktree remains. Compare them with step 3's approved inventory, whether or not the worktree still exists. A changed tip or discard list stops for a revised confirmation; worktree absence never skips this check. Once the re-read confirms the approved inventory, preserve any existing recovery document and retained results outside the checkout before deletion, following `continuity`. Record abandonment or cancellation only in existing task material; mirror if available and explicitly report a pending mirror. Preserve legacy checkpoints unchanged. Create no task artifact solely to abandon.

   Remove only the approved worktree (POLICY's floor row on worktrees), with `git worktree remove --force <path>` for the discard this command exists for, and wait for removal before reporting it. Where the worktree is already gone, verify with `git worktree list` that the branch is not checked out elsewhere first. Either way, delete the local branch with the leased `git update-ref -d refs/heads/<branch> <approved-local-tip>` rather than `git branch -D`, so a tip that moved since step 3's approval fails the lease instead of being discarded. Report an absent branch or worktree instead of recreating it. Delete the approved review directory with `rm -rf -- <review directory>`.

   Delete the remote branch only when one was pushed, its pull request is still closed unmerged, and its tip still matches the approved remote inventory. Use an explicit expected-tip lease: `git push --force-with-lease=refs/heads/<branch>:<approved-remote-tip> origin :refs/heads/<branch>`. A lease failure stops for reinspection, and the deletion is retried only with the guard. A remote branch with no pull request stays, since someone else may be tracking it.

6. Report the target and disposition (release for retry or explicit cancellation), discarded work, ticket state, parent scope and state, relationships removed or retained, newly ready dependents and remaining blockers, and the worktree removed or already absent. For trackerless work, report that tracker reconciliation was skipped.

## Skills this loads

- `continuity`, read from its path only when recovery material exists
- `local-tracker`, read from its path, only when ticket-bound and `docs/agents/harness.json` sets `"tracker": "local-markdown"`
