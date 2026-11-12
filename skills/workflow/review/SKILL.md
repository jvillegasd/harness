---
name: review
description: "Trigger: the human asks for review, or `open-pr`, `build`, or `/work` calls for one at the level `harness review level` sets."
license: MIT
metadata:
  author: jvillegasd
  version: "3.0"
---

Get independent eyes on a change at the depth the human's review level allows. Blind hunters find defects. A finding survives only when a run proves it; everything else is dropped and counted.

## Why

A reviewer that has not heard the implementer's reasoning judges the code, not the story. Hunters favor recall and raise whatever they can trace; the review keeps only what a run proves, so the human reads defects instead of triaging guesses. A finding is dropped because no run proved it, never on the author's word. Each hunter's dropped count is reported, and the evals in `references/evals.md` show whether the rule costs recall. A review that cannot run independently is reported as unverified, never replaced by self-review.

## Level and coverage

Read `harness review level --json`; if it is unavailable, use `medium` and report the fallback. Only the human changes the setting. An explicit review uses the configured level, or Medium when the level is `off`. Quick and Standard requests mean Medium and High.

| Level | Automatic trigger | Hunters | Conditional calls | Cap |
| --- | --- | --- | --- | --- |
| `off` | Never | Explicit requests use Medium | | 0 automatically |
| `medium` | PR or `build` integration branch above `tests-docs`; local handoff that is `critical` or names security, data-loss, compatibility, or concurrency risk | One general reviewer | Fix executor | 2 |
| `high` | PR or local handoff at `structural` or above | General reviewer plus one or two specialists | Proof executor, fix executor, recheck | 6 |

The general reviewer is the `review-bugs` lens: correctness, requirements, failure paths, compatibility, relevant security risks, project conventions, and test gaps. Higher levels add independent investigation; they never remove questions from Medium.

At High, select one or both specialist lenses and state the concrete question each will investigate:

- `review-spec`: independently trace requirements to behavior when a requirements source exists.
- `review-risk`: inspect consequential trust boundaries, data loss, dependency failures, or concurrency. Name the categories that triggered it; for a security category the lens loads `security-audit` in guidance mode itself.

Choose by consequences, not file count. If no specialist has a distinct useful question, keep only the general reviewer and say why. Resolve roles through `delegation`.

Each lens is its own role, and each runs on the model `harness models` resolves for it: the `reviewer` assignment unless the human set that lens on its own. The fix recheck runs as `reviewer`. Never change models or install agents for a review.

The cap counts every role call in the review, retries and delegated fixes included. Splitting a large diff does not reset it. If coverage cannot fit, report the uncovered scope rather than exceeding the cap. When no review is triggered, record `no-review` with the level.

A review covering the current patch is required for every pull request this table triggers at `medium` or `high`, and for any the human asked to have reviewed: `open-pr`'s readiness report shows whether one does, `/land merge` refuses to merge without it, and `/land` after the human's merge reports when it was missing. At `off` the gap is reported without holding anything.

## Snapshot

1. Commit owned work where the floor lets you, then record `<base>..<sha>`. Confirm both resolve with `git rev-parse` and that the diff is non-empty before launching anything. Hunters read the repository at `<sha>`; make no edits until every hunter has reported. Work you may not commit is reviewed from `git diff --binary <base> -- <owned paths>` saved in the branch directory (`dirname "$(harness review path)"`), with the patch's sha256 as its content identity. Such a review stops after Verify: record confirmed findings as `confirmed` and hand them to the human, because a fix cannot be committed or recorded `fixed`.
2. Check whether the patch is already reviewed, unless the human asked for this review by name. Read `harness review list --head <sha>`. A recorded review covers the patch when its depth is the triggered level or deeper, it left no slice unreviewed, and its `sha` is `<sha>` or every commit after it is one the ledger names as a `fixed` commit. Report a covering review as the result, with its depth, its `sha`, and each finding still open, and stop: launch no one and record nothing, because a second hunt over the same content spends the cap again and finds what the first one did.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, omit `review-spec` and report requirements coverage unavailable.

When a diff is too large for one hunter, split it into coherent slices (by module or call path); each hunter covers its lens across the slices within the cap. An unreviewed slice stays unverified.

