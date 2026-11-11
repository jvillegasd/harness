# Autopilot

**You own the finish line.** The human has stepped away, so the run drives to its definition of done without checkpoints, inside a time ceiling, and stops at verified, open pull requests. It never merges.

## Frame

1. Take the goal from the human's request. When the request has none, ask for it and do not start: an unattended run with no goal has no finish line.
2. State the definition of done as a pass/fail check every iteration measures against (`principle-prove-it-works`). When the human gave none, propose one and start on it without waiting.
3. Set the ceiling: the human's ("autopilot, max 3h"), or 2 hours of wall-clock time.
4. Open the task document (`continuity`) and write the grant line into its `## State and next action`: the human's step-away words verbatim, and the time the ceiling ends. Commit it. After a compaction the client's hook restores that section, so the grant stays in view for the rest of this conversation.
5. Match the work to its playbook and run it, with the rules below in place of the playbook's checkpoints. A settled spec or a set of tickets is `build`: read it from its path and run it, with this grant standing in for the human's `/build` and its delivery request.

## What the grant covers

Everything up to verified, open pull requests: commits, pushes of the run's own branches, opening pull requests ready (never as drafts), pull requests for side fixes, and repairs of their checks. It never covers a merge, a tracker write, a message to a person, or anything the floor holds back. A pushed branch is never rewritten: a fix goes on the branch that owns the code, and a stacked child picks it up when `/land` repairs the child after its parent merges.

## Running

- **Each iteration** makes the smallest change the evidence justifies, verifies it against the definition of done, commits it when it advanced, and reverts it when it did not.
- **A decision only the human could make** gets the simplest safe default. Record it as an `agent` row in the `## Decisions` table with the alternative it beat, and keep going; asking would stall the run until the human returns.
- **Discoveries that support the goal** are yours: a broken skill, a related bug, a flaky check, a tooling failure. Fix each in its own pull request, then return to the goal. A problem that does not block the goal becomes a follow-up in the report.
- **A plateau** means changing approach, not stopping.
- **Two fixes that share one premise** have failed the same check: read `principle-attack-the-premise` and test the premise directly before another fix assumes it.
- **A failed unit** is retried by its cause: a smaller scope after a context or memory limit, the same start after a dropped connection, and a different model after a tool error. After two retries, drop the unit and replan around it.
- **A dead end** that survives the replan is parked with its evidence for the report.
- **The definition of done** never loosens to declare victory.

## Stopping

- **Done:** each pull request is verified and green at its current head, with `open-pr`'s readiness report. After the handoff, nothing is pushed without the human's go.
- **Ceiling reached:** pause with `continuity`'s Pause (a `wip:` commit and the next action), then report.
- **Goal ruled out:** when the evidence rules out the goal itself, as work mode's Start step 1 describes, stop and report that evidence, since there is nothing left to drive toward.

"Keep going" and "going to bed" never trigger a pause; only done, the ceiling, or a ruled-out goal ends the run.

**Reply:** the goal and the definition of done with its final state, each pull request with its readiness verdict, the `agent` decisions each with "tell me X to switch", the follow-ups, the parked dead ends with their evidence, and the time used against the ceiling.
