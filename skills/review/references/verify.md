# Verify findings

Two briefs, each given once per review with every finding it covers. The parent pastes the matching brief together with the numbered findings and the revision. Neither brief says who raised a finding or why the code was written this way.

## Executor: make each one fail

- **Goal**: prove each numbered finding true by running the real code.
- **Scope**: new test or script files inside the detached worktree the parent names.
- **Base**: the reviewed revision the worktree is checked out at.
- **Verify**: each check runs and fails for the reason its finding gives.
- **Report**: the fields below, once per finding.

1. Read every finding and the code it cites.
2. For each finding, write the smallest test or script that calls the real code and fails loudly if the finding is true. Follow the style of an existing test file, or write a script that imports the shipped code. Keep one file per finding, so one check never masks another.
3. Run each check. Confirm the failure comes from the cited path, because a check that fails for an unrelated reason proves nothing.
4. Leave the code under test, Git history, and everything outside the worktree as you found them. For a security finding, use dummy principals and secrets, and keep every run off the network and away from shared services.

This report replaces the executor role's output fields, since a detached worktree has no branch. For each finding, by its number, report `verdict: red`, `green`, or `could-not-run`, then:
- the command;
- the decisive lines of its output;
- the path of the test file;
- for `green` or `could-not-run`, what you tried.

`red` means the finding is true.

## Reviewer: try to refute

Given only the numbered findings and the code, decide for each whether the failure it describes can reach observable behavior. Trace from the input or state that would trigger it toward the claimed wrong output, crash, or data change. Judge each finding on its own; one finding's verdict says nothing about another's.

Report, for each finding by its number, one of:
- `refute: unreachable`, with the `path:line` where the path is cut;
- `refute: reachable`, with the completed trace;
- `refute: unknown`, with what you could not trace.

Only `unreachable` with a cited cut counts as a refutation.
