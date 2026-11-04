# Verify findings

Three briefs, each given once per review with every finding it covers. The parent pastes the matching brief together with the numbered findings and the revision. No brief says who raised a finding or why the code was written this way.

## Executor: make each one fail

- **Goal**: prove each numbered finding true by running the real code.
- **Scope**: new test or script files inside the recorded review/verifier worktree the parent names. Keep every file in a recorded artifact list, including package-local tests when they are needed to reach package-private code. Never overwrite baseline source files.
- **Base**: the reviewed revision the worktree is checked out at.
- **Verify**: each check runs and fails for the reason its finding gives.
- **Report**: the fields below, once per finding.

1. Read every finding and the code it cites.
2. For each finding, write the smallest test or script that calls the real code and fails loudly if the finding is true. Follow the style of an existing test file, or write a script that imports the shipped code. Keep one file per finding, so one check never masks another.
3. Run each check. Confirm the failure comes from the cited path, because a check that fails for an unrelated reason proves nothing.
4. Leave baseline source files, Git history, and everything outside the worktree as you found them. Record all created paths and leave the artifacts for the parent to save. For a security finding, use dummy principals and secrets, and keep every run off the network and away from shared services.

This report replaces the executor role's output fields, since a detached worktree has no branch. For each finding, by its number, report `verdict: red`, `green`, or `could-not-run`, then:
- the command;
- the decisive lines of its output;
- the path of the test file;
- for `green` or `could-not-run`, what you tried.

`red` means the finding is true.

## Reviewer: try to refute

Given only the numbered findings and the code, decide for each whether the failure it describes can reach observable behavior. Trace from the input or state that would trigger it toward the claimed wrong output, crash, or data change. Judge each finding on its own; one finding's verdict says nothing about another's. Read only the original snapshot supplied by the parent; report an identity mismatch as unknown.

Report, for each finding by its number, one of:
- `refute: unreachable`, with the `path:line` where the path is cut;
- `refute: reachable`, with the completed trace;
- `refute: unknown`, with what you could not trace.

Only `unreachable` with a cited cut counts as a refutation.

## Reviewer: validate the fixes

Given the confirmed findings, the check that proved each one, the repair delta, and the requirements, decide for each finding whether the repair removes the failure at its root, not at one symptom or one call site, and whether the repair breaks anything the findings did not name. Read the repaired code and its callers; you cannot run anything. Use the repaired snapshot supplied by the parent.

Open your report with the reviewer role's header lines, with `revision:` echoing the repaired revision and `axis: validate`. Then report, for each finding by its number, one of:
- `holds`, with the `path:line` where the failure path is now cut;
- `fails`, with the trace that still reaches the failure;
- `unknown`, with what you could not read or trace.

Use `unknown` when you could not read the code a verdict turns on. Never report `fails` for something you did not observe: a `fails` reopens the finding and sends it to the human.

Then list:
- `regressions:` each new defect the repair introduced, in the reviewer role's finding format, or `none`;
- `follow_ups:` each observation that blocks nothing, such as a missing test for a fixed path or a related cleanup, one line each, or `none`.

Report nothing the repair did not cause.
