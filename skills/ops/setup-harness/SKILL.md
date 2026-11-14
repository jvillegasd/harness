---
name: setup-harness
description: Configure optional tracker and lifecycle capabilities for a repository.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Setup Harness

Configure the optional tracker and lifecycle capabilities for this repository. Explore before writing and reuse the repository's clear domain and documentation conventions; ask only when an inference would be unsafe.

## Choosing the tracker mode

Run `harness setup probe` for the evidence: the GitHub remote, the default branch and whether it is protected, the mode an existing configuration selects, and any local spec tree. Settle the mode in this order: the request, then the existing configuration, then that evidence. Without a GitHub remote, use local markdown. With one, a protected default branch decides for GitHub, because local markdown's bookkeeping commits need an unprotected default branch. An existing local spec tree points to local markdown. Hosting alone is weak evidence, so name what you relied on in the report. Ask the human when the evidence conflicts, for example a protected default branch alongside an existing local spec tree, or when a GitHub remote is the only signal: GitHub mode creates labels and a workflow on the remote, so the choice is theirs. Other trackers follow the last section.

## Writing the mode

Run `harness setup tracker --mode github`, or `harness setup tracker --mode local-markdown` with `--spec-home <dir>` when the project's docs convention names one (the default is `docs/specs`; state the path in the report instead of asking about it). The command owns the configuration's stable shape, keeps mapped label values, `criticalPaths`, and keys it does not know, and prints every path or label it created, updated, or removed. In GitHub mode it refuses missing forge credentials before any write, creates the mapped labels, installs the lifecycle workflow and its reconciler, and removes the superseded script; leave interactive authentication to the human.

Then add `criticalPaths` to `docs/agents/harness.json`: a list of repo-relative globs (`**` spans directories) for the code where a mistake is a one-way door, such as auth, payments, migrations, and deployment configuration. `change-map` treats a match as `critical`. Infer it from the repository's structure, keep any existing entries, list it in the report, and leave it out when nothing stands out.

For GitHub, include the workflow and reconciler in the local change, and explain that automation activates only after both reach the default branch.

For local markdown, `Status:` uses the four mapped values plus literal `done`. Explain once that local markdown requires an unprotected default branch for the bookkeeping commits listed by `local-tracker`, because later protection breaks subsequent tracker writes, and that committed lifecycle state emits no issue events for automation.

For other trackers, record native parent/dependency behavior in `docs/agents/README.md`. Install no GitHub automation and promise no automatic parent closure.

Report every created or updated path the command printed, the `criticalPaths` you added, the tracker mode and `spec_home` you chose, and the evidence behind them.
