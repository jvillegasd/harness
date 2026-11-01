---
name: spec
description: "Trigger: spec this out, write a specification, or break work into tickets. Draft an optional local specification, its test seams, and when asked its tickets, without automatic publication or implementation."
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
---

Draft a local specification when the human asks for one or when durable design reasoning is worth keeping, agree the test seams with the human, and when tickets are requested break the spec into vertical slices. A spec is optional, and a local draft needs no tracker configuration.

## Why

A spec records what was settled and what was not, so later work starts from agreed behavior instead of a reconstruction. That only helps if exact requirements stay verbatim and uncertainty stays visible as uncertainty. Describing behavior rather than files keeps the spec true while the code moves, since file paths and snippets go stale first. The test seams are the one decision that shapes everything `tdd` and `review` do afterward, so they are confirmed with the human before code exists.

## Steps

1. Read the settled discussion and the relevant code, domain terms, and ADRs, and reuse grounded evidence. Ask only the unresolved consequential questions, and draft the independent settled sections meanwhile.
2. Reuse an existing document; otherwise use the repository's spec location or `docs/agents/tasks/<feature>.md`. When the work needs recovery, fold the `continuity` information into this document instead of starting a second one.
3. Draft the sections below. Include exact examples and identifiers, and preserve the user's wording where it matters. Keep file paths and code snippets out of the body unless a prototype (`proto`) settled them, because they go stale; link the prototype branch as a primary source instead.
4. Write the rationale: the usage from the caller's view first, then the shape derived from that usage. When the evidence points to a different design than the one drafted, say so and redo it rather than patching it.
5. Agree the test seams (see Seam agreement) and record the human's answer in the spec.
6. Return the local draft. Approving it authorizes local implementation only; publishing the spec or tickets needs its own request. When publication is requested, use the tracker configuration for that destination and read back the result.
7. Only when the human asks for tickets, break the spec into tickets (see Tickets). A request to publish the spec alone does not create tickets.

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

### Tickets

Tickets are optional, so skip this part unless the human asked for them. Drafting is local; publishing to a tracker is a separate request, and an explicit request to create tickets in a named tracker covers the scoped writes without another approval.

1. Read the tracker conventions and any referenced tickets with their full thread, and search for duplicates. Preserve identifiers when updating existing work.
2. Cut tracer-bullet vertical slices: each is a narrow but complete path through every layer, demoable or verifiable on its own, so ask of each "what can I demo?". Size a slice to one fresh context. Cut by dependency and risk, not by layer or file count.
3. Put prefactoring first, following "make the change easy, then make the easy change". Break a wide refactor into expand, migrate in batches, and contract, so every slice leaves the system working.
4. Show the breakdown as a numbered list with each slice's title, what blocks it, and what it delivers. Ask about granularity and blocking edges before publishing, because rewriting a published graph is costly.
5. Write each ticket body with: what to build (behavioral and durable, with no file paths), acceptance criteria that fail on the base commit, blocked by, and out of scope. A research finding keeps its evidence and confidence; do not turn an uncertain finding into a settled requirement.
6. When ticket publication is requested, resolve the configured tracker and the exact write set. Use native sub-issue and dependency relationships where supported and textual edges otherwise; for file-backed trackers use `local-tracker`. Publish blockers before dependents, preserve unrelated metadata, and read back every write. Apply a ready label only when the scope is settled and the blockers are satisfied; planning items stay outside the ready frontier.
7. After a partial failure, report what was created and what remains, then read the actual state before retrying so no duplicate appears. Never invent a parent, claim work, or start implementation as part of ticketing.

## Output

The document locator, the settled behavior, the proposed test seams awaiting confirmation or the confirmed ones, and the consequential open questions, with the draft clearly distinguished from anything published. For tickets, the numbered breakdown or the published links with their dependencies and open decisions.
