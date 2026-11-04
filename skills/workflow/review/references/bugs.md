# Axis: bugs

Hunt for defects that change behavior. Read the diff against the whole repository and the requirements, and answer three questions: does it solve its problem, does it fit the system, and does it break anything else?

- **Root cause.** On a bug fix, find the root cause and check that the fix lands there, not at one symptom or one call site.
- **The safety assumption.** State the key fact the change depends on to be safe, and the evidence for it, on your `summary:` line. Label the evidence using the certainty ladder: asserted, cited, or traced. If you cannot establish the assumption, say so in the summary. Report a `q` only when you can name a concrete potential defect and the missing fact needed to decide it, especially when impact is high. An unproven assumption alone leaves a clean hunt clean; a concrete unresolved `q` goes through Verify.
- **Where grep stops.** Listing callers is the easy part. Follow what a symbol search misses:
  - the source and pinned version of a library the code calls;
  - when things run: startup, teardown, retries, and concurrent callers;
  - a wire format, or a file or database layout that another program reads;
  - a config key or flag;
  - another language reading the same bytes;
  - code three hops downstream.
- **The class.** When a defect is one instance of a pattern, search the scope for every instance and list them all under one class.
- **Instruction drift.** A line that the change makes false is a defect, whether it sits in the project instruction file, a README, a skill, or other docs. For documentation, failure means it lies.
- **Error paths.** Look for a failure handled wrongly, swallowed, or missing, and for partial state left behind.
- **Tests.** Judge whether the checks would catch this change breaking:
  - a changed behavior with no check that asserts what a user or caller sees;
  - a check that would still pass if the logic it covers were deleted, or that asserts internals instead of the behavior;
  - edge cases the change makes reachable but no check covers: boundaries, empty and invalid input, retries, and failure paths;
  - nondeterminism: time, order, randomness, or the network left uncontrolled, so the check passes or fails by chance;
  - a focused or skipped test (`.only`, `skip`, `t.Skip`) left in, so CI passes without running the suite.
- **Impact and evidence.** For every finding, report impact (`high`, `medium`, or `low`) separately from severity and class, and report the strongest evidence actually obtained (`asserted`, `cited`, `traced`, `executed`, or `observed`). The review parent may run at most one focused safety probe per slice after all hunting, including after a clean hunt. No supported findings does not prove the safety assumption. A probe can raise evidence only to what it actually executed or observed.
- **Pre-existing.** A defect in code the diff did not introduce (check the base) is `pre-existing`.

Style, naming, and structure belong to the standards hunter, and trust boundaries and dependency failures to the risk hunter when one runs; when none runs, they are yours.
