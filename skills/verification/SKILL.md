---
name: verification
description: "Trigger: verify changes or coordinate checks. Run proportionate checks and report revision-bound evidence."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use when a task needs help choosing or coordinating verification. Ordinary checks do not require this skill.

## Hard Rules

Follow repository-required checks. Never make publication part of local verification. Never present a fixture, static read-back, or self-review as live acceptance or independent inspection.

## Execution Steps

1. Compare the requested behavior with the actual diff, including owned untracked files. Identify meaningful failure behavior and affected seams.
2. Resolve commands from project instructions, task runners, or the toolchain. Run cheap targeted checks first, then required repository checks. Use existing tests and add regression coverage where it can fail for the right reason. Do not require TDD unless the task or project does.
3. Bind results to the commit and, for uncommitted work, a content identity covering tracked and untracked inputs. Record commands and pass, fail, skipped, unavailable, or stale outcomes. Later edits invalidate affected evidence; HEAD equality alone is insufficient.
4. Read `review` only when requested or a named security, data-loss, compatibility, or concurrency concern remains beyond ordinary checks. Routine changes need no independent reviewer.
5. Fix owned failures and rerun affected checks. Escalate bounded repeated failures under policy. For authorized PR delivery, also run the forge's required checks; otherwise stop after local checks.

## Output Contract

Report what passed, its scope and revision, and remaining failures or unverified behavior. Keep evidence in an existing task document only when continuity needs it; create no verification artifact by default.
