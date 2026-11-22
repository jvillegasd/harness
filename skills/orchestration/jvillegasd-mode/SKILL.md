---
name: jvillegasd-mode
description: "Development mode: match a task to a playbook and run it to its steps, or on autopilot while you step away. Invoke only by name: \"/jvillegasd-mode fix the flaky login test\", \"/jvillegasd-mode autopilot, I'm stepping away: ship the export feature\"."
argument-hint: "[the task; add \"autopilot\" or \"I'm stepping away\" with a goal to run it unattended]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

In this mode the task is matched to one playbook, the playbook's steps go into the todo list, and the work runs to those steps. Only the human starts it, by typing `/jvillegasd-mode`, because starting it opts into this rigor.

Task: $ARGUMENTS

## Start

1. Restate the human's goal in a line, and the smallest change you expect to meet it. When the evidence rules out the change they asked for, show it and stop.
2. When the request grants a step-away ("autopilot", "I'm stepping away", "going to bed, keep going"), read [autopilot](playbooks/autopilot.md) now; it frames the run before anything else.
3. Size the work, then match it to one row of the matcher and read that playbook. A **task** is one pull request (a bug, a ticket, a small change). A **feature** is several slices of one behavior, built as an epic. A **project** is several features toward one product goal, such as "build an app that does X, Y, and Z". The rigor scales with the size; the lifecycle (understand, plan, build, verify, review, deliver, address feedback) does not change.
4. Copy every step of the playbook into the todo list verbatim, before any task-specific item. A step you will not take stays in the list as `skip: <reason>`, so the human sees what was left out and why.
5. When the work has a task document (`checkpoint`), record the playbook and step there as `checkpoint` describes, so a compaction resumes the same step.

jvillegasd-mode lasts the conversation. "New task" means match again from step 1, and "stop jvillegasd-mode" ends it.

## Matcher

| The task | Playbook |
| --- | --- |
| Something is broken, throwing, or failing | [bug fix](playbooks/bug-fix.md) |
| New or changed behavior | [feature](playbooks/feature.md) |
| A read-only question (how, why, are we sure, should we), or a ticket or production error to analyze with no fix requested | [investigate](playbooks/investigate.md) |
| The structure changes and the behavior does not | [refactor](playbooks/refactor.md) |
| Slow, with a number to move | [perf](playbooks/perf.md) |
| One metric to climb toward a target over many attempts | [hillclimb](playbooks/hillclimb.md) |
| A runtime symptom or a captured profile to diagnose, with no fix requested | [forensics](playbooks/forensics.md) |
| A shape or interface to choose, with no precedent | [design](playbooks/design.md) |
| Write or change a skill, an `AGENTS.md`, or a `CLAUDE.md` | [skill](playbooks/skill.md) |
| A prototype, spike, or mock-up | `proto`'s steps |
| A plan for a change, before anything is built, or tickets from research findings or a request | `epic`'s steps, then `build`'s, unless the human asked only for the plan |
| A settled epic or a set of slices to build | `build`'s steps |
| Review a diff, a branch, or a PR | `review`'s steps, at the depth the human asked |
| An existing PR: review comments, red checks, conflicts | `address-review`'s steps, with its mode declared |
| Open a PR, ship this | `open-pr`'s steps |
| Merge, or "it merged, clean up" | Reply "run `/land` in this worktree" and stop. Merging and cleanup belong to the human through `land` |
| Clean up old worktrees or branches | Reply "run `/cleanup`" and stop. The human names what a sweep removes |
| Resume or pick up earlier work | `checkpoint`'s Resume, then continue the playbook and step it recorded, or match what is left |
| A project: several features toward one product goal, or a whole app | [project](playbooks/project.md) |
| Large, multi-part, or none of the above | [you-got-this](playbooks/you-got-this.md) |

A bug that is a slowdown with a number is perf, and a one-off speedup is perf while a target reached over many measured attempts is hillclimb. A single ticket or a plain request to change behavior matches its kind's row (bug fix, feature, refactor), whose playbook runs `build` on one slice. When the task fits two rows, take the narrower one, and you-got-this only when no narrower row covers it.

A row that names a skill runs that skill's steps as the playbook: they go into the todo list like any playbook's, and jvillegasd-mode adds only its triggers and reply. `epic` and `build` are guarded so the model never starts them on its own; the human's `/jvillegasd-mode` request is what starts them, and it covers an epic continuing into `build`.

## Deciding and getting stuck

These replace every skill's asks and stops while the mode runs.

- **Each iteration** makes the smallest change the evidence justifies, verifies it, commits it when it advanced, and reverts it when it did not.
- **A decision a skill would ask the human about** (a test seam, a default, a naming or scope call) gets the simplest safe choice. Record it as an `agent` row in the task document's `## Decisions` table with the alternative it beat, list it first in the reply with "tell me X to switch", and keep going (`principle-never-block-on-the-human`). Settle a question a run can answer with `proto` instead of choosing.
- **A product or preference call no run can settle** is asked once, batched, with your recommendation, while the work that does not depend on it continues. Under autopilot it takes the default like any other decision.
- **Discoveries that support the goal** are yours: a broken skill, a related bug, a flaky check, a tooling failure. Fix each in its own pull request, then return to the goal. A problem that does not block the goal becomes a follow-up in the reply.
- **A plateau** means changing approach, not stopping.
- **Two fixes that share one premise** have failed the same check: test the premise directly before another fix assumes it (`principle-attack-the-premise`).
- **A failed unit** is retried by its cause: a smaller scope after a context or memory limit, the same start after a dropped connection, and a different model after a tool error. After two retries, drop the unit and replan around it.
- **A dead end** that survives the replan is parked with its evidence for the reply. Stop only when every remaining unit is parked, or when the evidence rules out the goal itself.
- **The definition of done** never loosens to declare victory.

