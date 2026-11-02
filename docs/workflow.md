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

For longer work, the agent keeps requirements, decisions, evidence, and the next action in `docs/agents/tasks/<feature>.md`. Small tasks need no extra document.

## Larger builds

Invoke `/build` to implement a specification's slices or a set of tickets unattended. It uses one integration branch and bounded workers in separate worktrees. The parent verifies and commits their results, then checks and reviews the combined change. Publication still requires a delivery request; merging still requires `/land`.

Use this when the work is settled enough to delegate. Missing acceptance criteria, disputed test seams, or shared assumptions need resolving first.

## Optional tracking

Invoke `setup-harness` to configure a repository's tracker. GitHub mode installs labels and a lifecycle workflow; local mode stores status in Markdown. Ordinary coding needs neither.

GitHub dependencies become ready only when their blockers were closed by merged PRs. Closing an issue manually does not count. A parent closes when all required children have merge evidence. Tracker writes and ticket publication require an explicit request.

## When work stops

The agent reports contradictory evidence, scope changes, or repeated failed attempts instead of guessing indefinitely. The report should say what failed and what decision or evidence is needed next.

To discard unmerged work, invoke `/abandon` and approve its named inventory. Unrelated work and user worktrees must survive.
