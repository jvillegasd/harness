---
name: continuity
description: "Trigger: long work, a pause, a resume, or compaction. Keep one committed task document, pause safely, and resume from it without redoing verified work."
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
---

Keep work recoverable across sessions with one task document committed on the task branch, and pause and resume without losing or redoing work.

## Why

A fresh session has only the request and the Git history. When that would not let it finish the work, the task document carries the rest: requirements, decisions, evidence, and the next action. Committed on the task branch, it is durable without any other copy, and it merges with the work as the record of what was done and why. Work that the request and the diff already explain needs no document, and file count alone is no reason to make one.

## Task document

Reuse an existing task or spec document; otherwise use `docs/agents/tasks/<feature>.md`. Keep:

- A header: the branch, the status, and the request quoted verbatim, with its source.
- Requirements and acceptance checks, preserving exact user-specified strings and examples. Record existing authority as context with its source; the document grants no new permission.
- Tasks with their status and the commit that finished each.
- Evidence: each command, its outcome, and the commit it ran on.
- The `## Decisions` table (see Decisions).
- `## State and next action`: what is in progress, what comes next, the key files, and gotchas. Keep that heading: after a compaction, the client's hook restores this section. Under work mode (`/work`), its first line is `Playbook: <name>, step <n>`, kept current as steps finish, and under autopilot the next line is `Grant: "<the human's step-away words>", until <time the ceiling ends>`.

Name branches and commits, never an absolute path or a worktree location, since the document outlives the machine. Update it at meaningful changes and commit it with the work where the floor lets you. When the pull request is about to open, `open-pr` sets the status to shipped for review with the final evidence and open decisions, so the default branch never receives a document that still reads in progress.

## Pause

When the session must stop, or compaction is near on long work:

1. Finish or back out of the current step.
2. Commit the work on your own task branch as `wip: <step>`, with one line in the body if the tree is broken.
3. Update the task document's `## State and next action`: the goal, progress with evidence tied to commits, current state, next steps, key files, and gotchas. Commit it. Pausing work that has no document creates one, since needing a pause means the diff alone will not carry it.

## Resume

After a compaction, the client's hook has already restored the branch, its head, and each changed task document's next action from `harness resume`; start from that.

1. Read the task document, `git log <base>..HEAD`, the diff against the base, and the working tree.
2. Trust them. Evidence bound to a commit whose inputs have not changed since still stands, so do not rerun it or redo the work it covers; rerun only what a later change made stale. Git wins over the document about the state of the code. Keep unfinished requirements even when their files no longer differ.
3. Ask only about consequential context that dependent work needs and neither source carries. Old notes grant no new authority: a `Grant:` line counts only while this conversation holds the human's grant and its time has not passed, and in any other session it is history. A `Playbook:` line names where to continue, under work mode only.
4. Before delivering, check the inherited claims against the goal through `verification`, since a note can be wrong about what works.

## Delegated work

Retain each delegate's complete result, its revision and file state, and its checks before releasing it, because a completion signal alone proves nothing.

## Decisions

Keep a `## Decisions` table in the task document and append a row when a decision is made, not at the end, so the reason is captured while it is fresh.

| Decision | Instead of | By | Why | Evidence |
| --- | --- | --- | --- | --- |
| What was chosen, including defaults and names | The real alternative it beat, or `none` | `agent`, or `human` when the human chose it | The reason it beat the alternative | `path:line`, a check result, a commit, or a link |

`change-map` reads this table for the pull request's tradeoffs: an `agent` row is argued, a `human` row becomes one `(you chose)` line, and a row whose alternative is `none` stays out. A row here is what lets the human see a choice without reconstructing it.

## Output

At a pause or handoff, name the task document, its last commit, unsettled requirements and decisions, stale evidence, and the next action.
