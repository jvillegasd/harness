---
name: review
description: "Trigger: requested independent review or a concrete risk beyond ordinary checks. Inspect a named concern without implementation rationale."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Get an independent inspection of a named concern: a human-requested review, or a concrete security, data-loss, compatibility, or concurrency risk that ordinary checks cannot settle. Name the concern first, since routine work needs no review.

## Why

A reviewer that has not heard the implementer's reasoning judges the code instead of the story, so it can find what the author talked themselves past. That independence is the whole value, which is why a review that cannot run independently is reported as unverified instead of being replaced by self-review.

## Steps

1. Define the concern and the requirements. Snapshot the complete relevant change, including owned untracked files and relevant caller context, without staging unrelated work. Identify the base and HEAD, committing owned work first where POLICY lets you, and add a content digest only for inputs that stay uncommitted. Keep that identity with every report.
2. A `change-map` with `critical` items is a named concern: scope the review to those hunks and their callers, and name the risk the map gives. Launch the named `reviewer` in a fresh context and give it only the concern, the requirements stripped of rationale, the complete scoped diff, the revision identity, and the standards: project instructions and the `ponytail` body. Withhold conversation history, implementation rationale, rationale-bearing plans, continuity documents, worker results, and prior findings. Give project files by absolute path and paste external standards inline, because a read-only role cannot read outside the project. Keep each snapshot whole rather than truncated. When the scoped diff is too large for one reviewer, split it into coherent slices (by module or by the concern's call path), give each slice to its own fresh reviewer with the same concern, and report which slices were reviewed; an unreviewed slice stays unverified.
3. Require `loaded:` to name each standard read or supplied inline, and `revision:` to match the snapshot. Retry once with the input repaired when standards are missing or the output is incomplete; then report unverified. Retain the complete report locally when recovery needs it.
4. Triage `bug` and `risk` findings. A reviewer that cannot run commands reaches the ladder's step 3 at most (walked, see `verification`), so before fixing, try to reach step 4: write a test or script against the real code that fails because of the finding. A finding you reproduce is confirmed, and the fix lands with that failing check turned green where that is feasible. A finding you cannot reproduce is rejected with concrete path and line evidence, or escalated when it hinges on a consequential decision. A `q:` finding is a claim the reviewer could not trace; try to run it the same way, and drop it when it does not reproduce. Keep the disposition with the evidence or report. Drop nits unless useful, answer questions, and reclassify only on evidence.
5. Run a second inspection only after substantive repairs that affect the concern, over the repair delta with the context it needs. Give it the requirements and the new revision, not the earlier rationale or findings. A nit alone starts no round. Allow at most two rounds: new blocking findings in repaired code, or an unresolved blocker at the cap, escalate. A substantive repair after the cap stays unverified until the human decides.

## Adversary mode

An opt-in mode for changes where a missed defect is costly. The human asks for it, or the parent proposes it for a `critical` item that is a one-way door, and the human accepts. It replaces round 1's single reviewer with two reviewers on different clients, so one model's blind spots do not decide the verdict. It costs two reviews instead of one, which is why it stays opt-in, and it is not an extra round: the cap of two rounds is unchanged.

1. Resolve the reviewer with `harness models resolve reviewer --json`. Adversary mode needs that assignment on a client other than the session's, because `harness models launch-args` launches a role only on its assigned client. The `diversity` preset does this, as does a reviewer override (`harness models set reviewer --client <c> --model <m>`) followed by `harness install`; both change machine state, so name them for the human rather than running them. When the reviewer resolves to the session's client, or only one client is reachable, say so and run the normal single review. Otherwise launch one reviewer on the session's client natively and the resolved one through the locked `orchestration` skill, as POLICY's Delegation section describes.
2. Give both reviewers the same concern and the same blind brief as step 2, plus one instruction: try to break the change, and prove each defect on the ladder by tracing it to observable behavior.
3. Keep a finding only when the parent reproduces it (ladder step 4) or both reviewers walked it independently (step 3 each). Report every dropped finding with the reason, such as "one reviewer only, not reproduced". Triage the kept findings as in step 4.

A missing, stale, or unavailable reviewer leaves the change unverified. The report stays with the parent, and the review posts nothing to the forge.

## Output

The concern, the inspected revision and content identity, the standards loaded, the findings with their dispositions and how far each was proven on the ladder, the findings dropped in adversary mode with their reasons, and the remaining uncertainty. A change to the relevant snapshot invalidates the earlier verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
