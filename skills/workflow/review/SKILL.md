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

Read `harness review level --json`; if it is unavailable, use `medium` and report the fallback. Only the human changes the setting. An explicit review uses the configured level, or Medium when the level is `off`.

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

## Target

The target is what the human or the calling skill names; with none, it is the current task branch. Resolve it to `<base>..<sha>` and a ledger key before the snapshot, and pass that key as `--branch` to every `harness review` call below:

- **Current task branch**: the work in this checkout; the key is the branch.
- **Pull request** (number or URL): read `gh pr view <n> --json headRefName,headRefOid,baseRefName,isCrossRepository`, run `git fetch origin <baseRefName>` and `git fetch origin refs/pull/<n>/head`, and require `git rev-parse FETCH_HEAD` to equal `headRefOid`. `<sha>` is `headRefOid`, `<base>` is `git merge-base origin/<baseRefName> <sha>`, and the key is `headRefName`, the name `land` reads. A fork (`isCrossRepository` true) is keyed `pr-<n>` and is reviewed without running anything.
- **Branch** (a local name or `origin/<name>`, fetched first when remote): `<sha>` is its tip, `<base>` the merge-base with its open pull request's base, or with the default branch when it has none. The key is the name without the remote.
- **Range** `<base>..<sha>`: as given, both resolved to full SHAs; the key is `range-<base>-<sha>` with each cut to 12 characters.

Every target other than the current task branch is foreign. A foreign review creates no refs of its own (a fetch by pull request lands only in `FETCH_HEAD`), commits nothing, pushes nothing, and is report-only: it skips Fix, and its confirmed findings stay `confirmed` for the human to route through `address-review` on the owning branch.

When a foreign `<sha>` is not what this checkout has checked out, hunters read it from one read worktree: write a fresh directory outside the checkout to `worktree.txt` in the key's directory (`dirname "$(harness review path --branch <key>)"`) first, then run `git -c core.hooksPath=/dev/null worktree add --detach <path> <sha>`. It is the only worktree this review creates, the High executor reuses it, and Verify's last step removes it, so it lives for one run; a run that dies leaves `worktree.txt`, which keeps `harness review remove` and `harness review prune` off that directory until it is retired.

## Snapshot

1. For the current task branch, commit owned work where the floor lets you, then record `<base>..<sha>`; a foreign target is already committed, and its `<base>..<sha>` comes from Target. Confirm both resolve with `git rev-parse` and that the diff is non-empty before launching anything. Hunters read the repository at `<sha>`; make no edits until every hunter has reported. Work you may not commit is reviewed from `git diff --binary <base> -- <owned paths>` saved in the branch directory (`dirname "$(harness review path)"`), with the patch's sha256 as its content identity. Such a review stops after Verify: record confirmed findings as `confirmed` and hand them to the human, because a fix cannot be committed or recorded `fixed`.
2. Check whether the patch is already reviewed, unless the human asked for this review by name: `harness review covers --branch <key> --base <base> --sha <sha> --level <level>`, adding `--digest <digest>` for uncommitted work. When it prints `covered: true`, report the covering review as the result, with its depth, its `sha`, and each finding it lists as still open, and stop: launch no one and record nothing, because a second hunt over the same content spends the cap again and finds what the first one did.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, omit `review-spec` and report requirements coverage unavailable.

When a diff is too large for one hunter, split it into coherent slices (by module or call path); each hunter covers its lens across the slices within the cap. An unreviewed slice stays unverified.

## Hunt

Launch one fresh lens role per hunter, all in parallel: its lens is in its definition, so give it only the requirements, the diff, and the revision identity, plus `review-risk`'s categories. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, the pull request body (its tradeoffs are rationale), worker results, and prior findings.

Each report must open with `loaded:` naming the standards files it opened, or `none`, and `revision:` matching the snapshot. Retry once with the input repaired if the cap permits, then report that hunter unverified.

## Verify

