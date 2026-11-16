---
name: setup-harness
description: Configure a repository's tracker and critical paths for the harness.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Setup Harness

Write `docs/agents/harness.json` for this repository. Explore before writing and reuse the repository's clear domain and documentation conventions; ask only when an inference would be unsafe. Keep every key already in the file.

## Tracker

Set `"tracker"`. Use `"github"` when the repository has a GitHub remote (`git remote -v`) and the human tracks work in its issues: `harness tickets publish` writes only to that tracker. For another tracker, set its name and record its native parent and dependency behavior in `docs/agents/README.md`. Without a tracker, leave the key out: ticket drafts under `docs/agents/tickets/` and `build` need none. Ask the human when the evidence conflicts, and name what you relied on in the report.

The harness installs no labels and no lifecycle automation. `build` computes its frontier from merged work, and ticket state lives in the tracker's native open, closed, and blocked-by state.

## Critical paths

Add `criticalPaths`: a list of repo-relative globs (`**` spans directories) for the code where a mistake is a one-way door, such as auth, payments, migrations, and deployment configuration. `change-map` treats a match as `critical`. Infer it from the repository's structure, keep any existing entries, list it in the report, and leave it out when nothing stands out.

Report the file's path, the tracker you chose with its evidence, and the `criticalPaths` you added.
