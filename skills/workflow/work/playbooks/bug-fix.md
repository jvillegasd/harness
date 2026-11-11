# Bug fix

**You own the root cause.** Every shipped line traces to runtime evidence. A change that "might help" is a hypothesis, not a fix, and when evidence refutes a hypothesis, revert what it motivated.

1. Reproduce it yourself on the surface where it shows up: the project's `verify-<app>` skill, `journey-verification`, the CLI, or a test. Ask the human to reproduce only with a stated reason that surface cannot reach the target, after driving it as far as it goes. When it will not reproduce directly, synthesize the trigger, tighten the conditions, or instrument until it fires.
2. Run `diagnosing-bugs`'s loop. Seed its hypotheses with `how` over the affected subsystem and `why` for the regression history, and confirm the surviving mechanism with runtime evidence before planning a fix.
3. Plan the smallest fix the evidence justifies. When it crosses a function boundary, run [design](design.md)'s step 2 first.
4. Commit the failing repro before the fix, so history shows red then green (`tdd`'s cadence when the bug has a cheap local test path; `skip:` it when that test would be expensive, integration-heavy, or unclear).
5. Fix it, and verify on the same surface as step 1: the original repro now passes. Inconclusive, or green on another surface, is not a pass. Unit tests show branch behavior, not that the bug is gone.
6. Run `change-map`, then `review` at the level `harness review level` reports.
7. When delivery is authorized, run `open-pr`. Otherwise stop with the work ready locally and offer delivery once.

**Reply:** what was broken, the root cause, the fix, and how you verified it, with the failing and then passing repro output pasted verbatim.
