---
name: continuity
description: "Trigger: long work, interruption, or resume needs recoverable intent. Keep one task document and a full project-scoped Engram mirror."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use only when recovery needs exceed what the request and Git diff preserve. For bounded reconstructable work, create no artifact. Reuse an existing task/spec document; otherwise use `docs/agents/tasks/<feature>.md`. File count is not the trigger.

## Hard Rules

Neither a document nor memory grants authority. Preserve exact user-specified strings and examples, unfinished requirements, decisions, and conflicts. Engram failure leaves local work usable and the mirror explicitly pending. Follow the host's session identity rules; never invent a session ID or bypass attribution.

## Execution Steps

1. Keep one document containing requirements, acceptance checks, tasks/status, evidence with revision and dirty-content identity, pending decisions, and next action. Include branch/worktree and the document locator. Record existing authority as context with its source, never as new permission.
2. At meaningful changes and handoffs, update the document and mirror its full contents and locator in one project-scoped Engram observation with a stable `task/<feature>` topic. Search in the project, read candidates fully, and update the same observation. Ambiguous identity or a failed read is not absence: do not create duplicates or overwrite blindly. Read back a successful write; a failed save/read-back leaves the mirror pending.
3. On resume, read both the document and its project-scoped mirror, then inspect branch, HEAD, index, working diff, and owned untracked files. Preserve both versions' conflicts until evidence or the human resolves them. A newer timestamp alone wins nothing. Keep unfinished requirements even if their files no longer differ. Invalidate checks against changed or unidentified snapshots and re-derive next action from current state.
4. If one copy is unavailable, proceed with independently settled work using the readable copy and Git. Record the missing mirror and ask only about consequential missing context needed for dependent work. Do not call unavailable evidence absent or complete.
5. Recover existing `checkpoint/<branch>` and referenced legacy `handoff/<branch>` records when relevant. Validate branch and reference identities, preserve their originals without deletion or overwriting, and carry valid requirements, rationale, pending work, and retained-result locators into the task document. Stale checks and recorded approvals do not survive as evidence or authority. Preserve unavailable legacy context as pending.
6. For delegated work, retain complete result identities, requirements left unresolved, revision and file state, checks, and Dispatch/artifact locators when applicable. Verify retrieval before and after releasing disposable resources; retain a durable local artifact outside them if necessary. A completion signal alone is insufficient. Reviewers never receive this document.
7. On completion, merge, or abandonment, update an existing document only when useful and authorized; never create one merely to mark a terminal state. Preserve recovery material before an authorized worktree retirement and report unavailable mirroring without blocking unrelated local work.

## Output Contract

Name the task document, mirror status, unsettled requirements/decisions, stale or unavailable evidence, and next action at a handoff. Do not report durable memory until read-back succeeds.