## Triggers

These hold at every step of every playbook.

- **A broken skill mid-task** (a step that names a missing script, a stale path, two skills that contradict each other): fix it on its own branch. Do not block on it and do not silently work around it. It becomes its own PR. When the broken skill belongs to the harness and you are working in another repository, write a ready-to-apply note instead: the skill, the defect, the evidence, and the proposed fix, in the task document and the reply.
- **A step that verifies** passes only on the surface it names. Inconclusive, or green or measured on another surface, is not a pass.
- **A subagent to launch** (executor, code-search, reviewer, a review lens, or any other, which runs as the `worker` at a tier): launch it through `delegation`.
- **Bulk that would crowd the context** (a broad search, a long log, transcript, trace, or profile, a sweep of mechanical edits, an implementation big enough to bury the plan): route it to a role through `delegation` and keep only its reduced result, so this session stays on planning, review, and verification (`principle-guard-the-context-window`).
- **A principle applies** (the index below): apply its rule. Read the principle's `SKILL.md` from the installed skills (`~/.claude/skills` in Claude Code, `~/.agents/skills` in Codex and OpenCode) only when you need more than the rule, and name in the reply each principle that changed a decision.

### Principles

Each entry is when it applies, then the rule.

- `principle-laziness-protocol`: refactoring, sizing a diff, or tempted to add abstractions or layers. Bias to deletion and the smallest change that solves the problem.
- `principle-foundational-thinking`: before writing logic. Choose the core types, data structures, and what concurrent actors share first; scaffold before features.
- `principle-redesign-from-first-principles`: integrating a new requirement into an existing design. Redesign as if it had been there from day one instead of bolting it on.
- `principle-attack-the-premise`: two or more fixes that share one premise have failed the same check. Write the premise down and test it directly before another fix assumes it.
- `principle-subtract-before-you-add`: sequencing an addition, refactor, or rewrite. Remove dead weight first, then build on the simpler base.
- `principle-minimize-reader-load`: code that is hard to trace. Count layers and hidden state, collapse one-caller wrappers, shrink mutable scope.
- `principle-experience-first`: product, UX, or scope tradeoffs. Choose what the user experiences over implementation convenience.
- `principle-build-the-lever`: non-trivial work a tool could do or prove. Build the codemod, script, or generator instead of doing it by hand; the tool is what a reviewer reruns.
- `principle-model-the-domain`: stateful logic, or code that branches a lot or repeats a shape assumption. Encode the domain in a structure (state machine, typed model, table, reducer) instead of scattered conditionals.
- `principle-boundary-discipline`: validation, error handling, or framework adapters. Guard at system boundaries, trust internal types, keep business logic pure.
- `principle-type-system-discipline`: designing types or a signature in a typed language. Make illegal states unrepresentable and parse external data at the boundary.
- `principle-make-operations-idempotent`: commands, lifecycle steps, or loops that run amid crashes and retries. Every rerun converges to the same end state.
- `principle-migrate-callers-then-delete-legacy-apis`: a new internal API while old callers exist. Migrate every caller and delete the old API in the same wave.
- `principle-separate-before-serializing-shared-state`: concurrent actors that might write the same file, branch, key, or object. Eliminate the sharing first; serialize only for a real invariant.
- `principle-prove-it-works`: before declaring a task done. Verify against the real artifact on the surface the user touches, not a proxy or "it compiles".
- `principle-fix-root-causes`: debugging. Trace each symptom to the first place the system went wrong and fix it there; a guard is a fix only when the state it handles is legitimate.
- `principle-sequence-verifiable-units`: multi-step work, and how commits and pull requests stack. Small units that each end in a check, each verified before the next.
- `principle-test-behavior-not-implementation`: writing, changing, or keeping a test. Assert what callers observe, so the test fails when behavior breaks and passes when only internals change.
- `principle-explain-the-number`: before measuring, and before trusting or acting on a number. Find what limits it, and rule out that it measured something other than the work.
- `principle-guard-the-context-window`: large outputs, long files, repeated reads, or fan-out planning. Route bulk to roles and keep only their reduced results.
- `principle-never-block-on-the-human`: tempted to ask whether to do reversible work. Do it, show the result, and let the human correct it.
- `principle-encode-lessons-in-structure`: writing the same instruction a second time, or a recurring correction. Encode it as a check, lint, type, or script instead of more text.

## Finish

A playbook's Finish step is these three, each its own item in the todo list:

1. Run `change-map`.
2. Run `review` at the level `harness review level` reports.
3. Run `open-pr`, unless the human said to keep the work local.

## Reply

Every playbook ends with a reply that:

- opens with the human's goal;
- is written clean as you draft it, per `unslop` read from its path;
- carries, for each claim, its evidence or its label in the same sentence: measured, inferred, or guessed (a prediction or an unseen cause is a guess);
- runs every check it could run, so none is handed to the human;
- covers what the playbook's own Reply line names.
