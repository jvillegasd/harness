# Delegation

Sometimes the main agent hands a piece of work to a helper agent, called a role. This page explains which roles exist, how they are launched, and what limits them.

## When the agent delegates

The main conversation's context is its working memory, and reasoning gets worse as it fills. So the main agent leads: it plans, writes briefs, reviews, and verifies, and it hands bulk work to roles, keeping only their short results. It delegates for one of three reasons:

- **Protecting the context:** a search across many files, a long log or trace, a broad mechanical sweep, or an implementation big enough to bury the plan. The main agent reads the diff or the finding instead of producing it.
- **Independent inspection:** a fresh agent that has not seen the author's reasoning, such as a reviewer.
- **Useful parallel work:** separate pieces that can run at the same time.

Small edits stay with the main agent, because writing the brief would cost more context than the work. Briefs point at files instead of pasting them, and roles report a conclusion with file and line pointers, not raw output.

## The roles

| Role | Job | Limits |
| --- | --- | --- |
| `executor` | Implement one bounded piece of work | No commits, publishing, tracker writes, merges, worktree removal, or further delegation |
| `code-search` | Answer one question about the repository, with file and line citations | Read and search only |
| `worker` | Any other helper: a judge, a design proposal, a bulk read, a run a skill asks for | The same as `executor` |
| `reviewer` | Check that review fixes hold | Read and search only, and never sees the author's reasoning |
| `review-bugs`, `review-spec`, `review-risk` | Look for problems through one lens: general bugs, requirements, or risk | The same as `reviewer`, whose definition each lens builds on |

The definitions live in [roles/](../roles/) and in [the review skill](../skills/delivery/review/SKILL.md).

A role saying it finished is not proof. The main agent checks the role's scope, the Git state, and the results itself.

## How a role is launched

Agents launch roles through the `delegation` skill. Each role runs at a tier: `heavy` for system design, `high` for defects and analysis, `standard` for scoped work with tests, and `fast` for classification and bulk reads. The executor runs at `standard`, code search at `fast`, and the reviewer and its lenses at `high`. The `worker` has no tier of its own; the agent picks one for each launch from the work it hands over. To see or change which client and model serve each tier, or a role's tier and effort, run `harness models`. The review lenses use the `reviewer` setting until you set one of them on its own.

Where the role runs depends on its client:

- **Same client as your session:** the role runs as a native subagent.
- **A different client:** the role runs only inside a T3 Code session, as a delegated task with that client's model and effort. A T3 Code child does not load the installed role agent, so the role's definition is sent along in its brief.

`harness models` is the only place a role's model comes from. A role's agent is installed only on the client it is assigned to, so another client has no agent of that name to fall back to the session's model with. Claude Code refuses to launch an agent it does not have; whether Codex and OpenCode refuse too is unverified, so the delegation skill also forbids launching a role on a client it is not assigned to. A role whose model this machine cannot run gets no agent at all, and `harness install` warns. The `worker` is installed on every client, because its tier is chosen at each launch.

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
