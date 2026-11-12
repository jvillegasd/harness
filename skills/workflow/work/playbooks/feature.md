# Feature

**You own the design.** `implement` builds it; the steps around it are what work mode adds.

1. Run `how` over the affected subsystem.
2. Name the data shape and its organizing structure (`principle-model-the-domain`) before any logic: a state machine over scattered booleans, a table over branching, a typed model over repeated shape assumptions.
3. When the change crosses a function boundary, run [design](design.md)'s step 2 before implementing. `skip:` it for a change inside one function.
4. Run `implement`'s steps.
5. Verify on the surface the user touches, in the running app (`verification`, with the project's `verify-<app>` skill or `journey-verification`).
6. Commit in small ordered units, each verified before the next (`principle-sequence-verifiable-units`).
7. Finish, as work mode's Finish lists.

**Reply:** what you built, the data shape and why, the design alternatives in a table when there were any, and open decisions.
