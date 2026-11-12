---
name: tickets
description: "Cut a spec, a triage brief, research findings, or a request into tracer-bullet tickets, drafted locally and published only on request."
argument-hint: "[spec path, triage brief, finding ids, or the scope; add \"publish to <tracker>\" to authorize the writes]"
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Cut settled scope into tickets. Each ticket is a **tracer bullet**: a narrow path through every layer, finished as one pull request by one worker or one session, with acceptance checks that are red on the base commit. Drafts stay local; publishing them is its own request.

Source: $ARGUMENTS

## Why

`build` hands each ticket to a worker that sees nothing else, and `implement` takes one in a fresh session, so a ticket carries everything needed to build it and prove it. Tracer bullets keep every ticket demoable, and ordering by dependency and risk meets the unknowns first.

## Steps

1. **Read the source**: the spec with its agreed seams, the triage brief, the research findings by id, or the human's request, with every ticket it references and that ticket's full thread. Search the tracker that `docs/agents/harness.json` names, and the drafts under `docs/agents/tickets/`, for tickets already covering this work. Done when you can list every requirement in the source and every overlapping ticket or draft.
2. **Cut the tracer bullets.** Ask of each "what can I demo?", size it to one fresh context, and cut and order by dependency and risk. Prefactoring comes first ("make the change easy, then make the easy change"), and a wide refactor becomes expand, migrate in batches, and contract, so each ticket leaves the system working. When an overlapping ticket or draft already covers a bullet, update it in place and keep its identifier; create a parent only for a grouping the human asked for. Done when every requirement from step 1 sits in exactly one ticket's What to build or acceptance criteria, or is named in the breakdown as left out of this set.
3. **Name each seam**: the spec's agreed seam, or, without a spec, one proposed and confirmed with the human as `spec`'s Seam agreement describes. Done when every ticket has a confirmed seam, because `build` refuses a ticket without one.
4. **Show the breakdown** as a numbered list: each ticket's title, what blocks it, and what it delivers, and the requirements left out of this set. Done when the list is shown.
5. **Write each ticket** to `docs/agents/tickets/<feature-slug>/<NN>-<slug>.md`. The feature slug follows the spec's title, or names the feature in a few words when the source has no title, and takes a suffix when another feature's drafts already use it. Each ticket has:
   - **What to build**: the behavior in the domain's own words, durable and free of file paths.
   - **Acceptance criteria**: checks that are red on the base commit.
   - **Seam**: from step 3.
   - **Blocked by**: ticket numbers, or none.
   - **Out of scope**.

   A research finding keeps its evidence and confidence, so an uncertain finding stays an open question in the ticket. Done when every ticket has all five parts.
6. **Publish only when asked**: read [publishing](references/publishing.md) and follow it. Otherwise the drafts are the result.

Ticketing ends at drafts or published tickets; claiming a ticket and implementing it are later requests.

## Output

The numbered breakdown with each ticket's blockers, seam, and what it delivers; the draft paths or the published links; and the open questions, with drafts labeled apart from published tickets.
