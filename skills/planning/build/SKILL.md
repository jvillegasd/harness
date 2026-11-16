---
name: build
description: "Build one ticket or request inline with TDD, or a settled epic's slices unattended on one integration branch, then verify, map, review, and optionally open a pull request. Invoke only by name: \"/build the export epic\", \"/build ticket 12\"."
argument-hint: "[a request, ticket, epic path, slice numbers, or tracker query; add \"and open a PR\" to authorize delivery]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Build what was asked, sized by the input. **One slice** (a plain request, one ticket, or an epic that has only one slice) is built inline in this session with `tdd`. **Several slices** (an epic or a set of tickets with more than one slice, however many are ready at first) are built unattended on one integration branch: fan the unblocked slices out to executors, merge each finished slice locally, and finish with the branch verified, mapped, and reviewed, ready for a human to read and merge.

Input: $ARGUMENTS

## Why

The several-slice path runs many workers without supervision and spends model budget at that scale, so only a human starts it: by typing `/build`, through `/jvillegasd-mode` on a settled epic, by answering go when an epic asks whether to build it, or through an autopilot grant, which counts as both the invocation and the delivery request. The invocation authorizes local work: the integration branch, worker worktrees, local merges of slice branches into the integration branch, and local commits on branches this run creates. It does not authorize a push, a pull request, a tracker write, or a merge into the default branch or of a pull request; a pull request needs the delivery request in the arguments or a later one from the human, and merging stays with the human. Under an autopilot grant, its rules (decision defaults, stuck rule, and ceiling) replace this skill's asks and stops.

Computing the frontier from merged work, rather than from tracker counts, keeps the run correct when a ticket is closed by hand or a label goes stale mid-run. Running one slice end to end before fanning out means a broken brief fails once instead of N times.

## One slice

1. Read the request and the relevant code; for a ticket, read its body and full comment thread. State the intended behavior and smallest change. Resolve routine details from repository conventions, and ask only about consequential gaps in scope or acceptance criteria.
2. Inspect Git state and get on the task branch as the floor's Working rules describe, in a sibling worktree when separation helps. Make the smallest complete change (`principle-laziness-protocol`). Keep a task document only when recovery across an interruption warrants one.
3. Read and use `tdd`. Reuse the seams the epic or ticket agreed; otherwise propose the relevant public boundary and settle it with the human before writing tests. For each behavior change, watch one meaningful test fail for the intended reason, implement enough to pass, then move on. Documentation and mechanical changes use appropriate checks rather than manufactured tests. When a meaningful red test cannot be run, report the gap and settle the reduced assurance before going on.
4. Run the relevant and all repository-required checks, bound to the committed revision or the uncommitted content as `verification` describes. Commit owned work where authorized. Use `change-map` for a multi-file handoff and `review` when its Level section calls for one.
5. Report the behavior delivered, commits, checks, and gaps, with observed red and green results apart from unavailable checks. Local readiness completes this path; publication follows only an explicit delivery request.

## Several slices


1. Read the epic and its slices in full, with their threads, and the agreed test seams. An epic without slices is cut into them first with `epic`'s steps 4 and 5, drafted locally, without publishing. For drafts, run `harness tickets check <feature-slug>`. Stop and ask when the seams were never agreed, a slice has no acceptance criteria that fail on the base, or the check reports a problem, such as a blocking cycle, because workers cannot repair those.
2. Create one integration branch for the run from the default branch, named as the floor's Working rules name a task branch. Record its base SHA, and keep a task document (`continuity`) with the slice list, status, and a `## Decisions` table when the run is long.
3. Compute the frontier: the tickets whose blockers are all integrated on the integration branch. Use the merged work on that branch as the source of truth for what is done. For drafts, run `harness tickets frontier <feature-slug> --done <integrated numbers>` and take its `ready` list; for published tickets, read tracker state only to find the edges.
4. Run the first slice alone, end to end: brief it, build it, check it, integrate it. Fan out only after it succeeds, and revise the brief template if it failed.
5. Before launching, list each frontier slice's expected file set from its scope and a quick search of the code it names. Slices whose sets overlap never run at the same time: launch one, and start the next from the integration head after the first is integrated, because parallel edits to one file come back as conflicts the parent must resolve by hand. For each frontier ticket that overlaps no running slice, launch one `executor` in its own worktree branched from the integration branch's current head, natively or through T3 Code's `delegate_task` with `isolation: "worktree"`, as `delegation` describes; commit the integration head first, because T3 Code prepares the child's checkout from the parent's commit. Write each brief to stand alone: the goal, the scope it may touch (its file set), the exact base SHA, how to verify it, and what to report. Tell the executor to use `tdd` at the agreed seams. Executors never commit, push, or merge. Build a slice yourself instead when it touches concurrency, a cross-cutting interface, or a path `change-map` rates `critical`, because those need the session's model rather than the executor's cheaper one; integrate it like any other slice.
6. When an executor reports, check the result against its scope and the actual Git state in its worktree; a completion message is not evidence. A file outside the slice's set is a scope finding: keep it only when the slice needed it and no running slice owns that file. A missing or unverifiable result is a gap, not a pass. Rerun the verification commands yourself.
7. Integrate each verified slice. The parent commits the slice on the worker's branch with a Conventional Commit, then merges it into the integration branch locally. Resolve a conflict by reading both sides; when it needs a product decision, record the gap and continue with independent slices. Then retire its executor worktree with `harness worktree retire <path> --into <integration branch>`, which removes the worktree and deletes the slice branch only when the branch is merged and the worktree holds no other work. When it refuses over ignored paths, pass `--allow-ignored` only when every path it lists is regenerable build output, since removal deletes them too. A slice whose merge stopped, or any other refusal, leaves the worktree in place, listed in the output. Recompute the frontier and continue from step 5 until no slice remains ready.
8. Apply the floor's stop rule per slice. When two fixes built on the same idea fail a slice's check, test that idea before another fix; when the test shows it is wrong and there is no new one, the slice stops and is reported with what was tried and the evidence, and its dependents wait. When a slice shows that a premise the whole epic shares is wrong (for example, the agreed seam cannot test the behavior), stop launching new slices and report it, because every remaining slice inherits the same mistake.
9. When every slice is integrated, run `verification`, `change-map`, and `review` on the integration branch, as `review`'s Level section sets for a build. Pass the full epic and agreed seams verbatim to the general reviewer. At `off`, report the branch unreviewed. A delegated correction counts against the review's role-call cap. Integrate confirmed fixes through step 7, record each `fixed` with its commit, and rerun affected checks. If the cap cannot cover a required correction and its recheck, report the gap for a human decision.
10. End with the integration branch ready. When the delivery request was given, open the pull request with `open-pr`; otherwise stop there and offer it once. Do not push, merge, or write to the tracker on your own. List every executor worktree step 7 left in place, with the reason.

## Output

For one slice, step 5's report. For several:

- Slices done, each with its commit, and the slices that remain.
- Gaps: missing or unverifiable results, stopped slices with their evidence, and decisions that wait on the human.
- Checks, each with its command and result against the final head.
- The change map for the integration branch.
- The review outcome, or that review was unavailable.
- The pull request link when delivery was requested.
