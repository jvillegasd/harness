---
name: continuity
description: "Trigger: long work, interruption, resume, or a boundary between phases. Keep one task document, an optional Engram mirror, a decisions table, and choose how to cross the boundary."
license: MIT
metadata:
  author: jvillegasd
  version: "1.1"
---

Keep work recoverable across sessions with one task document, mirrored in project-scoped Engram when the work spans sessions, and decide how to cross a boundary between phases.

## Why

A fresh session has only the request and the Git diff. When the requirements, decisions, evidence, and next action would not survive that, the task document carries them, and the mirror protects against losing the file. Work that fits one session needs the document at most, so the mirror is optional there and required only for multi-session work. Bounded work that the request and diff already explain needs no document, and file count alone is no reason to make one. Reuse an existing task or spec document; otherwise use `docs/agents/tasks/<feature>.md`.

## Steps

1. Keep one document with requirements, acceptance checks, tasks and status, evidence with its revision and dirty-content identity, pending decisions, and the next action. Include the branch, worktree, and the document's own locator. Preserve exact user-specified strings and examples. Record existing authority as context with its source; the document grants no new permission.
2. Append each decision to the document's `## Decisions` table as it is made (see Decisions). At meaningful changes and handoffs, update the document and, for multi-session work, mirror its full contents and locator in one project-scoped Engram observation under a stable `task/<feature>` topic. Search in the project, read candidates fully, and update the same observation. Skip the mirror for work that fits one session. Treat an ambiguous identity or a failed read as unknown rather than absent, so no duplicate is created and nothing is overwritten blindly. Read back every write; a failed save or read-back leaves the mirror marked pending. Use the host's session identity as given.
3. On resume, read the document and its mirror when one exists, then inspect the branch, HEAD, index, working diff, and owned untracked files. Where the two copies conflict, keep both until evidence or the human resolves it, since a newer timestamp settles nothing. Keep unfinished requirements even when their files no longer differ. Checks against a changed or unidentified snapshot are stale, so re-derive the next action from the current state.
4. When one copy is unavailable, continue independently settled work from the readable copy and Git, record the missing mirror, and ask only about consequential missing context that dependent work needs.
5. Recover existing `checkpoint/<branch>` records and referenced legacy `handoff/<branch>` records when relevant. Validate their branch and reference identities, leave the originals in place, and carry valid requirements, rationale, pending work, and retained-result locators into the task document. Stale checks and recorded approvals carry over as neither evidence nor authority; unavailable legacy context stays pending.
6. For delegated work, retain complete result identities, unresolved requirements, revision and file state, checks, and any artifact locators. Verify retrieval before and after releasing disposable resources, and keep a durable local copy outside them when needed, since a completion signal alone proves nothing.
7. On completion, merge, or abandonment, update an existing document only when that helps and is authorized; a terminal state alone is no reason to create one. Preserve recovery material before an authorized worktree retirement, and report an unavailable mirror without blocking unrelated local work.

## Decisions

Keep a `## Decisions` table in the task document and append a row when a decision is made, not at the end, so the reason is captured while it is fresh.

| Decision | Why | Evidence |
| --- | --- | --- |
| What was chosen, including defaults and names the human did not ask for | The reason it beat the alternatives | `path:line`, a check result, a commit, or a link |

`change-map` reads this table for "decisions you did not ask for", so a row here is what lets the human see an agent-made choice without reconstructing it.

## Phase boundaries

Decide how to cross only at a boundary between phases, such as spec to build or build to review, and not mid-phase. Take the first option that fits:

1. **Continue in this context** when the next phase needs this one's reasoning verbatim, or when context remains.
2. **Clear** when everything held here is disposable and the files and the task document carry what matters.
3. **Hand off** (`handoff`) only for a new tool or client, a new repository, a colleague, or a side fork.
4. **Delegate to a fresh subagent** when the next task is tight enough to run unattended.
5. Otherwise **compact**, with a note of what comes next.

Every move except continuing turns first-hand context into a summary, and a summary loses detail that cannot be recovered, so prefer continuing while it fits.

## Output

At a handoff, name the task document, mirror status, unsettled requirements and decisions, stale or unavailable evidence, and the next action. Call the mirror durable only after its read-back succeeded.
