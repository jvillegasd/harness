# Design

**You own the decision.** Explore competing shapes and pick one with evidence before anything is built on it.

1. Name the decision as one question. When there is no decision to make, there is no design step: `skip:` this playbook and match the task again.
2. Explore by the kind of question:
   - **UI or interaction:** read `principle-exhaust-the-design-space`, then build two or three competing variants side by side with `proto`, using `impeccable` for craft.
   - **A module or backend boundary:** run `codebase-design`'s Design It Twice, with three or more subagents each producing a very different interface for the same module, compared by how much each hides behind a small interface.
   - **Logic or state:** build a `proto` logic model.
3. Compare the options on evidence, pick one, and record it in the task document's `## Decisions` table (`continuity`), with the alternative it beat.
4. Match the build as a feature or a refactor, carrying the decision with it.

**Reply:** the options, the one chosen, and the evidence for it.
