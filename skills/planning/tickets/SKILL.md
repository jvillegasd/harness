---
name: tickets
description: "Break a spec, a triage brief, research findings, or a request into vertical-slice tickets, drafted locally and published only when asked. Invoke only by name: \"/tickets from the export spec\", \"/tickets from findings F2,F4\"."
argument-hint: "[spec path, triage brief, finding ids, or the scope; add \"publish to <tracker>\" to authorize the writes]"
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Break settled scope into tickets: tracer-bullet vertical slices with blocking edges, each one a pull request a worker or a session can finish on its own. Drafting is local; publishing to a tracker is a separate request.

Source: $ARGUMENTS

## Why

A ticket is the unit `build` hands a worker and `implement` takes in one session, so each one has to stand alone: what to build, how to prove it, and what it waits on. Cutting by behavior rather than by layer keeps every slice demoable, and cutting by dependency and risk puts the unknowns first. A spec is one source, not a prerequisite: a triage brief, research findings, or a settled request can be cut the same way. A ticket is never a prerequisite for coding.

## Steps

1. Read the source in full: the spec with its agreed test seams, a triage brief, the research findings by id, or the human's request. Read the tracker mode in `docs/agents/harness.json` and any referenced tickets with their full thread, and search for duplicates. Preserve identifiers when updating existing work.
2. Cut tracer-bullet vertical slices: each is a narrow but complete path through every layer, demoable or verifiable on its own, so ask of each "what can I demo?". Size a slice to one fresh context. Cut by dependency and risk, not by layer or file count. Reuse one existing ticket when one slice suffices, and create a parent only when it serves the requested grouping.
3. Put prefactoring first, following "make the change easy, then make the easy change". Break a wide refactor into expand, migrate in batches, and contract, so every slice leaves the system working.
4. Name each slice's test seam: the spec's agreed seam, or, without a spec, one proposed and confirmed with the human as `spec`'s Seam agreement describes, because `build` refuses a slice whose seam was never agreed.
5. Show the breakdown as a numbered list with each slice's title, what blocks it, and what it delivers. Ask about granularity and blocking edges before publishing, because rewriting a published graph is costly.
6. Write each ticket body with: what to build (behavioral and durable, with no file paths), acceptance criteria that fail on the base commit, the test seam, blocked by, and out of scope. A research finding keeps its evidence and confidence; do not turn an uncertain finding into a settled requirement. Draft locally beside the source, or under `docs/agents/tasks/<feature>/` when the source has no file.
7. When publication is requested, resolve the configured tracker and the exact write set; an explicit request to create tickets in a named tracker covers the scoped writes without another approval. Use native sub-issue and dependency relationships where supported and textual edges otherwise; a GitHub `blocked_by` edge takes the blocker's database id (`gh api repos/<owner>/<repo>/issues/<n> --jq .id`), not its number or node id; for file-backed trackers read `local-tracker` from its path. Publish blockers before dependents, preserve unrelated metadata, and read back every write. Apply a ready label only when the scope is settled and the blockers are satisfied; planning items stay outside the ready frontier.
8. After a partial failure, report what was created and what remains, then read the actual state before retrying so no duplicate appears. Never invent a parent, claim work, or start implementation as part of ticketing.

## Output

The numbered breakdown with each slice's blockers, seam, and what it delivers; the draft locations or the published links with their dependencies; and the open decisions, with the drafts clearly distinguished from anything published.
