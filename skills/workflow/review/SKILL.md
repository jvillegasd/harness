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

## Level

Read the level with `harness review level --json`; when the command is unavailable, use `medium` and say so. Only the human sets the level, so never run its setting form. It caps the reviews you start on your own. When the human asks for a review, it runs at any level, at Standard unless they name a depth.

| Level | You start a review when | Depth | Verify | After repairs | Subagents at most |
| --- | --- | --- | --- | --- | --- |
| `off` | never | none | none | none | 0 |
| `medium` (default) | a pull request is about to open with any tier above `tests-docs`, or a local handoff's highest tier is `critical` or names a concrete security, data-loss, compatibility, or concurrency risk | Quick | lean | none | 2 (3 in `build`, which adds `spec`) |
| `high` | the highest tier is `structural` or above, for a pull request or a local handoff | the Depth table | batched | one fix validator | 7: 6 to hunt and verify, 1 to validate |

The ceiling counts hunters, the verifier, the refuter, and the fix validator for one slice, and never more than four hunters; a diff split into slices multiplies it, and a delegated fix executor comes on top.

A review the human asked for verifies batched. Never launch a verifier or refuter per finding: the batch is what keeps the cost flat as findings grow. When no review runs because of the level, record `no-review` with the level in `--summary`.

## Depth

At `high`, or when the human asks for Deep, take the highest tier from `change-map`:

