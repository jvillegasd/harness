# Bug fix

**You own the root cause.** `diagnosing-bugs` finds it and fixes it; the steps around it are what work mode adds. Every shipped line traces to runtime evidence. A change that "might help" is a hypothesis, not a fix, and when evidence refutes a hypothesis, revert what it motivated.

1. Run `diagnosing-bugs`' Phases 1 to 4, with two bindings:
   - Its loop drives the surface where the bug shows up: the project's `verify-<app>` skill, `journey-verification`, the CLI, or a test. Ask the human to reproduce only with a stated reason that surface cannot reach the target, after driving it as far as it goes.
   - Seed its hypotheses with `how` over the affected subsystem and `why` for the regression history.

   Done when runtime evidence confirms the surviving mechanism.
2. Plan the smallest fix the evidence justifies. When it crosses a function boundary, run [design](design.md)'s step 2 first.
3. Run its Phases 5 and 6, committing the failing regression test before the fix, so history shows red then green. `skip:` that commit when Phase 5 finds no correct seam. Done when the original repro passes on step 1's surface; unit tests show branch behavior, not that the bug is gone.
4. Finish, as work mode's Finish lists.

**Reply:** what was broken, the root cause, the fix, and how you verified it, with the failing and then passing repro output pasted verbatim.
