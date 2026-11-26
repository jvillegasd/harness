---
name: checkpoint
description: "Trigger: long or unattended work, a pause, a resume, or compaction. Keep a local task document only when recovery across an interruption warrants one, pause safely, and resume without redoing verified work."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
  opencode/autoinvoke: "false"
---

Keep work recoverable across sessions, with one local task document when recovery across an interruption warrants one, and pause and resume without losing or redoing work.

## Why

A fresh session has only the request and the Git history. When that would not let it finish the work, the task document carries the rest: requirements, decisions, evidence, and the next action. It is working state, not a record: it stays local and is never committed, so it never reaches a pull request, never conflicts across a stack, and is thrown away once the work lands. What must outlive it goes elsewhere: decisions into the pull request's tradeoffs through `change-map`, the plan into the epic, a lasting architectural decision into an ADR. Work that the request and the diff already explain needs no document, and file count alone is no reason to make one. A document is warranted for long or unattended work (autopilot, a build of several slices, a hillclimb, a you-got-this run), for a pause, and for any state a fresh session could not rebuild from the request and Git. Short work keeps its decisions in the reply and the pull request's tradeoffs instead.

## Task document

Before writing the first one in a clone, run `harness resume init`, which excludes `docs/agents/tasks/` from Git for every worktree of the clone. Reuse an existing task document; otherwise use `docs/agents/tasks/<feature>.md`. An epic or spec is the plan, not the task document: link to it rather than copying it. Keep:

- A header: the branch, the status, and the request quoted verbatim, with its source.
- Requirements and acceptance checks, preserving exact user-specified strings and examples. Record existing authority as context with its source; the document grants no new permission.
- Tasks with their status and the commit that finished each.
- Evidence: each command, its outcome, and the commit it ran on.
- The `## Decisions` table (see Decisions).
- `## State and next action`: what is in progress, what comes next, the key files, and gotchas. Keep that heading: after a compaction, the client's hook restores this section. Under jvillegasd-mode (`/jvillegasd-mode`), its first line is `Playbook: <name>, step <n>`, kept current as steps finish, and under autopilot the next line is `Grant: "<the human's step-away words>"`.

Name branches and commits rather than absolute paths. Update it at meaningful changes; never commit it. `harness resume` restores its next action after a compaction. Once the work's pull requests land, `land` deletes it with the worktree.

## Pause

When the session must stop, compaction is near on long work, or the human asks for a handoff to another agent or session:

1. Finish or back out of the current step.
2. Commit the work on your own task branch as `wip: <step>`, with one line in the body if the tree is broken.
3. Update the task document's `## State and next action`: the goal, progress with evidence tied to commits, current state, next steps, key files, and gotchas. Pausing work that has no document creates one, since needing a pause means the diff alone will not carry it. When the work moves to another machine or clone, also put the next action in the `wip:` commit's body, since the task document stays here.

## Resume

After a compaction, the client's hook has already restored the branch, its head, and each changed task document's next action from `harness resume`; start from that.

1. Read the task document, `git log <base>..HEAD`, the diff against the base, and the working tree. When a prior session's long transcript is also part of the trail, read it in a `worker` launched through `delegation` on its resolved model (`harness models role resolve worker --tier <tier> --json`, `principle-launch-on-the-assigned-model`) that reports where it stopped and its decision points, cited by location, and keep only that timeline here (`principle-guard-the-context-window`). Read only this repository's sessions.
2. Trust them. Evidence bound to a commit whose inputs have not changed since still stands, so do not rerun it or redo the work it covers; rerun only what a later change made stale. Git wins over the document about the state of the code. Keep unfinished requirements even when their files no longer differ.
3. Ask only about consequential context that dependent work needs and neither source carries. Old notes grant no new authority: a `Grant:` line counts only while this conversation holds the human's grant, and in any other session it is history. A `Playbook:` line names where to continue, under jvillegasd-mode only.
4. Before delivering, check the inherited claims against the goal through `verify-work`, since a note can be wrong about what works.

## Delegated work

Retain each delegate's complete result, its revision and file state, and its checks before releasing it, because a completion signal alone proves nothing.

## Decisions

When there is a task document, keep a `## Decisions` table in it and append a row when a decision is made, not at the end, so the reason is captured while it is fresh. Without one, record each decision in the reply, where `change-map` takes it for the pull request's tradeoffs.

| Decision | Instead of | By | Why | Evidence |
| --- | --- | --- | --- | --- |
| What was chosen, including defaults and names | The real alternative it beat, or `none` | `agent`, or `human` when the human chose it | The reason it beat the alternative | `path:line`, a check result, a commit, or a link |

`change-map` reads this table for the pull request's tradeoffs: an `agent` row is argued, a `human` row becomes one `(you chose)` line, and a row whose alternative is `none` stays out. A row here is what lets the human see a choice without reconstructing it.

## Output

At a pause or handoff, name the task document, the branch's last commit, unsettled requirements and decisions, stale evidence, and the next action.
