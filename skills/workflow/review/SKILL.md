---
name: review
description: "Trigger: the human asks for review, or `open-pr`, `build`, or POLICY calls for one at the level `harness review level` sets."
license: MIT
metadata:
  author: jvillegasd
  version: "3.0"
---

Get independent eyes on a change at the depth the human's review level allows. Blind hunters find defects. A finding survives only when a run proves it; everything else is dropped and counted.

## Why

A reviewer that has not heard the implementer's reasoning judges the code, not the story. Hunters favor recall and raise whatever they can trace; the review keeps only what a run proves, so the human reads defects instead of triaging guesses. A finding is dropped because no run proved it, never on the author's word. Each hunter's dropped count is reported, and the evals in `references/evals.md` show whether the rule costs recall. A review that cannot run independently is reported as unverified, never replaced by self-review.

## Level and coverage

Read `harness review level --json`; if it is unavailable, use `medium` and report the fallback. Only the human changes the setting. An explicit review uses the configured level, or Medium when the level is `off`. Only an explicit request for Deep selects Deep. Quick and Standard requests mean Medium and High.

| Level | Automatic trigger | Hunters | Conditional calls | Cap |
| --- | --- | --- | --- | --- |
| `off` | Never | Explicit requests use Medium | | 0 automatically |
| `medium` | PR above `tests-docs`; local handoff that is `critical` or names security, data-loss, compatibility, or concurrency risk | One general reviewer | None | 2 |
| `high` | PR or local handoff at `structural` or above | General reviewer plus one or two specialists | Executor, recheck | 6 |
| Deep (explicit only) | Human request | High plus a second general reviewer | Same as High | 7 |

Every general reviewer uses `references/bugs.md`: correctness, requirements, failure paths, compatibility, relevant security risks, project conventions, and test gaps. Higher levels add independent investigation; they never remove questions from Medium.

At High, select one or two specialists and state the concrete question each will investigate:

- `spec`: independently trace requirements to behavior when a requirements source exists.
- `risk`: inspect consequential trust boundaries, data loss, dependency failures, or concurrency using `references/risk.md`.
- `standards`: inspect changed interfaces, structure, and documented conventions using `references/standards.md`.

Choose by consequences, not file count. If no specialist has a distinct useful question, keep only the general reviewer and say why. Deep's second general reviewer comes from another vendor when the installed reviewer assignment permits it; its findings join the union, because different models catch different defects. Resolve roles through POLICY's delegation rules; never change models or install agents to obtain diversity, and report when diversity is unavailable.

The cap counts every role call in the review, retries and delegated fixes included. Splitting a large diff does not reset it. If coverage cannot fit, report the uncovered scope rather than exceeding the cap. When no review is triggered, record `no-review` with the level.

## Snapshot

1. Commit owned work where POLICY lets you, then record `<base>..<sha>`. Confirm both resolve with `git rev-parse` and that the diff is non-empty before launching anything. Hunters read the repository at `<sha>`; make no edits until every hunter has reported. Work you may not commit is reviewed from `git diff --binary <base> -- <owned paths>` saved in the branch directory (`dirname "$(harness review path)"`), with the patch's sha256 as its content identity. Such a review stops after Verify: record confirmed findings as `confirmed` and hand them to the human, because a fix cannot be committed or recorded `fixed`.
2. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, omit `spec` and report requirements coverage unavailable.
3. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, and any other file that documents how code is written here. Include the `ponytail` skill body for the standards hunter. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

When a diff is too large for one hunter, split it into coherent slices (by module or call path); each hunter covers its axis across the slices within the cap. An unreviewed slice stays unverified.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each its axis brief from `references/<axis>.md`, the requirements, the diff, the revision identity, and the standards. The `risk` hunter also gets the categories that triggered it and, for a security category, the `security-audit` excerpts its brief lists, pasted inline. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired if the cap permits, then report that hunter unverified.

## Verify

1. Collect every `bug`, `risk`, and `q`, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them here.
2. Try to reproduce each finding yourself, with repro files in a scratch directory outside the checkout. A run that fails for the reason the finding gives confirms it and becomes its red check. A defect in prose, such as two contradictory instructions, is confirmed by citing both lines. Never run code from a fork's pull request; a fork's findings that need a run are dropped.
3. **High and Deep, when a finding needs a run you could not do inline**, and never for a fork: record a sibling path in the branch directory as `worktree.txt`, create one detached worktree there with `git -c core.hooksPath=/dev/null worktree add --detach <path> <sha>`, apply the saved patch with `git -C <path> apply --binary <patch>` when the work is uncommitted, and launch one fresh `executor` there with the executor brief from `references/verify.md` and every such finding's text, never the rationale or which hunter raised it. `red` confirms the finding.
4. Dispose each finding:
   - **confirmed**: your run or the executor ran red, or contradictory prose was cited.
   - **dropped**: everything else, with what was tried: the run stayed green, could not run, or the finding needed a run this level does not launch. Report each hunter's dropped count and reasons, not the findings.
5. Clean up the executor's worktree: every `!!` line of `git -C <wt> status --porcelain --ignored` must be regenerable build output; save its work with `git -C <wt> add -A . && git -C <wt> diff --cached --binary <sha> > "<branch directory>/<wt name>.patch"`; only when that exited 0 and the file exists, run `git worktree remove --force <path in worktree.txt>` and delete `worktree.txt`. Otherwise leave both and report the worktree.

## Fix

Fix confirmed findings inline and commit. A code finding is fixed when its red check turns green; a prose finding is fixed when the cited lines no longer contradict each other, and needs no recheck. Delegate to one `executor` for every confirmed finding in one brief only when the cap covers it and any recheck it needs.

At Medium, the green check or the reconciled lines are the proof. At High and Deep, launch one fresh `reviewer` with the recheck brief from `references/verify.md`, giving it the confirmed findings, their checks, the repair delta `git diff <sha> <repaired sha>` over owned paths, the requirements, and the repaired revision; it reads the repaired commit in the checkout. A nit or a fix with no behavior change needs no recheck.

- `holds`: the finding stays `fixed`.
- `fails`, or a new defect in the repaired code: reproduce it on the repaired revision. A red run records the finding `confirmed` again (a regression as a new round 2 finding) and takes it to the human; otherwise drop it and count it. Never start a second repair loop on your own.
- `unknown`, or a report that does not echo the repaired revision: retry once if the cap permits; the green check stands, and the recheck is reported as unknown.
- `follow_ups`: report them beside the outcome; record none and repair none.

## Record

Record each disposition with `harness review record` (`references/ledger.md`) the moment it is decided. A review with no findings, or with no review triggered, writes one line too. Whoever commits a fix appends `fixed` with the fixing commit. The review posts nothing to the forge; `address-review` handles pull request threads.

## Output

- Level, coverage, selected specialists with their reasons, and calls used against the cap.
- The reviewed revision and content identity, and the standards loaded.
- Each axis's confirmed findings side by side, sorted by severity within the axis and never reranked across axes, each with its disposition and the run that proved it.
- Each hunter's raised, confirmed, and dropped counts, with the reasons findings were dropped.
- What stayed unverified: hunters, slices, any worktree left in place.

A change to the reviewed revision invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [risk](references/risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
