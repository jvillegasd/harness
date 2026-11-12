---
name: implement
description: Implement an everyday feature, fix, or single ticket inline with TDD. Use for bounded implementation requests that do not need build's unattended worker orchestration.
argument-hint: "[task description or ticket]"
---

Implement one bounded task in the current session, using `tdd` for behavior changes and leaving the result checked and ready locally. A plain request or one ticket is enough; a specification is optional.

Task: $ARGUMENTS

## Steps

1. Read the request and relevant code; for a ticket, read its body and full comment thread. State the intended behavior and smallest change. Resolve routine details from repository conventions, and ask only about consequential gaps in scope or acceptance criteria.
2. Inspect Git state. Get on the task branch as the floor's Working rules describe, in a sibling worktree when separation helps, and work inline on that one branch. Use `ponytail` for coding decisions. Keep a task document only when recovery across an interruption warrants one.
3. Read and use `tdd`. Reuse test seams already agreed for this task; otherwise propose the relevant public boundary and settle seam decisions with the human before writing tests. For each behavior change, run one meaningful test and observe it fail for the intended reason, implement enough to pass, then move to the next behavior. Documentation and mechanical changes use appropriate checks rather than manufactured tests. If a meaningful red test cannot be run, report the gap and settle any reduced assurance before proceeding with the affected behavior.
4. Run the relevant checks and all repository-required checks, binding evidence to the committed revision or uncommitted content identity as `verification` describes. Commit owned work where authorized. Compare the result with the request; use `change-map` for a multi-file handoff and `review` only when its Level section calls for one.
5. Report the behavior delivered, commits, checks, and remaining gaps. Distinguish observed red and green results from checks that were unavailable. Local readiness completes this workflow; publication follows only an explicit delivery request.

For settled work that needs unattended orchestration across slices, suggest `/build` and let the human invoke it.
