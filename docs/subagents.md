# Delegation

Work stays inline unless a worker offers independent inspection, useful parallel work, or context isolation. More files alone are not a reason to delegate.

## Roles

| Role | Job | Limits |
| --- | --- | --- |
| `executor` | Implement one bounded slice | No commits, publication, tracker writes, merges, worktree removal, or further delegation |
| `code-search` | Answer one repository question with file and line citations | Read and search only |
| `reviewer` | Find defects or validate fixes | Read and search only; does not receive the author's rationale |

Definitions live in [roles/](../roles/) and [the review skill](../skills/workflow/review/SKILL.md). A worker's completion message is not proof: the parent checks its scope, Git state, and results.

## Launching a role

Each role is assigned a client, model, and effort; `harness models` shows and changes the assignments. A role assigned to the session's own client runs as a native subagent. A role assigned to another client runs only inside a T3 Code session, as a delegated task with that client's model and effort. A T3 Code child loads no installed role agent, so the role's definition travels in its brief.

If a role is missing or stale on this machine, `harness install` provisions it; agents never quietly substitute another one. When a review needs an independent reviewer that cannot run, the review is reported as unverified rather than done by the author.

## Reviews

The `reviewer` and `executor` roles carry out reviews. [How review works](review.md) explains what reviewers see, which findings reach you, and how fixes are checked.

## Enforcement limits

Client restrictions differ. Claude Code restricts read-only roles with tool permissions and a shell guard that allows only read commands; OpenCode denies them the shell and allows only its read and search tools. Some executor limits remain instructions. Native Codex subagents inherit the parent's sandbox. A T3 Code delegate runs in the runtime mode it inherits, and T3 Code has no read-only mode, so a delegated role's access limits are instructions in its brief.

These are best-effort controls. Repository tests validate generated configuration, not live compliance. Runtime enforcement remains unverified until an authorized probe runs against the installed client.
