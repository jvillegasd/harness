---
name: delegation
description: "Trigger: launch a role (executor, code-search, worker, reviewer, a review lens) or hand work to any subagent. Resolve the role's tier and model, launch it natively or through T3 Code, and check its result."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Launch a role at the tier and effort this machine assigns it, and check what it returns. Every subagent is a role.

## Why

Work inline by default. Delegate for a named benefit: independent inspection, useful parallel work, or context isolation, never because of file count or diff size. When an invoked skill's own method requires isolated subagents, that method is the named benefit: launch each one as the `worker` at the tier its work fits, giving it the brief the skill describes.

## Roles

| Role | For | Tier |
| --- | --- | --- |
| `executor` | A bounded implementation slice | standard |
| `code-search` | A cited answer about the repository | fast |
| `worker` | Any subagent no other role describes, including those a skill asks for without naming a role | chosen at launch |
| `reviewer` | A check of review fixes, and the shared text the lenses extend | high |
| `review-bugs`, `review-spec`, `review-risk` | An independent hunt through one lens | the reviewer's |

A tier names the kind of work a model is meant for. `harness models` maps each to a client and model, and the human changes it there:

| Tier | Work | Claude | Codex |
| --- | --- | --- | --- |
| `heavy` | System design, architecture | fable, unless the human maps it to another model | newest `-astra` |
| `high` | Defects, engineering, analysis, judging; design when no heavy model is listed | opus | newest `-sol` |
| `standard` | Scoped work with tests | sonnet | newest `-terra` |
| `fast` | Classification, gathering data in volume, boilerplate | haiku | newest `-luna` |

Choose the `worker`'s tier by this table, from the work the brief asks for, not from the skill that asked. When a skill names a model family, such as a judge on a family no candidate used, pass the matching client.

A skill that ships its own named agents, such as `impeccable`'s reviewers, launches those agents as it describes; they are not roles.

## Launch

1. Run `harness models resolve <role> --json`, and for the `worker` add `--tier <tier>`, plus `--client <client>` when the brief needs a model family. Use the tier, client, model, and effort it returns, and say the `note` when it has one (a tier that fell back, an effort the model does not take). A non-zero exit means the role has no runnable model here: say so.
2. When the role's client is this session's client, launch the installed agent natively: Claude Code's Agent tool, a Codex subagent spawned by the agent's name, or OpenCode's `task`. Pass a model at launch only for the `worker` in Claude Code, as the resolved model, which is one of the aliases the tool takes (`fable`, `opus`, `sonnet`, `haiku`) unless the human named a full id, since it overrides the agent's own; the other roles run on the model installed with them. The Agent tool takes no effort and no full model id, so a `worker` whose resolved effort or `claude-` model differs from its installed agent goes through step 3. A Codex or OpenCode `worker` launched natively runs at its installed tier and effort, so a different one goes through step 3 too. Without T3 Code, launch at what is installed and report what was not applied.
3. Otherwise, in a T3 Code session, start it with T3 Code's `delegate_task`:
   - `target.providerInstanceId` is `claudeAgent`, `codex`, or `opencode` for the role's client.
   - `target.model` is the resolved model, taking the matching id `orchestrator_capabilities` lists for an alias such as `opus`.
   - The effort goes in that provider's option: `effort`, `reasoningEffort`, or `variant`.
   - An executor or worker that needs its own checkout gets `isolation: "worktree"`, which starts from the parent's commit, so commit what the child must see first.
   - The child loads no installed role agent, so open its brief with the role's installed definition (such as `~/.claude/agents/<role>.md`) before the task. Its access limits are then instructions only.
   - Wait for the completion wake-up instead of polling, and `task_cancel` a delegate you no longer need.
4. Outside T3 Code, a role assigned to another client cannot launch. When a role cannot launch, say so and work inline where independence is not required. A missing or stale agent needs deliberate provisioning with `harness install`, not a silent substitute.

## Brief

Write each brief to stand alone: the goal, the scope it may touch, the base commit, how to verify it, and what to report. A delegated executor never commits; the parent commits its result.

Give new work, a fix round, or a retry to a fresh agent whose brief consolidates the original brief, every later directive, and the prior agent's result and branch. Resume or message an existing agent only when the work needs state that lives in it and is costly to move: its checkout, its uncommitted changes, or a process it still runs. A resumed agent can drop directives it was given earlier, so its "done" is checked like any result.

## Results

Check each result against its assigned scope and the actual Git state; a completion message alone is not evidence. Keep complete results before releasing disposable resources when recovery may need them.

## Output

Each role launched, where it ran, its result checked against scope, and any role that could not launch.
