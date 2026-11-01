---
name: triage
description: "Turn a ticket or production error into an evidence-backed plan brief, without implementing."
disable-model-invocation: true
argument-hint: "[ticket id, URL, pasted ticket, or error signal]"
metadata:
  opencode/autoinvoke: "false"
---

Turn a ticket or a production error signal into an evidence-backed plan brief, without implementing. This is analysis only: it reads and queries, and may launch read-only `code-search` workers, but writes no files, edits no code, and makes no commits.

Item: $ARGUMENTS

## Why

A plan is only as good as its premise. Checking first whether the problem is real and reachable, and grounding the rest in cited code, keeps implementation from starting on a false premise.

Accept a tracker item, pasted ticket, or production error signal for a bug, feature, adjustment, or alert. Without input, name and use the conversation's latest unresolved item.

## Steps

1. Parse pasted content as the primary source, including comments and transcripts unavailable through the tracker API. For bugs, extract expected/actual behavior, reproduction steps, errors, and affected components. For features or adjustments, extract outcomes and constraints. Read every comment thread.

   Fetch each blob once, after step 2 permits proceeding, and only images cited as evidence, in one call for the needed images. Record what they show in the evidence ledger right away so later steps use the ledger, not a re-fetch or a subagent.

2. Before reading code, read and follow `evidence` and run the kill query: is this still happening, reachable by real users, or ever called? A kill result ends triage with a one-paragraph report. Report and skip unconfigured capabilities.

3. Supplement from the tracker only when the input is an id or URL with no pasted body. Where the tracker is local markdown, read the `local-tracker` skill: the id resolves to a ticket file, and reading it means reading its `## Comments` too.

4. Investigate the code to ground the analysis, citing `path:line`. Search the repository directly by default. Delegate only when a worker earns its launch: independent searches that can run in parallel, or context isolation that keeps a large search's file contents out of this context. Name that purpose in one line, launch each track as the `code-search` role (POLICY's Delegation section), and give each worker one bounded repository question. Handle its citations as `research` describes under Code-search citations. Then confirm or refute the root-cause hypothesis with at most two further evidence queries.

5. For remaining load-bearing ambiguity, rank the assumptions behind it by consequence, resolve each through repository precedent, history, and `evidence`, and stop if the premise is refuted. Batch only the surviving questions into one stop, as POLICY's "Asking and deciding" describes.

## Output

- **Summary**: one line, what the item asks plus your read of it.
- **Evidence ledger**: a table of claim, prediction, verdict (confirmed, refuted, or no data), number, and link. A refuted claim gets its own line saying what changed as a result.
- **Findings**: for a bug, the root-cause mechanism with `path:line` and a confidence of confirmed, likely, or unclear. Confidence is derived from the ledger, never asserted: confirmed requires a code-level proof or a confirming production number. For a feature, where it fits and what it touches.
- **Options**: genuinely viable approaches scaled to the stakes. Two for a small fix, three or four only for a real architectural fork. Each with what changes, pros, cons, and blast radius. Do not invent alternatives to fill a count.
- **Recommendation**: the preferred option and why, or the exact open questions if uncertain.
- **Plan brief**, the handoff contract:
  - Implementation contract: the recommended option in one paragraph.
  - Surfaces: every path the change must touch, and the ownership boundary.
  - Verification concerns: what a contract must prove, and how.
  - Open questions: each with its evidence, the options, your default, and the cost of the default being wrong.

## Handoff

The brief is planning input, not an implementation contract. Return it as is; a later implementation request can use it directly, and a specification or native plan is optional.

## Skills this loads

- `evidence`, read from its path
- `diagnosing-bugs` when the root cause resists the first two evidence queries
- `local-tracker`, read from its path, only when the input is a ticket id or path and `docs/agents/harness.json` sets `"tracker": "local-markdown"`
