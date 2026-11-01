# Reconciler model

A TLA+ model of the agent lifecycle reconciler (`.github/scripts/reconcile-agent-lifecycle.go`) and its workflow (`.github/workflows/agent-lifecycle.yml`), checked with the locked `statechart` skill. It is the pilot for model-checking state machines in this harness.

## What it covers

A parent with two children, A and B, where B is blocked by A. People start work, mark a pull request ready, merge, close by hand, and reopen. Each workflow run is one atomic step that reads live state. The workflow's concurrency group keeps one running and at most one pending run, so a newer event replaces the pending one; closes and reopens the reconciler makes with `GITHUB_TOKEN` start no run.

Rules, checked once no run is running or pending:

- `ParentClosesOnlyWhenAllMerged` and `ParentClosesWhenAllMerged`: a parent is closed exactly when every child was closed by a merged pull request.
- `BlockedExactlyWhileBlockerUnmerged`: an open B is `blocked` exactly while A is not merged.
- `ParentNeverReadyForAgent`: a parent never carries `ready-for-agent`.

## What it found

Before the fix, two rules broke (`bugs-before-fix.mmd` draws both paths):

1. A parent closed by hand and reopened before the workflow ran was reset to `ready-for-agent` like a ticket, so an agent could pick up the parent itself.
2. A blocker's close event replaced in the pending slot by a later event left B `blocked` after A merged.

Both paths were replayed as Go tests (`TestReopenedParentIsNotReadyForAgent`, `TestParentRunHealsAChildsFrontier`) that failed on the old code and pass on the fix: a reopened parent is not reset, and reconciling a parent recomputes every child's frontier. With the fix all four rules hold over 6,714 states.

## What it leaves out

Runs are atomic, so concurrent writes inside one run are not modeled. One tree is modeled: an event from an unrelated tree can still replace a pending run, and that tree heals only on its next event. Label edits by hand, sub-issue changes, and blockers outside the tree are not modeled.

## Rerun

```sh
skills/statechart/scripts/statechart docs/agents/models/reconciler/Reconciler.tla \
  --config docs/agents/models/reconciler/Reconciler.cfg \
  --view docs/agents/models/reconciler/view.mjs --out <new scratch folder>
```

The first run installs Java and TLC with `mise` and needs the network. The model describes the code after the fix. To see the old failures, make `Run` recompute only B's frontier after an event on A, and pass `reset` instead of `FALSE` to `PFrontier`.
