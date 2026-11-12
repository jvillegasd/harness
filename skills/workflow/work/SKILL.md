---
name: work
description: "Work mode: match a task to a playbook and run it to its steps, or on autopilot while you step away. Invoke only by name: \"/work fix the flaky login test\", \"/work autopilot, I'm stepping away: ship the export feature\"."
argument-hint: "[the task; add \"autopilot\" or \"I'm stepping away\" with a goal to run it unattended]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Work mode: the task is matched to one playbook, the playbook's steps go into the todo list, and the work runs to those steps. Only the human starts it, by typing `/work`, because starting it opts into this rigor.

Task: $ARGUMENTS

## Start

1. Restate the human's goal in a line, and the smallest change you expect to meet it. When the evidence rules out the change they asked for, show it and stop.
2. When the request grants a step-away ("autopilot", "I'm stepping away", "going to bed, keep going"), read [autopilot](playbooks/autopilot.md) now; it frames the run before anything else.
3. Match the task to one row of the matcher and read that playbook.
4. Copy every step of the playbook into the todo list verbatim, before any task-specific item. A step you will not take stays in the list as `skip: <reason>`, so the human sees what was left out and why.
5. When the work has a task document (`continuity`), record the playbook and step there as `continuity` describes, so a compaction resumes the same step.

Work mode lasts the conversation. "New task" means match again from step 1, and "stop work mode" ends it.

## Matcher

| The task | Playbook |
| --- | --- |
| Something is broken, throwing, or failing | [bug fix](playbooks/bug-fix.md) |
| New or changed behavior | [feature](playbooks/feature.md) |
| A read-only question: how, why, are we sure, should we | [investigate](playbooks/investigate.md) |
| The structure changes and the behavior does not | [refactor](playbooks/refactor.md) |
| Slow, with a number to move | [perf](playbooks/perf.md) |
| A shape or interface to choose, with no precedent | [design](playbooks/design.md) |
| A prototype, spike, or mock-up | `proto`'s steps |
| A spec or plan for a change, before anything is built | `spec`'s steps. When product decisions are still open, reply that `/grill-with-docs` settles them first, and stop; under autopilot they are `agent` decisions instead |
| A ticket or a production error to turn into a plan brief, before any fix | `triage`'s steps |
| Tickets from a spec, a triage brief, research findings, or a request | `tickets`' steps |
| A settled spec or set of tickets to build | `build`'s steps |
| Review a diff, a branch, or a PR | `review`'s steps, at the depth the human asked |
| An existing PR: review comments, red checks, conflicts | `address-review`'s steps, with its mode declared |
| Open a PR, ship this | `open-pr`'s steps |
| Merge, or "it merged, clean up" | Reply "run `/land` in this worktree" and stop. Merging and cleanup belong to the human through `land` |
| Resume or pick up earlier work | `continuity`'s Resume, then continue the playbook and step it recorded, or match what is left |
| Large, multi-part, or none of the above | [large](playbooks/large.md) |

A bug that is a slowdown with a number is perf. A settled spec or ticket small enough for one slice matches the row for its kind, since `implement` reuses its agreed seams. When the task fits two rows, take the narrower one, and the large playbook only when no narrower row covers it.

A row that names a skill runs that skill's steps as the playbook: they go into the todo list like any playbook's, and work mode adds only its triggers and reply. `spec`, `tickets`, `build`, and `triage` are guarded so the model never starts them on its own; read each from its path in the installed skills, because the human's `/work` request is what starts it. A spec hands back asking "go to build it?". Only an explicit go answers it, since approving a spec authorizes local implementation alone; the go is a new task, a settled spec matched again.

## Triggers

These hold at every step of every playbook.

- **A broken skill mid-task** (a step that names a missing script, a stale path, two skills that contradict each other): fix it on its own branch. Do not block on it and do not silently work around it. It becomes its own PR when delivery is authorized. When the broken skill belongs to the harness and you are working in another repository, write a ready-to-apply note instead: the skill, the defect, the evidence, and the proposed fix, in the task document and the reply.
- **A step that verifies** passes only on the surface it names. Inconclusive, or green or measured on another surface, is not a pass.
- **A role to launch** (executor, code-search, reviewer, a review lens): launch it through `delegation`.
- **A principle applies** (the index below): read that principle's `SKILL.md` in full from the installed skills (`~/.claude/skills` in Claude Code, `~/.agents/skills` in Codex and OpenCode) before acting on it, and name in the reply each principle that changed a decision.

### Principles

- `principle-laziness-protocol`: refactoring, sizing a diff, or tempted to add abstractions or layers.
- `principle-foundational-thinking`: before writing logic, when choosing core types, data structures, and what concurrent actors share.
- `principle-redesign-from-first-principles`: integrating a new requirement into an existing design.
- `principle-attack-the-premise`: two or more fixes that share one premise have failed the same gate.
- `principle-subtract-before-you-add`: sequencing an addition, refactor, or rewrite.
- `principle-minimize-reader-load`: reviewing or shaping code that is hard to trace.
- `principle-outcome-oriented-execution`: planned rewrites and migrations with explicit phases.
- `principle-experience-first`: product, UX, or feature-scope tradeoffs.
- `principle-exhaust-the-design-space`: a novel interaction or architectural decision with no precedent.
- `principle-build-the-lever`: non-trivial work a tool could do or prove.
- `principle-model-the-domain`: stateful logic, or code that branches a lot or repeats a shape assumption.
- `principle-boundary-discipline`: wiring validation, error handling, or framework adapters.
- `principle-type-system-discipline`: designing types or a signature in a typed language.
- `principle-make-operations-idempotent`: commands, lifecycle steps, or loops that run amid crashes and retries.
- `principle-migrate-callers-then-delete-legacy-apis`: a new internal API while old callers exist.
- `principle-separate-before-serializing-shared-state`: concurrent actors that might write the same file, branch, key, or object.
- `principle-prove-it-works`: before declaring a task done.
- `principle-fix-root-causes`: debugging.
- `principle-sequence-verifiable-units`: multi-step work, and how commits and pull requests stack.
- `principle-test-behavior-not-implementation`: writing, changing, or keeping a test.
- `principle-explain-the-number`: before measuring, and before trusting or acting on a measured number.
- `principle-guard-the-context-window`: large outputs, long files, repeated reads, or fan-out planning.
- `principle-never-block-on-the-human`: tempted to ask whether to do reversible work.
- `principle-encode-lessons-in-structure`: writing the same instruction a second time, or a recurring correction.

## Finish

A playbook's Finish step is these three, each its own item in the todo list:

1. Run `change-map`.
2. Run `review` at the level `harness review level` reports.
3. When delivery is authorized, run `open-pr`. Otherwise stop with the work ready locally and offer delivery once.

## Reply

Every playbook ends with a reply that:

- opens with the human's goal;
- carries, for each claim, its evidence or its label in the same sentence: measured, inferred, or guessed (a prediction or an unseen cause is a guess);
- runs every check it could run, so none is handed to the human;
- covers what the playbook's own Reply line names.
