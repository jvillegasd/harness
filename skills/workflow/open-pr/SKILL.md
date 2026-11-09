---
name: open-pr
description: "Trigger: open a PR, publish a pull request, or ship this. Complete explicitly authorized delivery on the existing task branch."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Deliver finished work as a pull request: commits, push, the PR itself, and required-check repairs, on the existing task branch.

## Why

An explicit request to open or ship a PR covers these steps together (see POLICY's Delivery section), so they run without a second approval. Implementation or plan approval alone does not invoke delivery. Merging is outside this request: the human merges, or asks `/land` to. This skill ends with a readiness report, the facts the human reads before merging.

## Steps

1. Read the request and its existing authorization. Inspect Git status, the staged diff, the branch, worktrees, and any existing PR, and reuse the branch and PR for this work. Create a task branch if needed. Read linked tickets and comments when relevant; a ticket is optional.
2. Finish the local changes, read the diff once through `ponytail` and cut what it would not keep, map the change with `change-map`, and run `verification`, including its run-as-user verdict when the change map calls for one. Run `review` when its Level section calls for one, or at the depth the human asked for. A review that runs is required. Confirmed findings are fixed before the pull request opens, and each fix is recorded `fixed` with its commit, as `review`'s `references/ledger.md` shows. If a required review is unavailable, report it as unverified and hold the dependent delivery unless the human explicitly accepts the reduced assurance.
3. Prepare a concise PR description from [the PR body template](references/pr-body.md), which defers to the repository's own template: the problem, then the `change-map` output with every changed file and its tier, the fact each risky item rests on, merge danger, and decisions the human did not ask for, the evidence, and material unresolved risks other than review findings, which stay in `review`'s ledger. Preserve issue links and human-written context. Add a closing reference only for scope actually completed.
4. Commit only owned changes, since unrelated staged work belongs to someone else. Read the staged content and stage explicit paths. When unrelated changes are already staged, commit through an isolated temporary index or an exact-path commit, and only after confirming each included path holds solely owned changes, so the original index entries survive. If shared paths cannot be separated, hold that commit and ask.
   When a task document exists (`continuity`), commit its final update with the work: status shipped for review, the final evidence, and the open decisions.
   When a review applied, commit first, then add one line `Reviewed: <base>..<sha>` to the PR body, where `<sha>` is the commit that contains exactly the reviewed content (after repairs, the repaired head, rechecked at High; when any review slice stayed unreviewed, write no line and report the gap), so `land` can tell whether later commits changed what was reviewed. When the level kept review from running, write `Review: not run (level <level>)` instead. That line is the only review content in the PR body; findings and their dispositions stay in `review`'s local ledger.
5. Push the task branch and open or refresh its PR with the prepared title and body. Read back the branch, base, head revision, and metadata. Drive the required checks green with scoped repairs and affected rechecks. Wait for them with `gh pr checks <n> --required --watch` run in the background, which reports when they settle, rather than polling. Then check mergeability with `gh pr view <n> --json mergeable,mergeStateStatus`: resolve a mechanical conflict on the task branch, and ask before resolving a semantic one or rewriting a shared branch.
6. Apply tracker transitions only when that write set is already authorized; read `local-tracker` from its path for file-backed trackers.
7. Never merge. End with the readiness report below.

## Readiness report

The facts the human reads before merging, each taken at the PR's current head:

- **Checks:** each required check's state, and mergeability.
- **Review:** whether the `Reviewed:` line covers the head, or why no review ran.
- **Verdict:** `verification`'s verdict line, or why none ran.
- **Ledger:** open findings from `harness review list --head <head>`, and the dropped count.
- **Limits:** material remaining limitations.

Lead with `Ready to merge` only when every required check passed, the PR is mergeable, any required review covers the head, the verdict is not FAIL, and no finding is open; otherwise lead with `Not ready` and the first blocker.

## Output

The PR link and the readiness report.
