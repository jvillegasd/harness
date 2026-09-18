---
name: setup-harness
description: Configure a repository for the portable spec-to-PR workflow.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Setup Harness

Explore before writing. Reuse clear domain and documentation conventions; ask only about unsafe inferences. Always ask for tracker mode: hosting does not determine where contracts belong.

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

Always present GitHub and local markdown by name and ask, even without a remote. First run `gh api repos/{owner}/{repo}/branches/<default> --jq .protected`: if true, offer only GitHub and explain that local markdown needs an unprotected default branch. Off GitHub, present both and state that requirement. Other trackers follow the last section. `tracker` and `spec_home` select the mode; commands resolve `<spec_home>` to this value. `domain_layout` may be project-specific.

For local markdown, change only the two mode values. Propose `docs/specs` for `spec_home`, or the project's docs convention, and confirm the path with the mode:

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

Keep the four label keys stable while allowing mapped values to change. Update an existing file in place instead of discarding user choices.

For GitHub:

1. Verify the remote and `gh auth status`; escalate on missing credentials or issue permissions.
2. Create or update the four mapped labels with `gh label create --force`.
3. Write `docs/agents/issue-tracker.md` for locked Matt skills: identify GitHub, require the `gh` CLI, explain reading the source issue/spec, and name native sub-issues and dependencies as the relationship authority. Never require a second setup skill.
4. Copy `assets/agent-lifecycle.yml` to `.github/workflows/agent-lifecycle.yml` and `scripts/reconcile-agent-lifecycle.go` to `.github/scripts/reconcile-agent-lifecycle.go`. Remove a superseded `.github/scripts/reconcile-agent-lifecycle.py` if present.
5. Preserve unrelated workflows and configuration. Include both copies in the normal human-reviewed change, never push or merge them directly. Explain that automation activates only after both reach the default branch.

For local markdown:

Create no labels. `Status:` uses the four mapped values plus literal `done`.

Explain once that local markdown requires an unprotected default branch for the bookkeeping commits listed by `local-tracker`; later protection breaks subsequent tracker writes.

1. Write a standalone `docs/agents/issue-tracker.md` for locked Matt skills: identify local markdown, state that it needs no CLI or network, give the root path, and reproduce `local-tracker`'s Layout, State, Identity, and Frontier sections in full. Change those rules in the source skill first, then regenerate this restatement. Reference that skill for other rules, including lifecycle-to-commit mappings. Never require a second setup skill.
2. Copy no `.github/` workflow or reconcile script. Explain that committed lifecycle state emits no issue events for automation.
3. Preserve unrelated documentation and configuration.

For other trackers, record native parent/dependency behavior in `docs/agents/README.md`. Install no GitHub automation and promise no automatic parent closure.

Report every created or updated path. Do not authenticate interactively.
