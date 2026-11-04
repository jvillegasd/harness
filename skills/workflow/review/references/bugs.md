# General reviewer: bugs and baseline coverage

Hunt for defects that change behavior. Read the diff against the whole repository and the requirements, and answer three questions: does it solve its problem, does it fit the system, and does it break anything else?

- **Requirements.** Trace every supplied requirement to the changed behavior. Report missing behavior and unrequested scope. Without a requirements source, state that limit.
- **Contracts and security.** Inspect compatibility, permissions, input validation, data preservation, and failure recovery wherever the change touches them.
- **Conventions.** Check documented project rules and unnecessary complexity; leave tool-enforced formatting to tooling.
- **Root cause.** On a bug fix, find the root cause and check that the fix lands there, not at one symptom or one call site.
- **The safety fact.** Most risky-looking changes are safe because of one fact, such as "this call only drops entries that are already dead". Find that fact and try to break it. If it holds, say so on your `summary:` line. If it fails, that is your finding.
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
- **Pre-existing.** A defect in code the diff did not introduce (check the base) is `pre-existing`.

Specialists add independent depth. Their presence never removes these baseline questions from your review.