## Hunt

Launch one fresh lens role per hunter, all in parallel: its lens is in its definition, so give it only the requirements, the diff, and the revision identity, plus `review-risk`'s categories. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, the pull request body (its tradeoffs are rationale), worker results, and prior findings.

Each report must open with `loaded:` naming the standards files it opened, or `none`, and `revision:` matching the snapshot. Retry once with the input repaired if the cap permits, then report that hunter unverified.

## Verify

1. Collect every `bug`, `risk`, and `q`, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them here.
2. Try to reproduce each finding yourself, with repro files in a scratch directory outside the checkout. A run that fails for the reason the finding gives confirms it and becomes its red check. A defect in prose, such as two contradictory instructions, is confirmed by citing both lines. Never run code from a fork's pull request; a fork's findings that need a run are dropped.
3. **High, when a finding needs a run you could not do inline**, and never for a fork: record a sibling path in the branch directory as `worktree.txt`, create one detached worktree there with `git -c core.hooksPath=/dev/null worktree add --detach <path> <sha>`, apply the saved patch with `git -C <path> apply --binary <patch>` when the work is uncommitted, and launch one fresh `executor` there with the executor brief from `references/verify.md` and every such finding's text, never the rationale or which hunter raised it. `red` confirms the finding.
4. Dispose each finding:
   - **confirmed**: your run or the executor ran red, or contradictory prose was cited.
   - **dropped**: everything else, with what was tried: the run stayed green, could not run, or the finding needed a run this level does not launch. Report each hunter's dropped count and reasons, not the findings.
5. Clean up the executor's worktree: every `!!` line of `git -C <wt> status --porcelain --ignored` must be regenerable build output; save its work with `git -C <wt> add -A . && git -C <wt> diff --cached --binary <sha> > "<branch directory>/<wt name>.patch"`; only when that exited 0 and the file exists, run `git worktree remove --force <path in worktree.txt>` and delete `worktree.txt`. Otherwise leave both and report the worktree.

## Fix

Delegate every confirmed finding to one fresh `executor` in one brief, with the fix brief from `references/verify.md`. It edits the checkout and never commits. Read its diff, run each red check yourself, and commit the result. A code finding is fixed when its red check turns green; a prose finding is fixed when the cited lines no longer contradict each other, and needs no recheck. Fix inline only when the executor cannot launch or the cap has no call left for it beside any recheck the fix needs, and report which.

At Medium, the green check or the reconciled lines are the proof. At High, launch one fresh `reviewer` with the recheck brief from `references/verify.md`, giving it the confirmed findings, their checks, the repair delta `git diff <sha> <repaired sha>` over owned paths, the requirements, and the repaired revision; it reads the repaired commit in the checkout. A nit or a fix with no behavior change needs no recheck.

- `holds`: the finding stays `fixed`.
- `fails`, or a new defect in the repaired code: reproduce it on the repaired revision. A red run records the finding `confirmed` again (a regression as a new round 2 finding) and takes it to the human; otherwise drop it and count it. Never start a second repair loop on your own.
- `unknown`, or a report that does not echo the repaired revision: retry once if the cap permits; the green check stands, and the recheck is reported as unknown.
- `follow_ups`: report them beside the outcome; record none and repair none.

## Record

Record every disposition in the ledger as `references/ledger.md` describes.

## Output

- Level, coverage, selected specialists with their reasons, and calls used against the cap.
- The reviewed revision and content identity, and the standards each hunter loaded.
- Each axis's confirmed findings side by side, sorted by severity within the axis and never reranked across axes, each with its disposition and the run that proved it.
- Each hunter's raised, confirmed, and dropped counts, with the reasons findings were dropped.
- What stayed unverified: hunters, slices, any worktree left in place.

A change to the reviewed revision invalidates the verdict until the affected evidence is rechecked. The review posts nothing to the forge; `address-review` handles pull request threads.

## References

- [Reviewer role](roles/reviewer.md), the shared text each lens extends and the recheck's role
- Lens roles: [bugs](roles/review-bugs.md), [spec](roles/review-spec.md), [risk](roles/review-risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
