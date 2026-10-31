---
name: review
description: "Trigger: requested independent review or a concrete risk beyond ordinary checks. Inspect a named concern without implementation rationale."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use for requested independent inspection, or a concrete security, data-loss, compatibility, or concurrency concern ordinary checks cannot adequately settle. Name the concern first; routine work needs no review.

## Hard Rules

Launch the named `reviewer` with a fresh context per policy. Never pass conversation history, implementation rationale, a rationale-bearing plan, continuity documents, worker results, or prior findings. A missing, stale, or unavailable reviewer is unverified, never replaced by self-review. No forge comments.

## Execution Steps

1. Define the concern and requirements. Snapshot the complete relevant change, including owned untracked files and relevant caller context, without staging unrelated work. Identify the base, HEAD, and content digest for uncommitted inputs. Keep that identity with every report.
2. Give the reviewer the concern, requirements stripped of rationale, complete scoped diff, revision identity, and standards: project instructions and the `ponytail` body. Give project files by absolute path; paste external standards inline because a read-only role cannot read outside the project. Never truncate a snapshot: narrow a coherent scope or report unavailable delivery.
3. Require `loaded:` to name each standard read or supplied inline, and `revision:` to match the snapshot. Retry missing standards or incomplete output once with the input repaired; then report unverified. Retain the complete report locally when recovery needs it.
4. Triage `bug` and `risk`: fix, reject with concrete path/line evidence, or escalate an unresolved consequential decision. Keep the disposition with the evidence or report. Drop nits unless useful; answer questions and reclassify only on evidence.
5. Run a second inspection only after substantive repairs affecting the concern, over the repair delta with needed context. Give requirements and the new revision, never the earlier rationale or findings. A nit alone starts no round. At most two rounds: new blocking findings in repaired code or an unresolved blocker at the cap escalate. A substantive repair after the cap remains unverified pending a human decision, never silently passed by self-review.

## Output Contract

Report the concern, inspected revision/content identity, standards loaded, findings and dispositions, and remaining uncertainty. A changed relevant snapshot invalidates the earlier verdict until affected evidence is rechecked.

## References

- [Reviewer role](roles/reviewer.md)
