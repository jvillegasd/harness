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

1. Define the concern and the requirements. Snapshot the complete relevant change, including owned untracked files and relevant caller context, without staging unrelated work. Identify the base, HEAD, and a content digest for uncommitted inputs, and keep that identity with every report.
2. Launch the named `reviewer` in a fresh context and give it only the concern, the requirements stripped of rationale, the complete scoped diff, the revision identity, and the standards: project instructions and the `ponytail` body. Withhold conversation history, implementation rationale, rationale-bearing plans, continuity documents, worker results, and prior findings. Give project files by absolute path and paste external standards inline, because a read-only role cannot read outside the project. Keep the snapshot whole: narrow to a coherent scope or report the delivery as unavailable.
3. Require `loaded:` to name each standard read or supplied inline, and `revision:` to match the snapshot. Retry once with the input repaired when standards are missing or the output is incomplete; then report unverified. Retain the complete report locally when recovery needs it.
4. Triage `bug` and `risk` findings: fix, reject with concrete path and line evidence, or escalate an unresolved consequential decision. Keep the disposition with the evidence or report. Drop nits unless useful, answer questions, and reclassify only on evidence.
5. Run a second inspection only after substantive repairs that affect the concern, over the repair delta with the context it needs. Give it the requirements and the new revision, not the earlier rationale or findings. A nit alone starts no round. Allow at most two rounds: new blocking findings in repaired code, or an unresolved blocker at the cap, escalate. A substantive repair after the cap stays unverified until the human decides.

A missing, stale, or unavailable reviewer leaves the change unverified. The report stays with the parent, and the review posts nothing to the forge.

## Output

The concern, the inspected revision and content identity, the standards loaded, the findings with their dispositions, and the remaining uncertainty. A change to the relevant snapshot invalidates the earlier verdict until the affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
