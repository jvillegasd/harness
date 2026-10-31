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

Settle the mode in this order: the request, then existing configuration, then repository evidence. Without a GitHub remote, use local markdown. With one, a protected default branch decides for GitHub, because local markdown's bookkeeping commits need an unprotected default branch (check with `gh api repos/{owner}/{repo}/branches/<default> --jq .protected`). An existing local spec tree such as `docs/specs` points to local markdown. Hosting alone is weak evidence, so name what you relied on in the report. Ask the human when the evidence conflicts, for example a protected default branch alongside an existing local spec tree, or when a GitHub remote is the only signal: GitHub mode creates labels and a workflow on the remote, so the choice is theirs. Other trackers follow the last section.

Write `docs/agents/harness.json` with this stable shape:

```json
{
  "version": 1,
  "tracker": "github",
  "spec_home": "github-issues",
  "domain_layout": "single-context",
  "labels": {
    "blocked": "blocked",
    "ready_for_agent": "ready-for-agent",
    "in_progress": "in-progress",
    "ready_for_human": "ready-for-human"
  }
}
```

`tracker` and `spec_home` select the mode; commands resolve `<spec_home>` to this value. `domain_layout` may be project-specific. Add `criticalPaths`, a list of repo-relative globs (`**` spans directories) for the code where a mistake is a one-way door, such as auth, payments, migrations, and deployment configuration. `change-map` treats a match as `critical`. Infer it from the repository's structure, list it in the report, and leave it out when nothing stands out.

For local markdown, change only the two mode values. Use `docs/specs` for `spec_home`, or the project's docs convention when it has one, and state the path in the report instead of asking about it:

```json
{
  "version": 1,
  "tracker": "local-markdown",
  "spec_home": "docs/specs",
  "domain_layout": "single-context",
  "labels": {
    "blocked": "blocked",
    "ready_for_agent": "ready-for-agent",
    "in_progress": "in-progress",
    "ready_for_human": "ready-for-human"
  }
}
```

Keep the four label keys stable while allowing mapped values to change. Update an existing file in place so user choices survive.

For GitHub:

1. Verify the remote and `gh auth status`; stop and report missing credentials or issue permissions.
2. Create or update the four mapped labels with `gh label create --force`.
3. Write `docs/agents/issue-tracker.md` for locked Matt skills: identify GitHub, require the `gh` CLI, explain reading the source issue/spec, and name native sub-issues and dependencies as the relationship authority. The file stands alone and needs no second setup skill.
4. Copy `assets/agent-lifecycle.yml` to `.github/workflows/agent-lifecycle.yml` and `scripts/reconcile-agent-lifecycle.go` to `.github/scripts/reconcile-agent-lifecycle.go`. Remove a superseded `.github/scripts/reconcile-agent-lifecycle.py` if present.
5. Preserve unrelated workflows and configuration. Include both copies in the local change, and explain that automation activates only after both reach the default branch.

For local markdown:

Create no labels. `Status:` uses the four mapped values plus literal `done`.

Explain once that local markdown requires an unprotected default branch for the bookkeeping commits listed by `local-tracker`, because later protection breaks subsequent tracker writes.

1. Write a standalone `docs/agents/issue-tracker.md` for locked Matt skills: identify local markdown, state that it needs no CLI or network, give the root path, and reproduce `local-tracker`'s Layout, State, Identity, and Frontier sections in full. Change those rules in the source skill first, then regenerate this restatement. Reference that skill for other rules, including lifecycle-to-commit mappings. The file stands alone and needs no second setup skill.
2. Copy no `.github/` workflow or reconcile script. Explain that committed lifecycle state emits no issue events for automation.
3. Preserve unrelated documentation and configuration.

For other trackers, record native parent/dependency behavior in `docs/agents/README.md`. Install no GitHub automation and promise no automatic parent closure.

Report every created or updated path, the tracker mode and `spec_home` you chose, and the evidence behind them. Leave interactive authentication to the human.
