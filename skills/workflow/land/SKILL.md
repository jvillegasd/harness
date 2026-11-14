---
name: land
description: "Close the lifecycle of a merged pull request: report what the merge skipped, repair stacked dependents, reconcile the tracker, and retire the worktree. Merges an open one only when asked to. Invoke only by name: \"the PR merged, clean up\", \"land and merge it\"."
argument-hint: "[pull request numbers, or stack <tip>; none means the current branch's pull request; add merge to squash-merge the open ones]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Close the lifecycle of the pull requests the human names: clean up after a merged one, and squash-merge an open one only when this invocation asks for the merge.

Pull requests: $ARGUMENTS

## Why

Merging publishes to the shared default branch and is hard to undo, so the human merges. This skill merges only when the human's own `/land` invocation says so (`/land merge`, "land and merge it"), and that request is the whole approval for the pull requests it names. Cleanup phrasing and a ready branch are not that request. Reached any other way, stop and tell the human to invoke `/land`. Scope the human left vague, such as "all prs", stops and asks for the numbers.

`harness land check` owns the rules for whether and how a pull request lands; `harness land check --help` explains each field. This skill acts on its verdict and changes no application code.

## Steps

1. **Check.** Run `harness land check` with the numbers given, `--stack <tip>` for `stack <tip>`, and `--merge` only when the invocation asked for the merge. It prints one verdict per pull request in landing order. Take them in that order through steps 2 to 6; the first stop ends the run with every later pull request untouched. A verdict with `after` is stale by its turn: once the one before it has landed and step 4 repaired it, wait for its checks with `gh pr checks <n> --required --watch` in the background, then check it again alone.

2. **Read the verdict.**
   - `lane` `stop`: report its `holds` and stop with everything kept.
   - `lane` `open`: an empty `holds` goes to step 3. Otherwise report each hold as a finding and stop. Two kinds can be cleared. A hold that says only test or documentation files changed after the review clears when you read those hunks and they change no shipped behavior. A review hold or a ledger finding clears when the human, in this session, accepts that one by name; `findings` carries each finding's evidence to show them. A forge gate never clears: a red or pending required check is not merged past.
   - `lane` `merged`: nothing can hold a merge that happened. Lead the report with `holds` as what the merge skipped, then continue at step 4.

   A review the human asked for is required whatever `review.required` says: treat any `review.status` but `current` as a hold.

3. **Merge.** `gh pr merge <n> --squash --match-head-commit <head>`, so a push after the check fails the merge instead of landing unchecked code. The subject is the pull request title. The body carries the resulting behavior, the `## Tradeoffs` section, and material assumptions from the description, so the reasons reach the default branch. Leave out `--delete-branch`: its local checkout fails when another worktree holds the default branch and reads as a failed merge. Then read `gh pr view <n> --json state,mergeCommit`; anything but `MERGED` stops the run, whatever the merge command exited with.

4. **Repair dependents** before deleting anything. Squashing replaces the head's commits, so each of `dependents` still carries them. Repair each one with `mine` true as [`references/stacked.md`](references/stacked.md) describes. Leave every other one as it is and report it with what its author needs to do: retarget to `<default>` and rebase off `<head>`.

5. **Update the default branch.** `git fetch --prune origin`, then fast-forward the local `<default>`: `git -C <path> merge --ff-only origin/<default>` in the worktree that holds it, or `git fetch origin <default>:<default>` when none does. A `<default>` that cannot fast-forward is reported and left. For ticket-bound work, a pull request with a closing reference or linked ticket, reconcile the tracker as [`references/tracker.md`](references/tracker.md) describes.

6. **Clean up**, in this order, because the last command can remove the directory this session runs in. In a run of several, leave that last command until every pull request has landed.
   - Review directory: `harness review remove --branch <key>`. When it refuses over a recorded review worktree, report that worktree and leave the directory.
   - Remote branch, only when `mine` is true, `mode` is not `fork`, and step 4 repaired every dependent: `git push origin --delete <headRefName>`. Someone else's branch stays, and so does one a dependent still sits on; report either.
   - `mode` `local`: `harness worktree retire <this checkout> --into origin/<default>` removes the worktree and its branch. It refuses, changing nothing, when that would lose work. Report a refusal and leave the checkout, except for the three it names a way past: pass `--allow-ignored` when every ignored path it lists is regenerable build output, never local configuration or a secret; pass `--keep-branch` when the branch is not proven landed, and report the branch; and when this is the main worktree, `git switch <default>` and retire the branch as in `remote` mode. A `<default>` held by another worktree keeps the task branch here; report it.
   - `mode` `remote`: `harness worktree retire --branch <headRefName> --into origin/<default>`, which deletes the local branch only when no worktree holds it. The checkouts in `worktrees` stay; name them.
   - `mode` `fork`: the review directory is all there is.

7. **Report** the run in order and, when it stopped early, where and why. For each pull request: its mode, whether this run merged it and the merge commit, the review status and source, each hold the human accepted or the merge skipped, the branches and review directory removed or kept, the worktree retired or left, and each dependent repaired or left for its author. For ticket-bound work add the ticket states and labels moved; otherwise say the tracker was skipped.
