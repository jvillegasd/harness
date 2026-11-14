# Tracker reconciliation

For ticket-bound work only, after the merge is confirmed and `harness land sync` has updated the default branch. The landing request covers these scoped writes: name and verify the tickets and transitions without asking again. A transition outside that scope, or a protected branch, is reported and stops only the cleanup that depends on it.

The transitions are always the same three: the ticket closes, its parent closes once every child has merge evidence, and each sibling the merge unblocked moves to the mapped `ready_for_agent` state.

- **GitHub with the lifecycle workflow** (installed by `setup-harness`): confirm the workflow already made them.
- **GitHub without it, or another forge tracker**: apply them by hand and say which.
- **Local markdown** (`docs/agents/harness.json` sets `"tracker": "local-markdown"`): read the `local-tracker` skill. The transitions are file edits committed and pushed on the default branch, so they need a checkout of the updated `<default>` and must land before the task worktree is retired. Use the worktree that holds `<default>`. When none does and the verdict's `mode` is `local`, require this checkout to be clean and switch it to `<default>`; a failed switch stops reconciliation and cleanup. `harness land finish` then retires the task branch by name, since this checkout no longer holds it. In any other mode this checkout is not the run's, so report the transitions for the human instead. Preserve unrelated work on the default branch.
