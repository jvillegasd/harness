---
name: review
description: "Trigger: the human asks for review, or `open-pr`, `build`, or POLICY calls for one at the level `harness review level` sets."
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
---

Get independent eyes on a change at the depth its risk earns and the human's review level allows. Blind hunters find defects, a blind batch proves or refutes them, and the author never dismisses a finding alone.

## Why

A reviewer that has not heard the implementer's reasoning judges the code, not the story. That independence must hold at both ends: an author who triages findings with the full story undoes it at the last step. So findings are proven or refuted by agents that also never heard it. A review that cannot run independently is reported as unverified, never replaced by self-review.

## Level and coverage

Read `harness review level --json`; if unavailable, use `medium` and report the fallback. Only the human changes the setting. An explicit review uses the configured level; at `off`, an explicit request uses Medium. Only an explicit request for Deep adds the second correctness perspective. Quick and Standard requests mean Medium and High respectively for compatibility.

| Level | Automatic trigger | Coverage | Total role-call budget |
| --- | --- | --- | --- |
| `off` | Never | Explicit requests use Medium | 0 automatically |
| `medium` | PR above `tests-docs`; local handoff that is `critical` or names security, data-loss, compatibility, or concurrency risk | One general reviewer | 3 |
| `high` | PR or local handoff at `structural` or above | General reviewer plus one or two pertinent specialists | 6 |
| Deep (explicit only) | Human request | High plus a second independent general reviewer | 7 |

Every general reviewer uses `references/bugs.md`: correctness, requirements, failure paths, compatibility, relevant security risks, project conventions, and test gaps. Medium retains all these questions. Higher levels add independent investigation rather than removing questions from Medium.

At High, select one or two specialists and state the concrete question each will investigate:

- `spec`: independently trace requirements to behavior when a requirements source exists.
- `risk`: inspect consequential trust boundaries, data loss, dependency failures, or concurrency using `references/risk.md`.
- `standards`: inspect changed interfaces, structure, and documented conventions using `references/standards.md`.

Choose by consequences, not file count. If no specialist has a distinct useful question, retain the general reviewer and explain why. Deep adds a general reviewer from another vendor when the installed reviewer assignment permits it. Resolve roles through POLICY's delegation rules; never change models or install agents to obtain diversity. If diversity is unavailable, report that limitation and retain High coverage.

The budget covers the whole review: hunters, retries, verifiers, refuters, validators, and delegated fixes across every slice. Reserve one call for batch refutation and one for repair validation. High and Deep also reserve one for batch reproduction. Skip an unnecessary phase without spending its call. Fix inline by default. Splitting the diff does not reset the budget: each hunter covers its assigned axis across the slices. If coverage cannot fit, report the uncovered scope and request a larger budget rather than silently exceeding it.

When no review is triggered, record `no-review` with the level. An explicit request reviews even documentation-only work. Report planned and actual calls, including retries, against the total budget.

## Snapshot

1. Fix the base and the reviewed commit. Commit owned work first where POLICY lets you, and add a content digest only for inputs that stay uncommitted. Confirm both revisions resolve with `git rev-parse` and the diff is non-empty before launching anything. Keep this identity with every report.
2. Save the complete diff of owned changes as the snapshot patch, with unrelated staged and unstaged work left out:
   1. Mark owned untracked files with `git add -N <paths>`, so the diffs include them.
   2. Run `git diff --binary <base> -- <owned paths>`. This is the snapshot patch. Hunters receive it whole, and its hash is the content digest when work stays uncommitted.
   3. When work stays uncommitted, also save the verifier patch, `git diff --binary <sha> -- <owned paths>`, because the verifier worktree starts at the reviewed commit.
   4. Restore the index with `git reset -q -- <those untracked paths>`.

   Save both patches outside the working tree, in the branch directory, as `<run>-snapshot.patch` and `<run>-verifier.patch`, where `<run>` is the short reviewed SHA plus a UTC timestamp. The branch directory holds the ledger: `d="$(dirname "$(harness review path)")" && mkdir -p "$d"`, which fails on a detached head rather than writing elsewhere. It sits in the Git directory, so Git never tracks it, every worktree of the repository shares it, and `land` deletes it with the branch. When it is too large for one hunter, split it into coherent slices (by module or call path) and let the same assigned hunters inspect each slice within the total budget. An unreviewed slice stays unverified.
   Create one detached snapshot before hunting, using `git -c core.hooksPath=/dev/null worktree add --detach <recorded sibling path> <sha>`. Apply the verifier patch when present with `git -C <snapshot> apply --index --binary <patch>`. Record `git -C <snapshot> write-tree` as the baseline. All hunters and refuters read this snapshot, with project paths remapped into it. Paste external briefs inline. Finish all hunters before reproduction starts. Check baseline source identity before and after each phase; changes invalidate affected evidence. Reproduction writes only recorded test artifacts, which are saved and removed before refutation. Cleanup follows Verify step 8 even on a clean hunt.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, omit the `spec` specialist and report requirements coverage unavailable.
4. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, any other file that documents how code is written here. Include the `ponytail` skill body for the standards hunter to invoke for simplicity checks; its axis brief carries the smell baseline in `references/standards.md`. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each one its axis brief from `references/<axis>.md`, the requirements, the snapshot patch, the revision identity, and the standards. Give the brief and patch by absolute path when they sit inside the project, and inline otherwise. The `risk` hunter also gets the categories that triggered it and, for a security category, the `security-audit` excerpts its brief lists, pasted inline. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired only if the total budget permits, then report that hunter unverified. Keep each complete report locally when recovery needs it.

## Verify

1. Collect every `bug`, `risk`, and `q` across hunters, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them in this change.
2. Confirm a finding yourself when your own run reproduces it (ladder step 4); that run becomes its red check. A defect in prose with nothing to run, such as two instructions that contradict each other, is confirmed by citing both lines and needs no test. Refuting is never yours.
3. Look each remaining finding up in the ledger (`references/ledger.md`). Skip a repeat of a `refuted` entry only when both reviews were of committed work (`digest` null), that entry's commit is an ancestor of the reviewed commit (`git merge-base --is-ancestor`), and `git diff <entry sha> <sha>` is empty for the finding's path and for the path of the cut its evidence cites. Report it as "previously refuted: <reason>".
4. **Medium**: launch no verifier. Try to reproduce each open finding yourself as step 2 describes, with your repro files in a scratch directory outside the checkout, never in it. Code from a fork's pull request is never run: walk those findings instead. Then send every finding still open to step 6.
5. **High or Deep, only when open findings need executable reproduction**: run code unless the pull request is from a fork. A fork's code would need the sandbox `security-audit`'s "Universal execution safety" requires, because a test runs every package it imports, so for a fork send every finding to step 6 and count the skipped run as `could-not-run`. Otherwise use the recorded snapshot worktree and baseline. Launch one fresh `executor` there with the executor brief from `references/verify.md` and every open finding's text, never the rationale or which hunter raised it. A `red` verdict confirms that finding.
6. Only when findings remain open, launch one fresh `reviewer` with the refute brief from `references/verify.md` and every finding that is still open, each with its verifier verdict when there was one.
7. Dispose each finding:
   - **confirmed**: your run or the verifier ran red. The fix lands with that check turned green.
   - **refuted**: not reproduced (the verifier reported `green` or `could-not-run`, or, when lean, your own run stayed green), **and** the refute brief cites where the failure path is cut.
   - **unverified**: anything else. Take each one to the human with both verdicts.
8. Clean up the recorded worktree:
   1. Check its ignored files first: every `!!` line of `git -C <worktree> status --porcelain --ignored` must be regenerable build output.
   2. Save the verifier's own work, staged and untracked files included, into the branch directory: `git -C <worktree> add -A . && git -C <worktree> diff --cached --binary <baseline> > "<branch directory>/<worktree name>.patch"`.
   3. Only when step 2 exited 0 and its patch file exists, remove the worktree with `git worktree remove --force <recorded path>`.

   Leave a worktree whose ignored files are not all build output or whose patch was not saved, and any path you did not record, and report it.

Fix confirmed findings inline. Only when the remaining budget covers the fix and reserved validation, give every confirmed finding to one fresh `executor` in one brief, never one executor per finding.

## Validate the fix

A review hunts once. Repairs get one check, not another hunt, so the cost of a review stays bounded.

At every level, once every confirmed finding is fixed with its check turned green, launch one fresh `reviewer` with the validator brief from `references/verify.md`. Give it the confirmed findings, their red checks, the repair delta (`git diff <reviewed sha> <repaired sha>` over owned paths), the requirements, and the repaired revision, and nothing from the earlier rationale, the other findings, or which hunter raised a finding. A nit or a fix with no behavior change needs no validator.

- `holds`: keep the finding `fixed`.
- `fails`: record the finding `unverified` with the validator's trace, since a read-only trace proves nothing ran, and take it to the human. Never start a second repair loop on your own.
- A new defect the validator reports in the repaired code: record it as a round 2 finding with disposition `unverified`, and take it to the human.
- `unknown`, or a report that does not echo the repaired revision: it is not a failure, so do not reopen the finding yet. Retry once with the input repaired if the budget permits; when the retry also gives no verdict, record the finding `unverified` with what the validator could not read, so `land` holds it for the human.
- `follow_ups`: report them to the human beside the review outcome. They are not findings: record none of them and start no repair for them.

A passing regression check and a `holds` verdict complete a substantive repair at every level. If the validator is unavailable or the budget is exhausted, report the repair unverified. A further repair after validation is also unverified until the human decides.

## Record

Record each disposition with `harness review record` (`references/ledger.md`) the moment it is decided, so a later review's Verify lookup sees it. A review with no findings, or with no review triggered, writes one line too. Whoever commits a fix appends `fixed` with the fixing commit.

The review posts nothing to the forge. `address-review` handles pull request review threads, using its own [triage patterns](../address-review/references/triage-patterns.md).

## Output

- The configured level, effective coverage, risk tier, and selected specialists with their reasons.
- Planned and actual role calls against the whole-review budget.
- The inspected revision and content identity, and the standards loaded.
- Each axis's findings side by side, sorted by severity within the axis and never reranked across axes. Each finding carries its disposition and the ladder step it reached.
- The hunters, slices, and findings left unverified, any verifier worktree left in place, and the remaining uncertainty.

A change to the snapshot invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [risk](references/risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
