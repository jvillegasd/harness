---
name: review
description: "Trigger: a change is handed off (pull request, `build`, multi-file handoff), the human asks for review, or a concrete risk exceeds ordinary checks."
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
| `critical`, or the human asks | Standard | `bugs`; `spec` when a requirements source exists; `standards`; `security` when a critical item falls in a category listed in `references/security.md` |
| a critical one-way door (`change-map`'s merge danger), or the human asks | Deep | Standard plus a second `bugs` hunter from another vendor |

Deep needs the reviewer assigned to a client other than the session's: check with `harness models resolve reviewer --json`. The `diversity` preset, or `harness models set reviewer --client <c> --model <m>` followed by `harness install`, does this. Both change machine state, so name them for the human rather than running them. The assigned hunters then launch through `orchestration`, as POLICY's Delegation section describes, and the extra `bugs` hunter launches natively on the session's client, so two vendors hunt bugs. When the reviewer resolves to the session's client, run Standard and say so.

## Snapshot

1. Fix the base and the reviewed commit. Commit owned work first where POLICY lets you, and add a content digest only for inputs that stay uncommitted. Confirm both revisions resolve with `git rev-parse` and the diff is non-empty before launching anything. Keep this identity with every report.
2. Save the complete diff of owned changes as the snapshot patch: mark owned untracked files with `git add -N <paths>`, then run `git diff --binary <base> -- <owned paths>` and restore the index with `git reset -q -- <those untracked paths>`. Unrelated staged and unstaged work stays out. Hunters and verifiers all receive this same patch, kept whole, and its hash is the content digest when work stays uncommitted. When it is too large for one hunter, split it into coherent slices (by module or call path) and give each slice its own hunters. An unreviewed slice stays unverified.
3. Copy the requirements verbatim from their source: the ticket body and comment thread (latest wins), the spec, the task document's requirements, or the human's exact words. A paraphrase carries your framing into a blind brief. With no source, skip the `spec` hunter and report "no spec available".
4. Gather the standards: the project instruction file, `CODING_STANDARDS.md` and `CONTRIBUTING.md` when present, any other file that documents how code is written here, and the `ponytail` body. Give project files by absolute path and paste external text inline, because a read-only role cannot read outside the project.

## Hunt

Launch one fresh `reviewer` per hunter, all in parallel. Give each one its axis brief from `references/<axis>.md`, the requirements, the snapshot patch, the revision identity, and the standards. Give the brief and patch by absolute path when they sit inside the project, and inline otherwise. The `security` hunter also gets the categories that triggered it and, pasted inline, the `security-audit` excerpts its brief lists. Withhold conversation history, implementation rationale, plans that carry rationale, continuity documents, worker results, and prior findings.

Each report must open with `loaded:` naming the standards and `revision:` matching the snapshot. Retry once with the input repaired, then report that hunter unverified. Keep each complete report locally when recovery needs it.

## Verify

1. Collect every `bug`, `risk`, and `q` across hunters, and dedupe by path, line range, and class. Report `nit` and `pre-existing` findings without verifying or fixing them in this change.
2. Confirm a finding yourself when your own run reproduces it (ladder step 4) or the cited lines plainly contradict each other. Refuting is never yours.
3. Look each remaining finding up in the ledger (`references/ledger.md`). Skip a repeat of a `refuted` entry only when that entry's commit is an ancestor of the reviewed commit (`git merge-base --is-ancestor`) and `git diff <entry sha> <sha> -- <path>` is empty. Report it as "previously refuted: <reason>".
4. Give each finding still open its own sibling worktree, named for this run (`<run>` is the short reviewed SHA plus a UTC timestamp). Run `git -c core.hooksPath=/dev/null worktree add --detach ../<repo>-review-<run>-<n> <sha>`; the hooks setting keeps the reviewed code's checkout hooks from running in your shell. When work is uncommitted, run `git -C <worktree> apply --index --binary <snapshot patch>` so the snapshot sits in the index. Record each path you create. In each worktree, launch a fresh `executor` with the executor brief from `references/verify.md` and the finding text, never the rationale or which hunter raised it. Launch them all in parallel.
5. Dispose each finding:
   - **confirmed**: the verifier ran red. The fix lands with that check turned green.
   - **refuted**: the verifier reported `green` or `could-not-run`, **and** a fresh `reviewer` given the refute brief from `references/verify.md` cites where the failure path is cut.
   - **unverified**: anything else. Take each one to the human with both verdicts.
6. Save each verifier's own work as a patch, new files included: `git -C <worktree> add -N . && git -C <worktree> diff --binary`. When the patch is non-empty, remove the worktree with `git worktree remove --force <recorded path>`. Leave a worktree whose patch is empty, or whose path you did not record, and report it.

## Rounds

Run a second round only after substantive repairs to confirmed findings. It covers the repair delta at the same depth, with the context it needs, the requirements, and the new revision, and nothing from the earlier rationale or findings. The ledger lookup in Verify keeps it from relitigating. A nit alone starts no round.

Allow at most two rounds. A new confirmed finding in repaired code at the cap stops the loop: tell the human the approach may need rethinking instead of starting a third round. A substantive repair after the cap stays unverified until the human decides.

## Record

Append each disposition to the ledger (`references/ledger.md`) the moment it is decided, so the Verify lookup in a later round sees it. A review with no findings, or with depth none, writes one line too. The skill that commits a fix (`open-pr`, `build`, or `address-review`) appends `fixed` with the fixing commit.

The review posts nothing to the forge. `address-review` handles review threads from bots and other people on a pull request, using `references/triage-patterns.md`.

## Output

- The depth and the tier that set it.
- The inspected revision and content identity, and the standards loaded.
- Each axis's findings side by side, sorted by severity within the axis and never reranked across axes. Each finding carries its disposition and the ladder step it reached.
- The hunters, slices, and findings left unverified, any verifier worktree left in place, and the remaining uncertainty.

A change to the snapshot invalidates the verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
- Axis briefs: [bugs](references/bugs.md), [spec](references/spec.md), [standards](references/standards.md), [security](references/security.md)
- [Verify briefs](references/verify.md), [ledger](references/ledger.md), [evals](references/evals.md), [triage patterns](references/triage-patterns.md)
