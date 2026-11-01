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

An explicit request to open or ship a PR covers these steps together (see POLICY's Delivery section), so they run without a second approval. Implementation or plan approval alone does not invoke delivery. Merging is outside this request and stays with the human's `/land`.

## Steps

1. Read the request and its existing authorization. Inspect Git status, the staged diff, the branch, worktrees, and any existing PR, and reuse the branch and PR for this work. Create a task branch if needed. Read linked tickets and comments when relevant; a ticket is optional.
2. Finish the local changes, run `verification`, and map the change with `change-map`. Use `review` when the human asked for it, when the map has `critical` items, or when a concrete risk ordinary checks cannot settle remains. If a required review is unavailable, report it as unverified and hold the dependent delivery unless the human explicitly accepts the reduced assurance.
3. Prepare a concise PR description in the project's own style: the problem, then the `change-map` output (tiers, what to read, the fact each risky item rests on, merge danger, and decisions the human did not ask for), the evidence, and material unresolved risks. Preserve issue links and human-written context. Add a closing reference only for scope actually completed.
4. Commit only owned changes, since unrelated staged work belongs to someone else. Read the staged content and stage explicit paths. When unrelated changes are already staged, commit through an isolated temporary index or an exact-path commit, and only after confirming each included path holds solely owned changes, so the original index entries survive. If shared paths cannot be separated, hold that commit and ask.
5. Push the task branch and open or refresh its PR with the prepared title and body. Read back the branch, base, head revision, and metadata. Drive the required checks green with scoped repairs and affected rechecks.
6. Apply tracker transitions only when that write set is already authorized; read `local-tracker` for file-backed trackers. Update `continuity` only if this work already needs it.

## Output

The PR link, check results, the review outcome when one applied, and material remaining limitations.
