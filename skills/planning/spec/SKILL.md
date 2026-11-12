---
name: spec
description: "Draft a local specification with agreed test seams, without publishing or implementing. Invoke only by name: \"/spec the export feature\"."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "3.0"
  opencode/autoinvoke: "false"
---

Draft a local specification when the human asks for one or when durable design reasoning is worth keeping, and agree the test seams with the human. A spec is optional, and a local draft needs no tracker configuration. Tickets are a separate request: `tickets` cuts a spec into slices.

## Why

A spec records what was settled and what was not, so later work starts from agreed behavior instead of a reconstruction. That only helps if exact requirements stay verbatim and uncertainty stays visible as uncertainty. Describing behavior rather than files keeps the spec true while the code moves, since file paths and snippets go stale first. The test seams are the one decision that shapes everything `tdd` and `review` do afterward, so they are confirmed with the human before code exists.

## Steps

1. Read the settled discussion and the relevant domain terms and ADRs, and reuse grounded evidence. Explore the code through `code-search` launched by `delegation` when the search is wide: file pointers, conventions, test commands, and entry points, not pasted contents. Settle an open question a run can answer with `proto`, and keep each prototype's branch and evidence for the spec. Ask the human only the consequential questions no run can settle, with the options and your recommendation, and draft the independent settled sections meanwhile.
2. Reuse an existing document; otherwise use the repository's spec location or `docs/agents/tasks/<feature>.md`. When the work needs recovery, fold the `continuity` information into this document instead of starting a second one.
3. Draft the sections below. Include exact examples and identifiers, and preserve the user's wording where it matters. Keep file paths and code snippets out of the body unless a prototype (`proto`) settled them, because they go stale; link the prototype branch as a primary source instead.
4. Write the rationale: the usage from the caller's view first, then the shape derived from that usage. When the evidence points to a different design than the one drafted, say so and redo it rather than patching it.
5. Agree the test seams (see Seam agreement) and record the human's answer in the spec.
6. Return the local draft. Approving it authorizes local implementation only; publishing the spec needs its own request, and a request to publish it does not create tickets. When publication is requested, use the tracker configuration for that destination and read back the result.

### Spec sections

- **Problem**: who is affected, what is wrong or missing, and why it matters now.
- **Behavior**: what a user or caller observes, in the domain's own words.
- **Acceptance criteria**: checks that fail before the change and pass after it.
- **Test seams**: where behavior is proven, per Seam agreement.
- **Scope and out of scope**: what the work covers and what it deliberately leaves alone.
- **Decisions**: each settled choice with its reason.
- **Risks**: what could go wrong, and what is still uncertain.

### Rationale

Keep this part short and honest, because its job is to let a reviewer judge the design without the author in the room.

- **Usage**: write the calls, commands, or screens as the caller would use them, before any internals.
- **Shape**: the structure that follows from that usage.
- **Alternatives considered**: at least one real alternative and why it lost. A spec with none has not been designed yet.
- **Tradeoffs accepted**: what this design gives up.
- **Open questions**: phrased as questions, each saying what depends on the answer.

Scrap the design and redo it when the work shows repeated workarounds, casts to get types to fit, a growing need for a lock, or two deviations of the same shape. Each is a sign that the shape does not fit the usage, and further patches only bury that.

### Seam agreement

Propose where behavior will be tested. Prefer an existing seam, then the highest seam that still fails for the right reason, ideally one. State what each seam proves and what it leaves unproven. Confirm the seams with the human before implementation starts. This is the one human checkpoint of the skill, because a wrong seam produces tests that pass while the behavior breaks, and the cost shows up only after the code is written. `tdd` implements at these seams and `review` checks against them.

## Output

The document locator, the settled behavior, the proposed test seams awaiting confirmation or the confirmed ones, what the prototypes proved and what stays unproven, and the consequential open questions, with the draft clearly distinguished from anything published.
