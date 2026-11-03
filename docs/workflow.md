# Working with the harness

Start with a plain request. The agent inspects the repository, implements the agreed scope, checks it, and reports. You do not need a ticket, specification, or ceremony for ordinary work.

## Choose help when you need it

| Situation | Skill |
| --- | --- |
| A fuzzy idea needs decisions | `/grill-with-docs` |
| A ticket needs investigation before implementation | `/triage` |
| A question needs evidence | `research` |
| An experiment would settle an uncertainty | `proto` |
| A design needs a durable specification | `/spec` |
| A bug needs reproduction and diagnosis | `diagnosing-bugs` |
| You want test-first implementation | `tdd` |
| A frontend needs design work | `impeccable` |
| A state machine needs model checking | `/statechart` |
| A release needs browser QA | `/e2e-verification` |
| A session needs a retrospective | `/retro` |

These are independent tools. Using one does not require or automatically start another. A slash marks an explicit invocation here; consult each skill for its inputs.

## Local work and delivery

An implementation request authorizes local edits and commits on a task branch the agent creates. Consequential decisions come back to you; routine reversible choices do not need repeated approval.

“Open a PR” or “ship this” authorizes commits, push, PR creation or updates, and required-check repairs. Feedback stays on the same branch. Publication does not authorize merging: invoke `/land` for that.

Before handoff, the agent runs the repository checks and explains what changed, what to review, and any gaps. Review depth follows your `harness review level` setting. At `medium` and `high`, landing a PR above `tests-docs` requires review of its current patch.

