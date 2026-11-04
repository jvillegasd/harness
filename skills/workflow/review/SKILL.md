---
name: review
description: "Trigger: the human asks for review, or `open-pr`, `build`, or POLICY calls for one at the level `harness review level` sets."
license: MIT
metadata:
  author: jvillegasd
  version: "3.0"
---

Get independent eyes on a change at the depth the human's review level allows. Blind hunters find defects. The parent confirms what it can reproduce. Only a blind refuter can dismiss a finding, and the human decides everything that stays unproven.

## Why

A reviewer that has not heard the implementer's reasoning judges the code, not the story. A missed defect costs more than an extra question, so the review favors recall: a finding is never dropped on the author's word, and anything neither proven nor refuted goes to the human. A review that cannot run independently is reported as unverified, never replaced by self-review.

## Level and coverage

Read `harness review level --json`; if it is unavailable, use `medium` and report the fallback. Only the human changes the setting. An explicit review uses the configured level, or Medium when the level is `off`. Only an explicit request for Deep selects Deep. Quick and Standard requests mean Medium and High.

| Level | Automatic trigger | Hunters | Conditional calls | Cap |
| --- | --- | --- | --- | --- |
| `off` | Never | Explicit requests use Medium | | 0 automatically |
| `medium` | PR above `tests-docs`; local handoff that is `critical` or names security, data-loss, compatibility, or concurrency risk | One general reviewer | Refuter | 2 |
| `high` | PR or local handoff at `structural` or above | General reviewer plus one or two specialists | Executor, refuter, recheck | 6 |
| Deep (explicit only) | Human request | High plus a second general reviewer | Same as High | 7 |

Every general reviewer uses `references/bugs.md`: correctness, requirements, failure paths, compatibility, relevant security risks, project conventions, and test gaps. Higher levels add independent investigation; they never remove questions from Medium.

At High, select one or two specialists and state the concrete question each will investigate:

- `spec`: independently trace requirements to behavior when a requirements source exists.
- `risk`: inspect consequential trust boundaries, data loss, dependency failures, or concurrency using `references/risk.md`.
- `standards`: inspect changed interfaces, structure, and documented conventions using `references/standards.md`.

Choose by consequences, not file count. If no specialist has a distinct useful question, keep only the general reviewer and say why. Deep's second general reviewer comes from another vendor when the installed reviewer assignment permits it; its findings join the union, because different models catch different defects. Resolve roles through POLICY's delegation rules; never change models or install agents to obtain diversity, and report when diversity is unavailable.

The cap counts every role call in the review, retries and delegated fixes included. Splitting a large diff does not reset it. If coverage cannot fit, report the uncovered scope rather than exceeding the cap. When no review is triggered, record `no-review` with the level.

## Snapshot

1. Commit owned work where POLICY lets you, then record `<base>..<sha>`. Confirm both resolve with `git rev-parse` and that the diff is non-empty before launching anything. Hunters read the repository at `<sha>`; make no edits until every hunter has reported. Work you may not commit is reviewed from `git diff --binary <base> -- <owned paths>` saved in the branch directory (`dirname "$(harness review path)"`), with the patch's sha256 as its content identity.
2. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, omit `spec` and report requirements coverage unavailable.
3. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, and any other file that documents how code is written here. Include the `ponytail` skill body for the standards hunter. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

When a diff is too large for one hunter, split it into coherent slices (by module or call path); each hunter covers its axis across the slices within the cap. An unreviewed slice stays unverified.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each its axis brief from `references/<axis>.md`, the requirements, the diff, the revision identity, and the standards. The `risk` hunter also gets the categories that triggered it and, for a security category, the `security-audit` excerpts its brief lists, pasted inline. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired if the cap permits, then report that hunter unverified.

## Verify

1. Collect every `bug`, `risk`, and `q`, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them here.
2. Try to reproduce each finding yourself, with repro files in a scratch directory outside the checkout. A run that fails for the reason the finding gives confirms it and becomes its red check. A defect in prose, such as two contradictory instructions, is confirmed by citing both lines. Never run code from a fork's pull request; walk those findings instead. Refuting is never yours.
3. Check the ledger (`references/ledger.md`): a finding that repeats a `refuted` entry on committed work, whose commit is an ancestor of `<sha>` with no change since to the finding's path or the cited cut, is reported as "previously refuted: <reason>".
4. **High and Deep, when an open finding needs a run you could not do inline**, and never for a fork: create one detached worktree with `git -c core.hooksPath=/dev/null worktree add --detach <recorded sibling path> <sha>`, and launch one fresh `executor` there with the executor brief from `references/verify.md` and every such finding's text, never the rationale or which hunter raised it. `red` confirms the finding.
5. **Only when an unconfirmed `bug` remains**, launch one fresh `reviewer` with the refute brief from `references/verify.md` and every such `bug`, each with its executor verdict when there was one.
6. Dispose each finding:
   - **confirmed**: your run or the executor ran red, or contradictory prose was cited.
   - **refuted**: a `bug` that was not reproduced **and** the refuter cited where its failure path is cut.
   - **unverified**: everything else, including every unreproduced `risk` and `q`. Take each to the human with its evidence.
7. Clean up the executor's worktree: every `!!` line of `git -C <wt> status --porcelain --ignored` must be regenerable build output; save its work with `git -C <wt> add -A . && git -C <wt> diff --cached --binary <sha> > "<branch directory>/<wt name>.patch"`; only when that exited 0 and the file exists, run `git worktree remove --force <recorded path>`. Otherwise leave it and report it.

## Fix

Fix confirmed findings inline, each with its red check turned green, and commit. Delegate to one `executor` for every confirmed finding in one brief only when the cap covers it.

At Medium, the green check is the proof. At High and Deep, launch one fresh `reviewer` with the recheck brief from `references/verify.md`, giving it the confirmed findings, their checks, the repair delta `git diff <sha> <repaired sha>` over owned paths, the requirements, and the repaired revision; it reads the repaired commit in the checkout. A nit or a fix with no behavior change needs no recheck.

- `holds`: the finding stays `fixed`.
- `fails`, or a new defect in the repaired code: record it `unverified` (a regression as round 2) and take it to the human. Never start a second repair loop on your own.
- `unknown`, or a report that does not echo the repaired revision: retry once if the cap permits, then record the finding `unverified`.
- `follow_ups`: report them beside the outcome; record none and repair none.

## Record

Record each disposition with `harness review record` (`references/ledger.md`) the moment it is decided. A review with no findings, or with no review triggered, writes one line too. Whoever commits a fix appends `fixed` with the fixing commit. The review posts nothing to the forge; `address-review` handles pull request threads.

## Output

- Level, coverage, selected specialists with their reasons, and calls used against the cap.
- The reviewed revision and content identity, and the standards loaded.
- Each axis's findings side by side, sorted by severity within the axis and never reranked across axes, each with its disposition and the ladder step it reached.
- What stayed unverified: hunters, slices, findings, any worktree left in place.

A change to the reviewed revision invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [risk](references/risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
