# Large

**You own the workflow.** No narrower playbook fits, so design one: phases that scale rigor to the task, run as experiments, and leave a trail the human can audit.

1. **Frame.** State the definition of done as a pass/fail check (`principle-prove-it-works`), the scope in rough units, the blockers grounding surfaced, and the rigor level, biased high: one-way doors and a wide blast radius get more gates. In interactive work mode, show the human this framing once before a multi-hour run. Under autopilot, state it and start.
2. **Design the workflow.**
   - Split the work into units that each land on their own, riskiest unknown first (`principle-foundational-thinking`, `principle-sequence-verifiable-units`).
   - Build the verification harness before the work, with its baseline taken from the state before the change, so each check reads as old value against new.
   - For a one-way design decision, run [design](design.md). Mechanical work whose shape is already concrete skips it.
   - Fan out only across seams, each worker in its own worktree, through `delegation` (`principle-separate-before-serializing-shared-state`).
   - Write the phase list into the task document (`continuity`), then add each phase to the todo list.
3. **Run the loop.** Each unit is an experiment: state the hypothesis, make the smallest change, measure it against the definition of done on the real artifact, and keep it or revert it. Pair delegated work with a judge: when a worker games the check, harden its contract; when the check itself is wrong, fix the check in its own change. Every result is verified, not verified, or inconclusive, and inconclusive is not a pass.
4. **Keep the trail.** Add a row to the `## Decisions` table as each unit lands, not at the end.
5. **Verify and hand back.** Check the whole against step 1's definition on the real product, not only the harness. Turn a correction that kept recurring into a check (`principle-encode-lessons-in-structure`). Run `change-map`, then `review` at the level `harness review level` reports, and `open-pr` when delivery is authorized.

**Reply:** the phases, the rigor level and why, the task document path, what is verified against the definition of done, and what is still open.
