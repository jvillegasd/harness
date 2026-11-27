---
name: delegation
description: "Trigger: launch a role (executor, code-search, worker, reviewer, a review lens) or hand work to any subagent. Resolve the role's tier and model, launch it natively or through T3 Code, and check its result."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Launch a role at the tier and effort this machine assigns it, and check what it returns. Every subagent is a role.

## Why

The session's context is the run's working memory: it holds the goal, the plan, and the judgment that checks every result, and it degrades as it fills (`principle-guard-the-context-window`, read it in full). So the session leads, planning, briefing, reviewing, and verifying, and routes bulk to roles, keeping only their reduced results. Delegate for one of these benefits, and name it in the brief:

- **Context:** work whose raw material would crowd the session. A search or read across many files, a long log, transcript, trace, or profile, a broad sweep of mechanical edits, or an implementation large enough that typing it would bury the plan. The session reviews the diff or the finding rather than producing it.
- **Independent inspection:** a fresh agent without the author's reasoning, such as a reviewer.
- **Parallel work:** independent pieces that can run at once, each with its own output.

Keep work inline when the brief would cost more context than the work: a small edit, a read of the file you are about to change, or a step whose reasoning the session needs to hold. When an invoked skill's own method requires isolated subagents, that method is the named benefit: launch each one as the `worker` at the tier its work fits, giving it the brief the skill describes.

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
| `heavy` | System design, architecture | opus, or fable when the human maps it | newest `-astra` |
| `high` | Defects, engineering, analysis, judging; design when no heavy model is listed | opus | newest `-sol` |
| `standard` | Scoped work with tests | sonnet | newest `-terra` |
| `fast` | Classification, gathering data in volume, boilerplate | haiku | newest `-luna` |

Choose the `worker`'s tier by this table, from the work the brief asks for, not from the skill that asked. When a skill names a model family, such as a judge on a family no candidate used, pass the matching client.

A skill that ships its own named agents, such as `impeccable`'s reviewers, launches those agents as it describes; they are not roles.

## Launch

1. Resolve before every launch (`principle-launch-on-the-assigned-model`): run `harness models role resolve <role> --json`, and for the `worker` add `--tier <tier>`, plus `--client <client>` when the brief needs a model family. Use the tier, client, model, and effort it returns, and say the `note` when it has one (a tier that fell back, an effort the model does not take). A non-zero exit means the role has no runnable model here: say so.
2. When the role's client is this session's client, launch the installed agent natively: Claude Code's Agent tool, a Codex subagent spawned by the agent's name, or OpenCode's `task`. Pass a model at launch only for the `worker` in Claude Code, as the resolved alias (`fable`, `opus`, `sonnet`, `haiku`), since it overrides the agent's own; the other roles run on the model installed with them. The Agent tool takes no effort and no full model id, so a `worker` whose resolved effort or `claude-` model differs from its installed agent goes through step 3. A Codex or OpenCode `worker` launched natively runs at its installed tier and effort, so a different one goes through step 3 too. Without T3 Code, launch at what is installed and report what was not applied.
3. Otherwise, in a T3 Code session, start it with T3 Code's `delegate_task`:
   - `target.providerInstanceId` is `claudeAgent`, `codex`, or `opencode` for the role's client.
   - `target.model` is the resolved model, taking the matching id `orchestrator_capabilities` lists for an alias such as `opus`.
   - The effort goes in that provider's option: `effort`, `reasoningEffort`, or `variant`.
   - An executor or worker that needs its own checkout gets `isolation: "worktree"`, which starts from the parent's commit, so commit what the child must see first.
   - The child loads no installed role agent, so open its brief with the role's installed definition (such as `~/.claude/agents/<role>.md`) before the task. Its access limits are then instructions only.
   - Wait for the completion wake-up instead of polling, and `task_cancel` a delegate you no longer need.
4. Outside T3 Code, a role assigned to another client cannot launch. When a role cannot launch, say so and work inline where independence is not required. A missing or stale agent needs deliberate provisioning with `harness install`, not a silent substitute.

## Brief

Write each brief to stand alone: open with the goal, then the scope it may touch, the base commit, how to verify it, and what to report. Point at files, tickets, and documents by path or link rather than pasting them, so the brief stays small and the child reads only what it needs. Ask for a reduced result: the conclusion, the evidence as `path:line` pointers or commands with their outcome, and the gaps, never a raw dump of logs, files, or search hits. A delegated executor never commits; the parent commits its result.

Give new work, a fix round, or a retry to a fresh agent whose brief consolidates the original brief, every later directive, and the prior agent's result and branch. Resume or message an existing agent only when the work needs state that lives in it and is costly to move: its checkout, its uncommitted changes, or a process it still runs. A resumed agent can drop directives it was given earlier, so its "done" is checked like any result.

## Results

Check each result against its assigned scope and the actual Git state; a completion message alone is not evidence. Check it by the pointers it gives, reading the diff and the cited lines and rerunning its verification commands rather than redoing the task here, then keep your own short summary in the session and let the raw result go. Keep complete results before releasing disposable resources when recovery may need them.

## Output

Each role launched, the benefit it was launched for, where it ran, its result checked against scope, and any role that could not launch.