The [README state diagram](../README.md#everyday-use) shows the delivery path:

- **Ready locally:** the requested local work and applicable checks are complete. No publication is required.
- **Preparing PR:** publication is authorized. The agent completes local checks and applicable review before publishing.
- **PR open:** remote checks and feedback may require more changes on the same branch. Changed code needs fresh checks and any applicable review.
- **Ready to merge:** required checks and review are satisfied. The agent waits for your `/land`, then rechecks the gates before merging.
- **Needs your input:** a consequential decision or unavailable required check or reviewer blocks progress. Once resolved, work returns to the blocked stage. A failed check the agent can repair stays in the normal work loop.

These describe the intended workflow, not a centrally enforced task status. Live agent compliance is separate from the written policy.

See the [finding lifecycle](subagents.md#finding-lifecycle) for how review concerns are confirmed, refuted, repaired, and validated.

For longer work, the agent keeps requirements, decisions, evidence, and the next action in `docs/agents/tasks/<feature>.md`. Small tasks need no extra document.

## Larger builds

Invoke `/build` to implement a specification's slices or a set of tickets unattended. It uses one integration branch and bounded workers in separate worktrees. The parent verifies and commits their results, then checks and reviews the combined change. Publication still requires a delivery request; merging still requires `/land`.

Use this when the work is settled enough to delegate. Missing acceptance criteria, disputed test seams, or shared assumptions need resolving first.

## Optional tracking

Invoke `setup-harness` to configure a repository's tracker. GitHub mode installs labels and a lifecycle workflow; local mode stores status in Markdown. Ordinary coding needs neither.

GitHub dependencies become ready only when their blockers were closed by merged PRs. Closing an issue manually does not count. A parent closes when all required children have merge evidence. Tracker writes and ticket publication require an explicit request.

### GitHub ticket lifecycle

This tracks one actionable ticket enrolled in the lifecycle workflow, using the default label names. Repositories can configure different names. **Main agent** means your primary session. **Reconciler** means the installed GitHub workflow, not a subagent. A ready label does not authorize work to start.

```mermaid
stateDiagram-v2
    state "blocked" as Blocked
    state "ready-for-agent" as Ready
    state "in-progress" as Active
    state "ready-for-human" as Handoff
    state "Closed with merge evidence" as Closed

    Blocked --> Ready: Reconciler confirms all blockers have merge evidence
    Ready --> Active: Main agent claims ticket with your authorization
    Active --> Handoff: Main agent hands off PR with authorized tracker write
    Handoff --> Closed: You invoke /land and main agent confirms merge and ticket closure
    Ready --> Blocked: Reconciler finds an unsatisfied dependency
    Active --> Blocked: Reconciler finds an unsatisfied dependency
    Handoff --> Blocked: Reconciler finds an unsatisfied dependency
    Closed --> [*]
```

- **ready-for-agent:** all blockers have merge evidence, or there are no blockers. The main agent rechecks the claim before taking it and must not overwrite another run's claim.
- **ready-for-human:** the PR is ready for human review, with required checks and applicable review satisfied. PR feedback stays on the existing branch. The label does not mean a human must implement the ticket.
- **blocked:** a blocker lacks merge evidence, including when its repository cannot be read. Reconciliation can replace an active label with this label. Otherwise it preserves active claims and handoffs.
- **Closed with merge evidence:** a merged PR closed the ticket. Manual closure alone does not release dependents. `/land` checks tracker reconciliation and applies scoped repairs if the workflow did not run.

A parent issue is a container, not an actionable ticket in this diagram. It closes only after every required child has merge evidence. Reopening and `/abandon` release are recovery paths, outside this delivery view. The diagram describes the written workflow and reconciler rules, not proof of live agent compliance.

## When work stops

The agent reports contradictory evidence, scope changes, or repeated failed attempts instead of guessing indefinitely. The report should say what failed and what decision or evidence is needed next.

To discard unmerged work, invoke `/abandon` and approve its named inventory. Unrelated work and user worktrees must survive.

### Resume retained work

Resuming assumes the implementation still exists. The main agent uses the request, current Git state, and any existing task document or memory mirror. A small task needs no recovery document.

```mermaid
stateDiagram-v2
    state "Retained work" as Retained
    state "Context and Git inspected" as Inspected
    state "Needs your input" as Waiting
    state "Next action established" as Next
    state "Current evidence checked" as Checked
    state "Work resumed" as Resumed

    [*] --> Retained: You ask to resume
    Retained --> Inspected: Main agent reads context and inspects branch and working changes
    Inspected --> Waiting: Consequential context is missing or conflicting
    Waiting --> Inspected: You resolve the gap
    Inspected --> Next: Main agent reconciles requirements with current Git state
    Next --> Checked: Main agent rechecks stale evidence needed for the next action
    Checked --> Resumed: Main agent continues within existing authorization
```

Missing context blocks only dependent work. Old notes grant no new authority, and checks against a changed or unidentified snapshot are stale. See [continuity](../skills/support/continuity/SKILL.md) for recovery details.

### Abandon an attempt

Only your explicit `/abandon` starts disposal. First choose its scope: **dispose attempt, keep requirement** (the default), or **dispose attempt and cancel requirement**. Without a target, it scopes to the current task attempt, not an unrelated stranded branch. An **inventory** lists the exact commits, branch tips, uncommitted changes, task worktree, review records, and tracker changes proposed for discard or modification. Approval covers that list only.

```mermaid
stateDiagram-v2
    state "Disposal scope selected" as Scope
    state "Target inspected" as Target
    state "Stop and keep live claim" as Open
    state "Use /land for merged work" as Merged
    state "Discard and tracker changes listed" as Inventory
    state "Needs your decision" as Waiting
    state "Inventory approved" as Approved
    state "Tracker reconciliation" as Reconciling
    state "Tracking reconciled or not applicable" as Reconciled
    state "Approved resources discarded" as Discarded
    state "Requirement retained for a new attempt" as Retry
    state "Requirement explicitly cancelled" as Cancelled

    [*] --> Scope: You invoke /abandon keeping requirement by default or explicitly cancelling it
    Scope --> Target: Main agent inspects selected task attempt
    Target --> Open: PR is open
    Target --> Merged: PR is merged
    Target --> Inventory: PR is closed unmerged or no PR exists
    Inventory --> Waiting: Cancellation is unsupported or retained scope still needs a decision
    Waiting --> Inventory: You resolve scope or cancellation representation
    Inventory --> Approved: You approve exact discard and tracker changes
    Approved --> Inventory: Main agent finds changed inventory or tracker scope
    Approved --> Reconciling: Main agent applies approved tracker changes
    Reconciling --> Reconciled: Main agent verifies tracker state or skips trackerless work
    Reconciling --> Waiting: Tracker writes failed or final state is unresolved
    Reconciled --> Inventory: Final check finds changed branch tips or discard contents
    Reconciled --> Discarded: Main agent preserves recovery notes and removes only approved resources
    Discarded --> Retry: Requirement is still needed
    Discarded --> Cancelled: You explicitly cancelled the requirement
```

Tracker reconciliation must finish before disposal. On partial failure, the main agent reports completed and remaining changes and resumes from the actual state. Reopening a closed ticket requires explicit approval. Released open tickets become `blocked` or `ready-for-agent` according to their dependencies.

GitHub cancellation records the approved scope change and closes the ticket as `not planned`, without treating cancellation as merge evidence. Local Markdown tracking has no cancellation representation, so cancellation stops before writes until that decision is settled.

Disposal can delete the approved task worktree and local branch. A remote branch is deleted only when its PR remains closed unmerged and its tip matches the approved inventory. A remote branch without a PR stays. User worktrees and unrelated work remain protected.

**Retry is a new attempt.** Preserved notes retain context, not a guarantee that discarded code can be recovered. See [abandon](../skills/ops/abandon/SKILL.md) for the full safeguards.