| Highest tier | Depth | Hunters |
| --- | --- | --- |
| only `mechanical` or `tests-docs` | none | None; record `no-review` in the ledger |
| `structural` or `behavioral` | Quick | `bugs` |
| `critical`, or the human asks | Standard | `bugs`; `spec` when a requirements source exists; `standards`; `risk` when a critical item falls in a category listed in `references/risk.md` |
| a critical one-way door (`change-map`'s merge danger), or the human asks for Deep | Deep | Standard plus a second `bugs` hunter from another vendor, within the four-hunter cap |

Four axes cover the review: `bugs` (behavior and whether the tests would catch it breaking), `spec` (the requirements), `standards` (the documented rules and smells), and `risk` (trust boundaries and dependency failures). A review never runs more than four hunters per slice. When Deep would need a fifth, drop `standards`, whose findings are mostly judgement calls, before any other axis.

Deep needs the reviewer assigned to a client other than the session's: check with `harness models resolve reviewer --json`. The `diversity` preset, or `harness models set reviewer --client <c> --model <m>` followed by `harness install`, does this. Both change machine state, so name them for the human rather than running them. The assigned hunters then launch through `orchestration`, as POLICY's Delegation section describes, and the extra `bugs` hunter launches natively on the session's client, so two vendors hunt bugs. When the reviewer resolves to the session's client, run Standard and say so.

## Snapshot

1. Fix the base and the reviewed commit. Commit owned work first where POLICY lets you, and add a content digest only for inputs that stay uncommitted. Confirm both revisions resolve with `git rev-parse` and the diff is non-empty before launching anything. Keep this identity with every report.
2. Save the complete diff of owned changes as the snapshot patch, with unrelated staged and unstaged work left out:
   1. Mark owned untracked files with `git add -N <paths>`, so the diffs include them.
   2. Run `git diff --binary <base> -- <owned paths>`. This is the snapshot patch. Hunters receive it whole, and its hash is the content digest when work stays uncommitted.
   3. When work stays uncommitted and Verify runs batched, also save the verifier patch, `git diff --binary <sha> -- <owned paths>`, because the verifier worktree starts at the reviewed commit.
   4. Restore the index with `git reset -q -- <those untracked paths>`.

   Save both patches outside the working tree, in the branch directory, as `<run>-snapshot.patch` and `<run>-verifier.patch`, where `<run>` is the short reviewed SHA plus a UTC timestamp. The branch directory holds the ledger: `d="$(dirname "$(harness review path)")" && mkdir -p "$d"`, which fails on a detached head rather than writing elsewhere. It sits in the Git directory, so Git never tracks it, every worktree of the repository shares it, and `land` deletes it with the branch. When it is too large for one hunter, split it into coherent slices (by module or call path) and give each slice its own hunters. An unreviewed slice stays unverified.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, skip the `spec` hunter and report "no spec available".
4. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, any other file that documents how code is written here. Include the `ponytail` skill body for the standards hunter to invoke for simplicity checks; its axis brief carries the smell baseline in `references/standards.md`. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each one its axis brief from `references/<axis>.md`, the requirements, the snapshot patch, the revision identity, and the standards. Give the brief and patch by absolute path when they sit inside the project, and inline otherwise. The `risk` hunter also gets the categories that triggered it and, for a security category, the `security-audit` excerpts its brief lists, pasted inline. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired, then report that hunter unverified. Keep each complete report locally when recovery needs it.

## Verify

1. Collect every `bug`, `risk`, and `q` across hunters, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them in this change.
2. Confirm a finding yourself when your own run reproduces it (ladder step 4); that run becomes its red check. A defect in prose with nothing to run, such as two instructions that contradict each other, is confirmed by citing both lines and needs no test. Refuting is never yours.
3. Look each remaining finding up in the ledger (`references/ledger.md`). Skip a repeat of a `refuted` entry only when both reviews were of committed work (`digest` null), that entry's commit is an ancestor of the reviewed commit (`git merge-base --is-ancestor`), and `git diff <entry sha> <sha>` is empty for the finding's path and for the path of the cut its evidence cites. Report it as "previously refuted: <reason>".
4. **Lean** (`medium`): launch no verifier. Try to reproduce each open finding yourself as step 2 describes, with your repro files in a scratch directory outside the checkout, never in it. Code from a fork's pull request is never run: walk those findings instead. Then send every finding still open to step 6.
5. **Batched** (`high`, or the human asked): run code unless the pull request is from a fork. A fork's code would need the sandbox `security-audit`'s "Universal execution safety" requires, because a test runs every package it imports, so for a fork send every finding to step 6 and count the skipped run as `could-not-run`. Otherwise create one sibling worktree for this run: `git -c core.hooksPath=/dev/null worktree add --detach "$(git rev-parse --show-toplevel)/../<repo>-review-<run>" <sha>`. The hooks setting keeps the reviewed code's checkout hooks out of your shell. When work is uncommitted, run `git -C <worktree> apply --index --binary <verifier patch>`. Record the worktree's baseline with `git -C <worktree> write-tree`, and record each path you create. Launch one fresh `executor` there with the executor brief from `references/verify.md` and every open finding's text, never the rationale or which hunter raised it. A `red` verdict confirms that finding.
6. Launch one fresh `reviewer` with the refute brief from `references/verify.md` and every finding that is still open, each with its verifier verdict when there was one.
7. Dispose each finding:
   - **confirmed**: your run or the verifier ran red. The fix lands with that check turned green.
   - **refuted**: not reproduced (the verifier reported `green` or `could-not-run`, or, when lean, your own run stayed green), **and** the refute brief cites where the failure path is cut.
   - **unverified**: anything else. Take each one to the human with both verdicts.
8. Clean up the recorded worktree:
   1. Check its ignored files first: every `!!` line of `git -C <worktree> status --porcelain --ignored` must be regenerable build output.
   2. Save the verifier's own work, staged and untracked files included, into the branch directory: `git -C <worktree> add -A . && git -C <worktree> diff --cached --binary <baseline> > "<branch directory>/<worktree name>.patch"`.
   3. Only when step 2 exited 0 and its patch file exists, remove the worktree with `git worktree remove --force <recorded path>`.

   Leave a worktree whose ignored files are not all build output or whose patch was not saved, and any path you did not record, and report it.

Fix confirmed findings inline. When you delegate the fix, give every confirmed finding to one fresh `executor` in one brief, never one executor per finding.

## Validate the fix

A review hunts once. Repairs get one check, not another hunt, so the cost of a review stays bounded.

At `high`, or when the human asked, once every confirmed finding is fixed with its check turned green, launch one fresh `reviewer` with the validator brief from `references/verify.md`. Give it the confirmed findings, their red checks, the repair delta (`git diff <reviewed sha> <repaired sha>` over owned paths), the requirements, and the repaired revision, and nothing from the earlier rationale, the other findings, or which hunter raised a finding. A nit or a fix with no behavior change needs no validator.

- `holds`: keep the finding `fixed`.
- `fails`: record the finding `unverified` with the validator's trace, since a read-only trace proves nothing ran, and take it to the human. Never start a second repair loop on your own.
- A new defect the validator reports in the repaired code: record it as a round 2 finding with disposition `unverified`, and take it to the human.
- `unknown`, or a report that does not echo the repaired revision: it is not a failure, so do not reopen the finding yet. Retry once with the input repaired; when the retry also gives no verdict, record the finding `unverified` with what the validator could not read, so `land` holds it for the human.
- `follow_ups`: report them to the human beside the review outcome. They are not findings: record none of them and start no repair for them.

At `medium` no validator runs, so a substantive repair stays unverified until the human decides. A further repair after validation is also unverified until the human decides.

## Record

Record each disposition with `harness review record` (`references/ledger.md`) the moment it is decided, so a later review's Verify lookup sees it. A review with no findings, or with depth none, writes one line too. Whoever commits a fix appends `fixed` with the fixing commit.

The review posts nothing to the forge. `address-review` handles review threads from bots and other people on a pull request, using its own [triage patterns](../address-review/references/triage-patterns.md).

## Output

- The level, the depth, and the tier that set it.
- The inspected revision and content identity, and the standards loaded.
- Each axis's findings side by side, sorted by severity within the axis and never reranked across axes. Each finding carries its disposition and the ladder step it reached.
- The hunters, slices, and findings left unverified, any verifier worktree left in place, and the remaining uncertainty.

A change to the snapshot invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [risk](references/risk.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md)
