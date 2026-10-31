---
name: address-review
description: "Trigger: address review comments or fix red PR checks. Maintain the existing PR under its scoped authority."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Maintain an existing pull request: work through its unresolved review threads and failing required checks on the branch it already has.

## Why

Review comments and red checks are evidence and requests, not changes of scope. Staying on the retained branch, inside the authority the delivery already granted, keeps the PR one reviewable line of work.

## Steps

1. Resolve the PR, its requirements, current checks, and retained head branch. Reuse its checkout; if it is missing, a fresh checkout of that exact branch is fine. Inspect Git before editing so unrelated staged and unstaged work survives.
2. Read unresolved review threads and failing required checks, and group them by root cause. For a CI-only repair, skip the threads.
3. Fix supported defects, with regression coverage where the behavior warrants it and the relevant checks. Reject a mistaken finding with concrete evidence. Ask the human about consequential ambiguities and keep going on independent repairs.
4. Commit only owned changes. When the delivery is authorized, push the same branch, run the required remote checks, and refresh the PR metadata. A local-only repair stays local.
5. A request to address review threads includes replying to and resolving the existing threads once each fix is verified. Reply with the fixing commit or a specific evidence-backed explanation, and resolve only settled threads. A CI-only request carries no messages. Take product decisions to the human, not to the reviewers.
6. Record unresolved items and unavailable checks where `continuity` needs them. Merging and retiring the worktree belong to `land`.

## Output

Fixed and unresolved items, verification, whether changes remain local or were published, and any required review that remains unverified.
