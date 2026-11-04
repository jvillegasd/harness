# Delegation

Work stays inline unless a worker offers independent inspection, useful parallel work, or context isolation. More files alone are not a reason to delegate.

## Roles

| Role | Job | Limits |
| --- | --- | --- |
| `executor` | Implement one bounded slice | No commits, publication, tracker writes, merges, worktree removal, or further delegation |
| `code-search` | Answer one repository question with file and line citations | Read and search only |
| `reviewer` | Find defects, challenge findings, or validate fixes | Read and search only; does not receive the author's rationale |

Definitions live in [roles/](../roles/) and [the review skill](../skills/workflow/review/SKILL.md). A worker's completion message is not proof: the parent checks its scope, Git state, and results.

## Launching a role

Resolve the assignment with `harness models resolve <role> --json`. A role assigned to the session's client uses a native subagent. A role assigned to another client uses Orca and its orchestration guide, with arguments prepared by `harness models launch-args`.

A missing or stale installed role needs deliberate `harness install` provisioning. Do not silently substitute another agent. If independence is required and the reviewer cannot run, report the review as unverified.

Use `harness models --help` for presets and overrides. The parent retains complete worker results before releasing disposable resources. Releasing a worker does not authorize removing its worktree.

## Reviews

Reviewers receive requirements, a revision identity, the scoped diff, and project standards. They do not receive implementation rationale, conversation history, or continuity notes.

At `medium`, one general reviewer hunts, and a refuter runs only for bugs the main agent could not reproduce. At `high`, one or two specialists (requirements, consequential risk, or structure and conventions) join the general reviewer, an `executor` may reproduce findings, and fixes get one recheck. An explicit Deep review adds a second general reviewer and keeps the union of both reports. The [review skill](../skills/workflow/review/SKILL.md) defines the exact worker counts and procedure.

The author may reproduce a finding but cannot dismiss it alone. Confirmed findings need fixes; unresolved findings go to the human. Findings stay in a local per-branch ledger inside the Git directory, not in commits or PR bodies.

## Finding lifecycle

This follows one actionable finding through the review skill. The subagent roles are `reviewer` and `executor`. **Main agent** means your primary session, called the parent in the skill. Hunting, refutation, and the fix recheck use separate `reviewer` instances, not additional roles. The main agent can repair findings itself or delegate repairs to an `executor`, but only the main agent commits. Findings are deduplicated first. Nits and pre-existing defects are reported separately.

```mermaid
stateDiagram-v2
    state "Finding reported" as Reported
    state "Main agent attempts confirmation" as Confirmation
    state "executor attempts reproduction" as Reproduction
    state "reviewer attempts refutation" as Refutation
    state "Unverified" as Unverified
    state "Confirmed" as Confirmed
    state "Refuted" as Refuted
    state "Fixed" as Fixed
    state "Fix rechecked" as Rechecked
    state "Needs your decision" as Decision

    [*] --> Reported: reviewer hunts for defects
    Reported --> Confirmation: Main agent checks the finding
    Confirmation --> Confirmed: Defect reproduced or prose contradiction cited
    Confirmation --> Reproduction: Needs a run, at high or Deep
    Confirmation --> Refutation: Unreproduced bug
    Confirmation --> Unverified: Unreproduced risk or question
    Reproduction --> Confirmed: executor reproduces the defect
    Reproduction --> Refutation: Bug not reproduced or unable to run
    Reproduction --> Unverified: Risk or question not reproduced
    Refutation --> Refuted: reviewer cites where failure path is prevented
    Refutation --> Unverified: Evidence remains inconclusive
    Unverified --> Decision: Main agent brings finding to you
    Confirmed --> Fixed: Main agent commits repair with the check passing
    Fixed --> [*]: At medium, the passing check is the proof
    Fixed --> Rechecked: At high or Deep, reviewer says the fix holds
    Fixed --> Decision: reviewer rejects fix or finds a regression
    Rechecked --> [*]
    Refuted --> [*]
    Decision --> [*]
```

The main agent tries to reproduce every finding. At `high` and Deep, an `executor` also runs findings the main agent could not reproduce inline. Code from fork PRs is not run during review. Prose contradictions can be confirmed by citing the conflicting instructions. Only a bug goes to the refuter, and refutation requires both a failed reproduction and a `reviewer`'s cited explanation of where its failure path is prevented. An unreproduced risk or question goes to you, because a missed defect costs more than a question.

Inconclusive evidence, failed rechecks, and new defects found in a recheck are recorded as `unverified` and brought to you. The main agent does not start a second repair loop on its own. A changed reviewed revision needs its affected evidence rechecked.

## Enforcement limits

Client restrictions differ. Claude Code and OpenCode restrict read-only roles with tool permissions and shell guards. Some executor limits remain instructions. Codex workers launched through Orca use a workspace-write sandbox with network access; it blocks commits through read-only Git metadata, but read-only role behavior and publication limits still depend on instructions. Native Codex subagents inherit the parent's sandbox.

These are best-effort controls. Repository tests validate generated configuration, not live compliance. Runtime enforcement remains unverified until an authorized probe runs against the installed client.
