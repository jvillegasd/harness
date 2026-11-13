---
name: delegation
description: "Trigger: launch a role (executor, code-search, reviewer, a review lens) or hand work to a subagent. Resolve the role's model, launch it natively or through T3 Code, and check its result."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Launch a role with the model and effort this machine assigns it, and check what it returns.

## Why

Work inline by default. Delegate for a named benefit: independent inspection, useful parallel work, or context isolation, never because of file count or diff size. When an invoked skill's own method requires isolated subagents, that method is the named benefit: launch them natively as the skill describes, without mapping them to a role.

## Roles

| Role | For |
| --- | --- |
| `executor` | A bounded implementation slice |
| `code-search` | A cited answer about the repository |
| `reviewer` | A check of review fixes, and the shared text the lenses extend |
| `review-bugs`, `review-spec`, `review-risk` | An independent hunt through one lens |

## Launch

1. Run `harness models resolve <role> --json` and use the model and effort it returns.
2. When the role is assigned to this session's client, launch the installed agent natively: Claude Code's Agent tool, a Codex subagent spawned by the agent's name, or OpenCode's `task`.
3. Otherwise, in a T3 Code session, start it with T3 Code's `delegate_task`:
   - `target.providerInstanceId` is `claudeAgent`, `codex`, or `opencode` for the role's client.
   - `target.model` is the resolved model unless it is `inherit`, taking the matching id `orchestrator_capabilities` lists for an alias such as `opus`.
   - The effort goes in that provider's option: `effort`, `reasoningEffort`, or `variant`.
   - An executor that needs its own checkout gets `isolation: "worktree"`, which starts from the parent's commit, so commit what the child must see first.
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
