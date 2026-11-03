---
name: proto
description: "Trigger: prototype, spike, or mock up an idea. Run optional local experiments, as a logic model or UI variants, to resolve a specific uncertainty."
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
---

Run a small local experiment to resolve a specific uncertainty. Ordinary implementation is not routed here merely because some decisions remain open.

## Why

An observable question is cheaper to answer by building the smallest slice than by asking or debating. The prototype keeps assumptions visibly separate from confirmed requirements, so the human can tell what the experiment showed. A prototype answers one stated question; when it answers several, it has become a product.

## Steps

1. Name the uncertainty as one question, the settled requirements, the explicit assumptions, and the smallest useful experiment. Inspect existing work and preserve unrelated changes. Reuse the branch, and isolate only when that helps.
2. Pick the shape (see Shapes) by the kind of question: a question about rules, state, or flow is a logic prototype, and a question about how something looks or feels is a UI prototype. A question that is neither can use a plain script or test.
3. Use `ponytail` and the project's existing styling layer. Build the slice and run the appropriate local checks. Keep assumptions distinct from confirmed requirements.
4. Show what to try, what the evidence supports, and what remains unknown. When human testing is the next source of evidence, wait for it; otherwise continue the independent experiments already authorized.
5. Incorporate feedback, and keep requirements in `continuity` when recovery needs them. Flag prototype-quality code or missing coverage that retained implementation must address.
6. Keep a prototype worth keeping on a throwaway local branch named `prototype/<name>`, and link that branch from the local spec or ticket draft as a primary source. Pushing it, or editing a published ticket to link it, needs its own request. Fold only the validated decision into the real code, since the prototype's own code was written to learn, not to last.
7. Finish with the experimental result, or continue into implementation when asked. An explicit delivery request uses `open-pr`, and discarding the prototype requires an explicit request and approval of the exact work to discard.

## Shapes

### Logic

For a question about state, rules, or flow, build one self-contained HTML file a non-developer can open from disk with no build step.

- A pure state module with no DOM access, so the rules can be read and tested apart from the page and later lifted into real code.
- A visible state panel that always shows the current state, so the effect of every action can be seen.
- Free-play controls for trying any action in any order.
- Scenario tabs that walk through the cases that matter, including the awkward ones, each stating the expected outcome.

### UI

For a question about look and feel, build 3 to 5 structurally different variants on one existing route, so the comparison uses real data and real surroundings. Variants that differ only in color or spacing answer a smaller question than the one worth asking, so vary the layout and the interaction model.

- Switch variants with a `?variant=` query parameter.
- Add a small floating switcher, hidden outside development, so it cannot reach users.
- Remove the variants that lost once the decision is made.

## Output

The result, how to try it, the evidence and its limits, the prototype branch when one is kept, and any unresolved decision.
