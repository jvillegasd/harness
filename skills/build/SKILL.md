---
name: build
description: "Build a set of tickets or a spec's slices unattended on one integration branch, then verify, map, review, and optionally open a pull request. Invoke only by name: \"/build the spec\", \"/build tickets 12-18\"."
argument-hint: "[spec path, ticket ids, or tracker query; add \"and open a PR\" to authorize delivery]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Build a set of tickets or a spec's slices unattended on one integration branch: fan the unblocked slices out to executors, merge each finished slice locally, and finish with the branch verified, mapped, and reviewed, ready for a human to read and `/land`.

Spec, tickets, or query: $ARGUMENTS

## Why

This runs many workers without supervision and spends model budget at that scale, so only a human's explicit `/build` starts it. The invocation authorizes local work: the integration branch, worker worktrees, local merges, and local commits on branches this run creates. It does not authorize a push, a pull request, a tracker write, or a merge; a pull request needs the delivery request in the arguments or a later one, and merging stays with `/land`. If this skill is reached any other way, stop and tell the human to invoke `/build`.

Computing the frontier from merged work, rather than from tracker counts, keeps the run correct when a ticket is closed by hand or a label goes stale mid-run. Running one slice end to end before fanning out means a broken brief fails once instead of N times.

## Steps

1. Read the spec and the tickets in full, with their threads, and the agreed test seams (see `spec`). Stop and ask when the seams were never agreed, a slice has no acceptance criteria that fail on the base, or the blocking edges contain a cycle, because workers cannot repair those.
2. Create one integration branch for the run, named `<type>/<slug>` from the default branch, as an agent-created task branch. Record its base SHA, and keep a task document (`continuity`) with the slice list, status, and a `## Decisions` table when the run is long.
3. Compute the frontier: the tickets whose blockers are all integrated on the integration branch. Use the merged work on that branch as the source of truth, and read tracker state only to find the edges.
4. Run the first slice alone, end to end: brief it, build it, check it, integrate it. Fan out only after it succeeds, and revise the brief template if it failed.
5. For each frontier ticket, launch one `executor` in its own worktree branched from the integration branch's current head, natively or through `orchestration`, as POLICY's Delegation describes. Write each brief to stand alone: the goal, the scope it may touch, the exact base SHA, how to verify it, and what to report. Tell the executor to use `tdd` at the agreed seams. Executors never commit, push, or merge.
6. When an executor reports, check the result against its scope and the actual Git state in its worktree; a completion message is not evidence. A missing or unverifiable result is a gap, not a pass. Rerun the verification commands yourself.
7. Integrate each verified slice. The parent commits the slice on the worker's branch with a Conventional Commit, then merges it into the integration branch locally. Resolve a conflict by reading both sides; when it needs a product decision, record the gap and continue with independent slices. Recompute the frontier and continue from step 5 until no slice remains ready.
8. Apply POLICY's stop rule per slice. A slice that fails the same check after three distinct fixes stops and is reported with what was tried, the evidence, and the next hypothesis, and its dependents wait.
9. When every slice is integrated, run `verification` on the integration branch, then `change-map`, then `review` on its `critical` items. Send the review findings to one fresh `executor` as a single brief, integrate its fix, and rerun the affected checks.
10. End with the integration branch ready. When the delivery request was given, open the pull request with `open-pr`; otherwise stop there and offer it once. Do not push, merge, or write to the tracker on your own, and leave worker worktrees in place for the human unless they are clean and merged, since removal is `land`'s and `abandon`'s job.

## Output

- Slices done, each with its commit, and the slices that remain.
- Gaps: missing or unverifiable results, stopped slices with their evidence, and decisions that wait on the human.
- Checks, each with its command and result against the final head.
- The change map for the integration branch.
- The review outcome, or that review was unavailable.
- The pull request link when delivery was requested.
