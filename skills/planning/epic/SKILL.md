---
name: epic
description: "Turn a settled plan into an epic: one big ticket with the settled behavior and agreed test seams, cut into vertical slices drafted locally and published only on request. Invoke only by name: \"/epic the export feature\"."
argument-hint: "[the plan, a triage brief, finding ids, or the scope; add \"publish to <tracker>\" to authorize the writes]"
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "4.0"
  opencode/autoinvoke: "false"
---

Turn a settled plan into an **epic**: one big ticket that records what was settled, and its **slices**, the vertical tickets that build it. Each slice is a tracer bullet: a narrow path through every layer, finished as one pull request by one worker or one session, with acceptance checks that are red on the base commit. The epic and its slices stay local; publishing them is its own request.

Source: $ARGUMENTS

## Why

The epic is what later work starts from instead of a reconstruction, so exact requirements stay verbatim and uncertainty stays visible as uncertainty. It describes behavior rather than files, since paths and snippets go stale first. The test seams shape everything `tdd` and `review` do afterward, so they are the one checkpoint confirmed with the human before code exists. `build` hands each slice to a worker that sees nothing else, so a slice carries everything needed to build and prove it.

## Steps

1. **Ground.** Read the settled discussion, the domain terms in `GLOSSARY.md`, and the ADRs, and reuse grounded evidence. Explore wide code through `code-search` launched by `delegation`: file pointers, conventions, test commands, and entry points, not pasted contents. Settle a question a run can answer with `proto`, and keep its branch as a primary source. Search the tracker that `docs/agents/harness.json` names, and the drafts under `docs/agents/tickets/`, for work already covering this. Ask the human only the consequential questions no run can settle, with the options and your recommendation, and draft the independent settled sections meanwhile. When product decisions are still open, stop and say that `/grill-with-docs` settles them first. Done when no consequential product decision is open, or each one left is written as an open question saying what depends on it.
2. **Write the epic** to the repository's spec location or `docs/agents/tasks/<feature>.md`, reusing an existing document. It doubles as the `checkpoint` task document. Its sections:
   - **Problem**: who is affected, what is wrong or missing, and why it matters now.
   - **Behavior**: what a user or caller observes, in the domain's own words, with exact examples and the human's wording where it matters.
   - **Acceptance criteria**: checks that fail before the change and pass after it.
   - **Test seams**: per step 3.
   - **Scope and out of scope**.
   - **Decisions**: each settled choice with its reason.
   - **Rationale**: the usage from the caller's view first (the calls, commands, or screens), then the shape that follows from it, at least one real alternative and why it lost, the tradeoffs accepted, and the open questions. An epic with no alternative has not been designed yet.
   - **Risks**: what could go wrong, and what is still uncertain.
   - **Slices**: the numbered breakdown from step 4, linked to each draft.

   Keep file paths and code out of the body unless a prototype settled them. Redo the design rather than patching it when the work shows repeated workarounds, casts to make types fit, a growing need for a lock, or two deviations of the same shape.
3. **Agree the test seams.** Propose where behavior will be tested: an existing seam first, then the highest seam that still fails for the right reason, ideally one. State what each proves and leaves unproven, confirm them with the human, and record the answer in the epic. This is the skill's one human checkpoint, because a wrong seam produces tests that pass while the behavior breaks.
4. **Cut the slices.** Ask of each "what can I demo?", size it to one fresh context, and order by dependency and risk. Prefactoring comes first, and a wide refactor becomes expand, migrate in batches, and contract, so each slice leaves the system working. An overlapping ticket or draft is updated in place and keeps its identifier. Every acceptance criterion of the epic sits in exactly one slice, or is named as left out of this set. Show the breakdown: each slice's title, what blocks it, and what it delivers.
5. **Write each slice** to `docs/agents/tickets/<feature-slug>/<NN>-<slug>.md`, the slug following the epic's title, with a suffix when another feature already uses it. Each opens with `# <NN> <title>` and has these five `##` sections, in order: **What to build** (behavior, free of file paths), **Acceptance criteria** (red on the base commit), **Seam** (from step 3), **Blocked by** (`none`, or numbers in this feature), and **Out of scope**. A research finding keeps its evidence and confidence, so an uncertain finding stays an open question. Done when `harness tickets check <feature-slug>` prints nothing.
6. **Publish only when asked**: read [publishing](references/publishing.md) and follow it. Otherwise the local epic and drafts are the result.

Approving the epic authorizes local implementation only, as the floor says; building it is `build`.

## Output

The epic's path, the settled behavior, the test seams awaiting confirmation or confirmed, what the prototypes proved, the numbered slices with their blockers and seams, the draft paths or published links, and the open questions, with local drafts labeled apart from anything published.
