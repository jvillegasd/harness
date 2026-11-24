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

The floor delivers finished work on a task branch without asking, so these steps run together whenever work is done, unless the human said to keep it local. Merging is outside delivery: the human merges, or asks `/land` to. This skill ends with a readiness report, the facts the human reads before merging.

## Steps

1. Read the request and its existing authorization. Inspect Git status, the staged diff, the branch, worktrees, and any existing PR, and reuse the branch and PR for this work. Create a task branch if needed. Read linked tickets and comments when relevant; a ticket is optional.
2. Finish the local changes, read the diff once and cut what `principle-laziness-protocol` would not keep, then run `no-comments`, read from its path, on the diff against the base, limited to owned paths, with its encodings approved in advance; what it leaves open goes in the PR's risks. Decide whether the work ships as one pull request or a stack, as [stacks](references/stacks.md) describes; each pull request in a stack runs the rest of these steps on its own. Map the change with `change-map`, and run `verify-work`, including its run-as-user verdict when the change map calls for one.
3. Prepare a concise PR description from [the PR body template](references/pr-body.md), which defers to the repository's own template: the problem, then the `change-map` output with every changed file and its tier, the fact each risky item rests on, merge danger, and the tradeoffs, the evidence, and material unresolved risks other than review findings, which go in the review's verdict. Preserve issue links and human-written context. Add a closing reference only for scope actually completed. Read `unslop` from its path and apply it to the title and body.
4. Commit only owned changes, since unrelated staged work belongs to someone else. Read the staged content and stage explicit paths. When unrelated changes are already staged, confirm each owned path holds solely owned changes, then run `git add --intent-to-add <new owned files>` and `git commit --only -m <message> -- <owned paths>`: it commits those paths' working-tree content and leaves every other index entry as it was. If shared paths cannot be separated, hold that commit and ask.
   When a task document exists (`checkpoint`), carry its final evidence and open decisions into the description. The document itself stays local and is never committed.
   The PR body carries no review content: the review's result is its verdict comment.
5. Push the task branch and open or refresh its PR with the prepared title and body, ready for review, never as a draft. Read back the branch, base, head revision, and metadata. Then run `review` on the pull request when its Level section calls for one, or at the depth the human asked for. A review that runs is required: it fixes confirmed findings on the task branch, pushes them, and posts its verdict on the pull request at the repaired head, as `review`'s Verdict section shows. When the level kept review from running, post nothing. Then post `verify-work`'s run-as-user verdict as its Verdict section describes; when review's repairs changed code after the runner drove it, get a fresh verdict at the repaired head first. If a required review is unavailable, report it as unverified in the readiness report unless the human explicitly accepts the reduced assurance. For a stack, take one status pass per PR instead of driving each one, as [stacks](references/stacks.md) describes. Otherwise drive the required checks green with scoped repairs and affected rechecks. Wait with `harness pr status --watch <n>` run in the background rather than polling: it returns once the verdict no longer waits on pending checks, and its exit code names the first blocker. On a conflict, resolve a mechanical one on the task branch, and ask before resolving a semantic one or rewriting a shared branch.
6. Apply tracker transitions only when that write set is already authorized.
7. Never merge. End with the readiness report below.

## Readiness report

The facts the human reads before merging, taken at the PR's current head. `harness pr status <n>` reports the forge gates, the review and run-as-user verdicts and whether each covers the head, and the unresolved threads, and its exit code names the first blocker. Report its verdict, and add the two facts it does not hold:

- **Verdict:** `verify-work`'s verdict line, or why none ran.
- **Limits:** material remaining limitations.

Lead with `Ready to merge` only when `harness pr status` exits 0 and the verdict is not FAIL; otherwise lead with `Not ready` and the first blocker it names.

## Output

Each PR link with its readiness report.
