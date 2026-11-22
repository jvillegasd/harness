# Autopilot

**You own the finish line.** The human has stepped away, so the run drives to its definition of done without checkpoints and without a time limit, and stops at verified, open pull requests. It never merges.

## Frame

1. Take the goal from the human's request. When the request has none, ask for it and do not start: an unattended run with no goal has no finish line.
2. State the definition of done as a pass/fail check every iteration measures against (`principle-prove-it-works`). When the human gave none, propose one and start on it without waiting.
3. Open the task document (`checkpoint`) and write the grant line into its `## State and next action`: the human's step-away words verbatim. Commit it. After a compaction the client's hook restores that section, so the grant stays in view for the rest of this conversation.
4. Match the work to its row and run it, with the rules below in place of its checkpoints. The grant stands in for the invocation of any guarded skill the row runs, `build` included.

## What the grant covers

The floor already delivers the run's work: commits, pushes of the run's own branches, opening pull requests ready (never as drafts), pull requests for side fixes, and repairs of their checks. The grant adds the decisions the human would make, taken as `agent` defaults below. It never covers a merge, a tracker write, a message to a person, or anything the floor holds back. A pushed branch is never rewritten: a fix goes on the branch that owns the code, and a stacked child picks it up when `/land` repairs the child after its parent merges.

## Running

jvillegasd-mode's Deciding and getting stuck rules run the work, with one change: nothing waits for the human, so a product or preference call takes the default too, and so does a gate the mode would hold for the human's approval.

## Stopping

- **Done:** each pull request is verified and green at its current head, with `open-pr`'s readiness report.
- **Dead end:** every remaining unit is parked with its evidence and the replan finds no new approach. Push what is verified, pause the rest with `checkpoint`'s Pause (a `wip:` commit and the next action), then report.
- **Epic written:** when the goal is a plan, the committed epic and its slices are done, and the run ends without building it.
- **Goal ruled out:** when the evidence rules out the goal itself, as jvillegasd-mode's Start step 1 describes, stop and report that evidence, since there is nothing left to drive toward.

"Keep going" and "going to bed" never trigger a pause; only done, a written epic, a dead end, a ruled-out goal, or the human's stop ends the run.

## Audit the trail

Nobody watched this run, so the `## Decisions` table is what the human trusts in its place. Before the report, whatever ended the run:

1. Walk the table against `git log <base>..HEAD` and the task document's evidence. Done when every row's evidence resolves and shows what the row claims, and every fork, pivot, revert, and default the run took has a row. Correct by appending a row that supersedes the wrong one, so the table stays a record of what was believed and when.
2. Launch one fresh `worker` at the `high` tier through `delegation`, on a model family other than this session's. Give it the task document, the base and head, and the location of this session's record, and none of your reasoning. It reads the record itself and returns, cited by row or by location in the record, what the human should read first: an `agent` decision with weak or missing evidence, a check claimed without a run, a choice that widened the scope or covered a symptom, a decision the record shows and the table lacks. It flags and redoes nothing. When no other family can launch, use this one and say so.
3. Add each omission it found as a row, and carry its flags into the reply unedited: a flag you disagree with stays, with your answer beside it.

**Reply:** the goal, then the `agent` decisions each with "tell me X to switch", so they are the first thing the human reads; then `Attention`, opening with `reviewed by <model>` and listing each flag with its row, or `no flags`; then the definition of done with its final state, each pull request with its readiness verdict, the follow-ups, the parked dead ends with their evidence, and the time the run took.
