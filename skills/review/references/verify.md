# Verify a finding

Two briefs. The parent pastes the matching one together with the finding text and the revision. Neither brief says who raised the finding or why the code was written this way.

## Executor: make it fail

- **Goal**: prove one finding true by running the real code.
- **Scope**: new test or script files inside the detached worktree the parent names.
- **Base**: the reviewed revision the worktree is checked out at.
- **Verify**: the check runs and fails for the reason the finding gives.
- **Report**: the fields below.

1. Read the finding and the code it cites.
2. Write the smallest test or script that calls the real code and fails loudly if the finding is true. Follow the style of an existing test file, or write a script that imports the shipped code.
3. Run it. Confirm the failure comes from the cited path, because a check that fails for an unrelated reason proves nothing.
4. Leave the code under test, Git history, and everything outside the worktree as you found them. For a security finding, use dummy principals and secrets, and keep every run off the network and away from shared services.

Report `verdict: red`, `green`, or `could-not-run`, then:
- the command;
- the decisive lines of its output;
- the path of the test file;
- for `green` or `could-not-run`, what you tried.

`red` means the finding is true.

## Reviewer: try to refute

Given only the finding and the code, decide whether the failure it describes can reach observable behavior. Trace from the input or state that would trigger it toward the claimed wrong output, crash, or data change.

Report one of:
- `refute: unreachable`, with the `path:line` where the path is cut;
- `refute: reachable`, with the completed trace;
- `refute: unknown`, with what you could not trace.

Only `unreachable` with a cited cut counts as a refutation.
