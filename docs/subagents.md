# Delegation

Sometimes the main agent hands a piece of work to a helper agent, called a role. This page explains which roles exist, how they are launched, and what limits them.

## When the agent delegates

By default the agent does the work itself. It hands work to a role only for one of three reasons:

- **Independent inspection:** a fresh agent that has not seen the author's reasoning, such as a reviewer.
- **Useful parallel work:** separate pieces that can run at the same time.
- **Context isolation:** a large search or task that would crowd the main conversation.

Touching many files is not, on its own, a reason to delegate.

## The roles

| Role | Job | Limits |
| --- | --- | --- |
| `executor` | Implement one bounded piece of work | No commits, publishing, tracker writes, merges, worktree removal, or further delegation |
| `code-search` | Answer one question about the repository, with file and line citations | Read and search only |
| `reviewer` | Check that review fixes hold | Read and search only, and never sees the author's reasoning |
| `review-bugs`, `review-spec`, `review-risk` | Look for problems through one lens: general bugs, requirements, or risk | The same as `reviewer`, whose definition each lens builds on |

The definitions live in [roles/](../roles/) and in [the review skill](../skills/workflow/review/SKILL.md).

A role saying it finished is not proof. The main agent checks the role's scope, the Git state, and the results itself.

## How a role is launched

Agents launch roles through the `delegation` skill. Each role has an assigned client, model, and effort level. To see or change them, run `harness models`. The review lenses use the `reviewer` assignment until you set one of them on its own.

Where the role runs depends on its client:

- **Same client as your session:** the role runs as a native subagent.
- **A different client:** the role runs only inside a T3 Code session, as a delegated task with that client's model and effort. A T3 Code child does not load the installed role agent, so the role's definition is sent along in its brief.

If a role is missing or out of date on this machine, `harness install` sets it up. Agents never quietly swap in a different role. When a review needs an independent reviewer that cannot run, the review is reported as unverified. The author does not review its own work instead.

## Roles in a review

The review lenses, `reviewer`, and `executor` carry out reviews together. [How review works](review.md) explains what reviewers see, which findings reach you, and how fixes are checked.

## How strongly the limits are enforced

Each client enforces role limits differently:

| Client | How read-only roles are limited |
| --- | --- |
| Claude Code | Tool permissions, plus a shell guard that allows only read commands |
| OpenCode | No shell, and only its read and search tools |
| Codex | Native subagents inherit the parent session's sandbox |
| T3 Code delegate | Runs in the mode it inherits. T3 Code has no read-only mode, so the limits are instructions in the brief |

Some `executor` limits are instructions only, in every client.

Treat these as best-effort controls. The repository's tests check the generated configuration, not whether a live client obeys it. Runtime enforcement stays unverified until an approved test runs against the installed client.
