---
name: address-review
description: "Trigger: apply feedback to existing work, address review comments, or fix red PR checks. Maintain the existing branch and PR under their scoped authority."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Maintain existing work: apply the human's feedback, work through unresolved review threads, and repair failing required checks, all on the branch and pull request the work already has.

## Why

Feedback, review comments, and red checks are evidence and requests, not changes of scope. Staying on the retained branch, inside the authority the work already has, keeps it one reviewable line of work instead of a second PR or a restarted delivery.

## Steps

1. Resolve the work: the existing branch, its PR if there is one, its requirements, current checks, and the feedback or threads in play. Reuse its checkout; if it is missing, a fresh checkout of that exact branch is fine. Inspect Git before editing so unrelated staged and unstaged work survives. Use `continuity` only where recovery needs it.
2. Turn the feedback, unresolved threads, and failing required checks into actionable items grouped by root cause. For a CI-only repair, skip the threads. Ask the human about consequential ambiguities early, hold only the items that depend on the answer, and keep going on the rest.
3. Fix supported defects with `ponytail`, adding regression coverage where the behavior warrants it, and run the relevant checks through `verification`. Reject a mistaken finding with concrete evidence. Work the human deferred stays explicitly unverified until its checks run.
4. Commit only owned changes. When the work's delivery is authorized, push the same branch, run the required remote checks, and refresh the PR metadata. Otherwise finish locally and ask once for the concrete missing delivery action when the result is ready.
5. A request to address review threads includes replying to and resolving the existing threads once each fix is verified. Reply with the fixing commit or a specific evidence-backed explanation, and resolve only settled threads. A CI-only request carries no messages. Take product decisions to the human, not to the reviewers.
6. Record unresolved items and unavailable checks where `continuity` needs them. Merging and retiring the worktree belong to `land`.

## Output

Fixed and unresolved items, verification, whether changes remain local or were published, and any required review that remains unverified.