1. Collect every `bug`, `risk`, and `q`, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them here.
2. Try to reproduce each finding yourself, with repro files in a scratch directory outside the checkout. A run that fails for the reason the finding gives confirms it and becomes its red check. A defect in prose, such as two contradictory instructions, is confirmed by citing both lines. Never run code from a fork's pull request; a fork's findings that need a run are dropped.
3. **High, when a finding needs a run you could not do inline**, and never for a fork: use the read worktree of a foreign target, or record a sibling path in the branch directory as `worktree.txt` and create one detached worktree there with `git -c core.hooksPath=/dev/null worktree add --detach <path> <sha>`; then apply the saved patch with `git -C <path> apply --binary <patch>` when the work is uncommitted, and launch one fresh `executor` there with the executor brief from `references/verify.md` and every such finding's text, never the rationale or which hunter raised it. `red` confirms the finding.
4. Dispose each finding:
   - **confirmed**: your run or the executor ran red, or contradictory prose was cited.
   - **dropped**: everything else, with what was tried: the run stayed green, could not run, or the finding needed a run this level does not launch. Report each hunter's dropped count and reasons, not the findings.
5. Retire the worktree `worktree.txt` records, whether the executor's or a foreign target's read worktree: `harness worktree retire <path in worktree.txt> --at <sha> --save-patch "<branch directory>/<wt name>.patch"`, which saves whatever work the worktree holds as that patch before removing it. When it refuses over ignored paths, pass `--allow-ignored` only when every path it lists is regenerable build output. Delete `worktree.txt` once it exits 0; on any other refusal leave both and report the worktree.

## Fix

A foreign target skips this section.

Delegate every confirmed finding to one fresh `executor` in one brief, with the fix brief from `references/verify.md`. It edits the checkout and never commits. Read its diff, run each red check yourself, and commit the result. A code finding is fixed when its red check turns green; a prose finding is fixed when the cited lines no longer contradict each other, and needs no recheck. Fix inline only when the executor cannot launch or the cap has no call left for it beside any recheck the fix needs, and report which.

At Medium, the green check or the reconciled lines are the proof. At High, launch one fresh `reviewer` with the recheck brief from `references/verify.md`, giving it the confirmed findings, their checks, the repair delta `git diff <sha> <repaired sha>` over owned paths, the requirements, and the repaired revision; it reads the repaired commit in the checkout. A nit or a fix with no behavior change needs no recheck.

- `holds`: the finding stays `fixed`.
- `fails`, or a new defect in the repaired code: reproduce it on the repaired revision. A red run records the finding `confirmed` again (a regression as a new finding) and takes it to the human; otherwise drop it and count it. Never start a second repair loop on your own.
- `unknown`, or a report that does not echo the repaired revision: retry once if the cap permits; the green check stands, and the recheck is reported as unknown.
- `follow_ups`: report them beside the outcome; record none and repair none.

## Record

Record every disposition in the ledger as `references/ledger.md` describes, under the target's key with its `--base` and `--sha`. Every review that ends with no finding left `confirmed` and no slice unreviewed also records one whole-review `clean` line, at the head that holds the reviewed content: the repaired head when fixes landed, with `--digest` when the work is uncommitted. It is the one marker of a complete review: `harness review covers` reads it, and `land` reads it when the pull request body has no `Reviewed:` line, as for a foreign review, which writes nothing to the forge. A review that left a slice unreviewed records no `clean` line.

## Output

- The target, its ledger key, and whether it was foreign (report-only) or a fork (no runs).
- Level, coverage, selected specialists with their reasons, and calls used against the cap.
- The reviewed revision and content identity, and the standards each hunter loaded.
- Each axis's confirmed findings side by side, sorted by severity within the axis and never reranked across axes, each with its disposition and the run that proved it.
- Each hunter's raised, confirmed, and dropped counts, with the reasons findings were dropped.
- What stayed unverified: hunters, slices, any worktree left in place.

A change to the reviewed revision invalidates the verdict until the affected evidence is rechecked. The review posts nothing to the forge on its own; `address-review` handles pull request threads.

When the target was foreign and has an open pull request, post the result on it when the human's request already asked for that ("review 123 and post it"); otherwise end by asking whether to post. Only that request or a yes posts, as one comment with `gh pr comment <n> --body-file <file>`: the reviewed `<base>..<sha>`, the level, and each confirmed finding with its path, lines, severity, summary, and the run that proved it, or that none was confirmed. Dropped findings, hunter reasoning, and the ledger stay local. Without either, post nothing. That comment is the only forge write a review makes: approving or requesting changes stays with the human.

## References

- [Reviewer role](roles/reviewer.md), the shared text each lens extends and the recheck's role
- Lens roles: [bugs](roles/review-bugs.md), [spec](roles/review-spec.md), [risk](roles/review-risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
