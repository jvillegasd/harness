---
name: review
description: "Trigger: a change is handed off (pull request, `build`, multi-file handoff), the human asks for review, or a concrete risk exceeds ordinary checks. Blind reviewers hunt at the depth `change-map` sets; blind executors prove each finding."
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
---

Get independent eyes on a change at the depth its risk earns. Blind hunters find defects, blind verifiers prove or refute each one, and the author never dismisses a finding alone.

## Why

A reviewer that has not heard the implementer's reasoning judges the code, not the story. That independence must hold at both ends: an author who triages findings with the full story undoes it at the last step. So findings are proven or refuted by agents that also never heard it. A review that cannot run independently is reported as unverified, never replaced by self-review.

## Depth

Take the highest tier from `change-map`:

| Highest tier | Depth | Hunters |
| --- | --- | --- |
| only `mechanical` or `tests-docs` | none | None; record `no-review` in the ledger |
| `structural` or `behavioral` | Quick | `bugs` |
| `critical`, or the human asks | Standard | `bugs`; `spec` when a requirements source exists; `standards`; `security` when a critical item touches auth, permissions, secrets, an input boundary, CI, or an installer |
| the human asks for Deep, or accepts it for a critical one-way door | Deep | Standard plus a second `bugs` hunter from another vendor |

Deep needs the reviewer assigned to a client other than the session's: check with `harness models resolve reviewer --json`. The `diversity` preset, or `harness models set reviewer --client <c> --model <m>` followed by `harness install`, does this. Both change machine state, so name them for the human rather than running them. The assigned hunters then launch through `orchestration`, as POLICY's Delegation section describes, and the extra `bugs` hunter launches natively on the session's client, so two vendors hunt bugs. When the reviewer resolves to the session's client, run Standard and say so.

## Snapshot

1. Fix the base and the reviewed commit. Commit owned work first where POLICY lets you, and add a content digest only for inputs that stay uncommitted. Confirm both revisions resolve with `git rev-parse` and the diff is non-empty before launching anything. Keep this identity with every report.
2. Take the complete diff, including owned untracked files, without staging unrelated work, and keep it whole. When it is too large for one hunter, split it into coherent slices (by module or call path) and give each slice its own hunters. An unreviewed slice stays unverified.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, skip the `spec` hunter and report "no spec available".
4. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, any other file that documents how code is written here, and the `ponytail` body. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each one its axis brief from `references/<axis>.md` pasted inline, plus the requirements, the scoped diff, the revision identity, and the standards. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired, then report that hunter unverified. Keep each complete report locally when recovery needs it.

## Verify

1. Collect every `bug`, `risk`, and `q` across hunters, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them in this change.
2. Look each finding up in the ledger (`references/ledger.md`). A repeat of an earlier `refuted` finding on an unchanged hunk is reported as "previously refuted: <reason>" and skipped.
3. Give each remaining finding its own detached worktree at the reviewed commit: `git worktree add --detach <scratch>/review-verify-<n> <sha>`, then apply any uncommitted snapshot with `git diff <sha> | git -C <worktree> apply`. In each one, launch a fresh `executor` with the executor brief from `references/verify.md` and the finding text. It is never told the rationale or which hunter raised the finding. Launch all of them in parallel.
4. Dispose each finding:
   - **confirmed**: the verifier's check ran red (ladder step 4 in `verification`). The fix lands with that check turned green.
   - **refuted**: the verifier could not make it fail, **and** a fresh `reviewer` given the refute brief from `references/verify.md` cites where the failure path is cut.
   - **unverified**: anything else. Take an unverified finding in security, privacy, auth, data, migration, or concurrency to the human, and report the rest as unverified.

   You may confirm a finding on your own run evidence. You never refute one alone.
5. Save each verifier's test as a patch (`git -C <worktree> diff`), then run `git worktree remove <worktree>` without `--force`. Leave and report a worktree that does not remove cleanly.

## Rounds

Run a second round only after substantive repairs to confirmed findings. It covers the repair delta at the same depth, with the context it needs, the requirements, and the new revision, and nothing from the earlier rationale or findings. The ledger lookup in Verify keeps it from relitigating. A nit alone starts no round.

Allow at most two rounds. A new confirmed finding in repaired code at the cap stops the loop: tell the human the approach may need rethinking instead of starting a third round. A substantive repair after the cap stays unverified until the human decides.

## Record

Append every finding with its disposition to the local ledger described in `references/ledger.md`. A review with no findings, or with depth none, gets one line too. The ledger stays on this machine. It never goes into a commit, pull request, or tracker, and a pull request carries only `open-pr`'s `Reviewed:` line.

The review posts nothing to the forge. `address-review` handles review threads from bots and other people on a pull request, using `references/triage-patterns.md`.

## Output

- The depth and the tier that set it.
- The inspected revision and content identity, and the standards loaded.
- Each axis's findings side by side, sorted by severity within the axis and never reranked across axes. Each finding carries its disposition and the ladder step it reached.
- The hunters, slices, and findings left unverified, and the remaining uncertainty.

A change to the snapshot invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [security](references/security.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md), [triage patterns](references/triage-patterns.md)
