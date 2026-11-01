---
name: verification
description: "Trigger: verify changes or coordinate checks. Run proportionate checks and report revision-bound evidence."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Choose and coordinate the checks for a change, and report evidence tied to the exact revision it covers. Ordinary checks do not need this skill.

## Why

A pass is only worth what it covers. Binding each result to a revision, and naming what was run against what, lets the next reader trust it or know to rerun it.

## Steps

1. Compare the requested behavior with the actual diff, including owned untracked files. Identify the meaningful failure behavior and the affected seams.
2. Resolve commands from project instructions, task runners, or the toolchain. Run cheap targeted checks first, then the repository-required ones. Use existing tests, and add regression coverage where it can fail for the right reason. TDD applies only when the task or project asks for it.
3. Bind results to the commit and, for uncommitted work, to a content identity covering tracked and untracked inputs. Record each command with its outcome: pass, fail, skipped, unavailable, or stale. A later edit makes the affected evidence stale, and HEAD equality alone does not identify a dirty tree.
4. Use `review` when the human asked for it or a named security, data-loss, compatibility, or concurrency concern remains beyond ordinary checks.
5. Fix owned failures and rerun the affected checks. When failures repeat without progress, stop and report as POLICY's "Asking and deciding" describes. For authorized PR delivery, also run the forge's required checks; otherwise stop after the local ones.

A fixture, a static read-back, or a self-review is not live acceptance or independent inspection, so report it as what it is.

## Certainty ladder

When a claim carries the safety of a change, say how far it is proven. Take each fact as far down this ladder as is cheap, and report where it stopped:

1. Said: someone asserted it. Worth nothing on its own.
2. Cited: a real `path:line`, or the library's own source, shows it.
3. Walked: the failure path was traced step by step and does not reach.
4. Ran: a script or test calls the real code and fails loudly if the fact is wrong.
5. Reproduced: it was observed in the running application.

Only steps 4 and 5 count as verified. When the project has a feature map from `e2e-verification` (a `verify-<app>` skill), drive the features that `change-map` names for the behavioral and critical items; that is step 5. A small script that imports the shipped library and calls the exact function in question usually reaches step 4 cheaply.

## Output

What passed, its scope and revision, and the remaining failures or unverified behavior. Keep the evidence in an existing task document only when `continuity` needs it.
