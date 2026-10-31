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

## Output

What passed, its scope and revision, and the remaining failures or unverified behavior. Keep the evidence in an existing task document only when `continuity` needs it.
