---
name: proto
description: "Trigger: prototype, spike, or mock up an idea. Run optional local experiments to resolve a specific uncertainty."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Run a small local experiment to resolve a specific uncertainty. Ordinary implementation is not routed here merely because some decisions remain open.

## Why

An observable question is cheaper to answer by building the smallest slice than by asking or debating. The prototype keeps assumptions visibly separate from confirmed requirements, so the human can tell what the experiment showed.

## Steps

1. Name the uncertainty, the settled requirements, the explicit assumptions, and the smallest useful experiment. Inspect existing work and preserve unrelated changes. Reuse the branch, and isolate only when that helps.
2. Use `ponytail` and the project's existing styling layer. Build the slice and run the appropriate local checks. Keep assumptions distinct from confirmed requirements.
3. Show what to try, what the evidence supports, and what remains unknown. When human testing is the next source of evidence, wait for it; otherwise continue the independent experiments already authorized.
4. Incorporate feedback, and keep requirements in `continuity` when recovery needs them. Flag prototype-quality code or missing coverage that retained implementation must address.
5. Finish with the experimental result, or continue into implementation when asked. An explicit delivery request uses `open-pr`, and discarding the prototype goes through `abandon`.

## Output

The result, how to try it, the evidence and its limits, and any unresolved decision.
