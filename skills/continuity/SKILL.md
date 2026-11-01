---
name: continuity
description: "Trigger: long work, interruption, or resume needs recoverable intent. Keep one task document and a full project-scoped Engram mirror."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Keep work recoverable across sessions with one task document and a full mirror of it in project-scoped Engram.

## Why

A fresh session has only the request and the Git diff. When the requirements, decisions, evidence, and next action would not survive that, the task document carries them, and the mirror protects against losing the file. Bounded work that the request and diff already explain needs no document, and file count alone is no reason to make one. Reuse an existing task or spec document; otherwise use `docs/agents/tasks/<feature>.md`.

## Steps

1. Keep one document with requirements, acceptance checks, tasks and status, evidence with its revision and dirty-content identity, pending decisions, and the next action. Include the branch, worktree, and the document's own locator. Preserve exact user-specified strings and examples. Record existing authority as context with its source; the document grants no new permission.
2. At meaningful changes and handoffs, update the document and mirror its full contents and locator in one project-scoped Engram observation under a stable `task/<feature>` topic. Search in the project, read candidates fully, and update the same observation. Treat an ambiguous identity or a failed read as unknown rather than absent, so no duplicate is created and nothing is overwritten blindly. Read back every write; a failed save or read-back leaves the mirror marked pending. Use the host's session identity as given.
3. On resume, read both the document and its mirror, then inspect the branch, HEAD, index, working diff, and owned untracked files. Where the two copies conflict, keep both until evidence or the human resolves it, since a newer timestamp settles nothing. Keep unfinished requirements even when their files no longer differ. Checks against a changed or unidentified snapshot are stale, so re-derive the next action from the current state.
4. When one copy is unavailable, continue independently settled work from the readable copy and Git, record the missing mirror, and ask only about consequential missing context that dependent work needs.
5. Recover existing `checkpoint/<branch>` records and referenced legacy `handoff/<branch>` records when relevant. Validate their branch and reference identities, leave the originals in place, and carry valid requirements, rationale, pending work, and retained-result locators into the task document. Stale checks and recorded approvals carry over as neither evidence nor authority; unavailable legacy context stays pending.
6. For delegated work, retain complete result identities, unresolved requirements, revision and file state, checks, and any artifact locators. Verify retrieval before and after releasing disposable resources, and keep a durable local copy outside them when needed, since a completion signal alone proves nothing.
7. On completion, merge, or abandonment, update an existing document only when that helps and is authorized; a terminal state alone is no reason to create one. Preserve recovery material before an authorized worktree retirement, and report an unavailable mirror without blocking unrelated local work.

## Output

At a handoff, name the task document, mirror status, unsettled requirements and decisions, stale or unavailable evidence, and the next action. Call the mirror durable only after its read-back succeeded.
