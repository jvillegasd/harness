# How review works

Review gives a change a second pair of eyes before it reaches you. Other agents read the change without hearing why the author wrote it that way, so they judge the code and not the explanation. Tests show what the code does. Review looks for what the tests and the author missed.

The key rule: **only problems a run proves reach you.** Reviewers can raise anything they suspect, but the agent must reproduce each problem with a test or a script before it counts. A finding nobody could prove is dropped, and the report tells you how many were dropped. The author never gets to dismiss a finding by arguing with it.

## Pick how much review you want

Your review level decides when a review starts on its own and how many agents it may use. `medium` is the default. Only you can change it:

```sh
harness review level high    # off, medium, or high
```

| Level | Starts on its own for | Who reviews | Most agents per review |
| --- | --- | --- | --- |
| `off` | Nothing | Nobody, unless you ask (then it runs as `medium`) | 0 unless you ask |
| `medium` | Every PR (or `build` pull-request branch) beyond docs or tests, and local work that is critical or names a security, data-loss, compatibility, or concurrency risk | One general reviewer | 2 |
| `high` | Every PR or local change beyond docs, tests, and mechanical edits | The general reviewer plus one or two specialists | 6 |

The kinds of change ("critical", "docs or tests", "mechanical") are the review tiers that `change-map` assigns to each changed file.

Asking for a review always runs one, at your level (or at `medium` when the level is `off`). It never quietly upgrades to `high`. Under `/jvillegasd-mode`, every playbook that changes code calls review before it hands off, so you do not need to ask. The table still decides whether it runs and how deep it goes.

```mermaid
stateDiagram-v2
  state "No review" as Skipped
  state "Medium: one general reviewer" as Medium
  state "High: general reviewer + specialists" as High

  [*] --> Skipped: Level off, or change too small
  [*] --> Medium: Level medium, or you ask while off
  [*] --> High: Level high
  Medium --> Reviewed
  High --> Reviewed
  Skipped --> [*]
  Reviewed --> [*]
```

## Review something other than your branch

Ask for a review of a PR number or URL, another branch, or a commit range such as `abc123..def456`, and you don't need it checked out. The agent fetches it, gives the reviewers a temporary copy of it, and deletes that copy when the review ends. Such a review fixes nothing and pushes nothing, and you send the findings to the PR's branch with `/address-review`. When the target has an open PR, the review posts its verdict there like any other, with the confirmed findings and how each was proven: asking for the review is asking for the verdict. It never approves the PR or requests changes on it; that stays with you. A PR from a fork is reviewed without running any of its code, so only findings that need no run can be proven.

## Who reviews

The **general reviewer** checks correctness, requirements, error handling, compatibility, relevant security, project conventions, and missing tests.

At `high`, a **specialist** digs deeper into one area where the change needs it:

- **Requirements:** does the change do exactly what the ticket or spec asked, no more and no less?
- **Risk:** trust boundaries, data loss, failing dependencies, or concurrency.

Each kind of reviewer is its own role: `review-bugs`, `review-spec`, `review-risk`, and `reviewer` for rechecking a fix. They all run at the tier and effort you chose for `reviewer` with `harness models` (`high` at `low` effort unless you change it). To give one of them its own tier, set it directly:

```sh
harness models role set review-risk --tier heavy --effort high
```

## What happens during a review

1. **Freeze the change.** The agent commits its work, and reviewers read exactly that commit. Nothing is edited until every reviewer has reported.
2. **Hunt.** Reviewers get the requirements, copied word for word, and the diff. They read the project's standards and principles themselves. They never see the conversation or the author's reasoning.
3. **Prove each finding.** The agent tries to reproduce every problem with a small test or script. At `high`, a separate agent can run what the main agent could not. Two instructions that contradict each other are proven by quoting both. Everything else is dropped and counted.
4. **Fix and check.** Each confirmed problem is fixed together with a test that failed before the fix and passes after it. For contradictory instructions, the fix rewords them so they agree. At `high`, one more reviewer reads the fix to make sure it solves the root cause and breaks nothing else.

There is only ever one round of fixes. If a fix does not hold, the agent brings it to you instead of looping.

## What happens to a finding

```mermaid
stateDiagram-v2
  state "Reported" as Reported
  state "Confirmed" as Confirmed
  state "Dropped" as Dropped
  state "Fixed" as Fixed
  state "Fix rechecked" as Rechecked
  state "Needs your decision" as Decision
  state "Noted only" as Advisory

  [*] --> Reported
  Reported --> Advisory: Style nit, or an older problem
  Reported --> Confirmed: A test reproduces it, or two rules conflict
  Reported --> Dropped: Nothing proved it
  Confirmed --> Fixed: Fix passes its test
  Fixed --> [*]: Medium, the test is the proof
  Fixed --> Rechecked: High, a reviewer checks the fix
  Fixed --> Decision: The recheck finds a real problem
  Rechecked --> [*]
  Dropped --> [*]
  Advisory --> [*]
  Decision --> [*]
```

### Why unproven findings are dropped

Dropping unproven findings means some real bugs slip through. In exchange, every finding you read has been run, so you can trust the report.

## What you get at the end

- The level used, which reviewers ran and why, and how many agents it took.
- The exact commit that was reviewed, and which principles shaped review decisions, with the specific choice each changed.
- The proven findings, grouped by reviewer and worst first within each group, each marked confirmed or fixed.
- How many findings each reviewer had dropped, and why.
- Anything the review could not cover, such as a reviewer that failed or a part of the change too large to fit.

## Where results live

The result is one verdict comment on the PR, posted with `harness review verdict`, and nothing is kept on your machine. Like a verifier's verdict, it reads `PASS`, `PASS+NOTES` (it may land, and the notes say what to read), or `FAIL` (a proven problem is unfixed, or part of the change went unreviewed). Below the verdict come the reviewed range, the level, the proven findings with how each was proven, and how many findings each reviewer dropped. A review of work that has no PR yet posts nothing; `open-pr` opens the PR first, then runs the review that posts it.

`/land` and the readiness report read the newest verdict from someone with write access to the repository, and ignore any other. They compare the commit it read with the PR's head file by file, so a rebase that changes no file's patch keeps it. When a skill calls for a review and the PR already has a current `PASS` or `PASS+NOTES` verdict at that level or deeper, the agent reports it and does not run a second one. A review you ask for by name always runs.

At `medium` and `high`:

- The PR's readiness report says whether a verdict covers its latest commit.
- `/land merge` refuses to merge a PR beyond docs or tests until one does. A GitHub approval does not count as a review, for anyone's PR.
- A `FAIL` verdict holds `/land merge` at any level until a newer verdict replaces it, or until you accept it by name.
- `/land` after your own merge tells you when no verdict covered it.

A review reads the change. Running it as a user is a separate check with its own verdict comment, which `/land merge` also requires for a change you can observe; [the workflow guide](workflow.md) covers it.

Review never pushes or merges on its own.

## Limits

- **Coverage gaps are reported, not hidden.** Whatever a review could not cover is reported as unreviewed.
- **A verdict covers only the change it read.** New commits that change a file's patch need the affected parts rechecked.
- **This page explains, it does not prove.** The [review skill](../skills/delivery/review/SKILL.md) is the exact procedure agents follow. The diagrams here describe it, but they do not prove how agents behave in practice.
