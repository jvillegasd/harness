---
name: local-tracker
description: "Map authorized tracker operations to committed markdown files without gating local implementation."
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Local Tracker

Read when `docs/agents/harness.json` selects `local-markdown`. This capability maps tracker operations; it grants no commits, publication, or automatic transition. Ordinary implementation and local spec drafts need no tracker writes.

In this harness `ready-for-human` means the pull request is ready for a human to review, unlike mattpocock/skills, where it means a human must implement the ticket.

## Layout and identity

Use `<spec_home>/<feature-slug>/spec.md` and `<spec_home>/<feature-slug>/issues/<NN>-<slug>.md`. Derive the feature slug from the title, lowercase with non-alphanumeric runs replaced by one hyphen. Preserve existing identifiers. Ticket id `<feature-slug>-<NN>` resolves to exactly one issue file; a promoted single-slice spec uses `<feature-slug>` and resolves to `spec.md`. Numbers alone are ambiguous across features.

Use repository-relative ticket paths in PR closing prose, never `Closes #<NN>` for a local number. Read the whole file and its oldest-first `## Comments` thread. Search the configured spec home for duplicate work before writing.

## State

An implementation-ready ticket has `Status: <mapped label>` and `Blocked by: <same-feature numbers or none>`. Cross-feature dependencies stay explicit in prose. Planning-only items without acceptance checks have no ready label and block parent closure. A parent with children becomes `done` only when every required child has merge evidence. A promoted one-slice `spec.md` is itself the ticket and becomes `done` on its own merge. Cancellation is not `done`; no cancellation representation is defined here, so ask before inventing one.

## Authorized operations

Local drafting stays local. Publishing tracker state needs explicit scope covering commits and pushes to its destination. When already covered by the request or an invoked capability, do not ask again. For default-branch bookkeeping, run `harness land sync` to fetch and fast-forward the default branch, then locate its worktree, verify protection and preserve unrelated index/worktree state. If protected, report the need for a supported PR route rather than creating an unpushable commit. Commits on the default branch need an explicit request (the floor); a covered tracker write allows only the default-branch bookkeeping below, and implementation never qualifies.

| Operation | State change |
| --- | --- |
| Publish requested spec/tickets | Write the requested artifacts and dependency edges. A parent is mapped `in_progress`; ready children are mapped `ready_for_agent`, unsatisfied dependents `blocked`. |
| Promote a settled planning item or single-slice spec | Preserve identity, add behavior/acceptance/checks and state in place; create no duplicate child. |
| Explicitly authorized claim | Re-read current state; claim `ready_for_agent` as `in_progress`. Reuse an existing claim only with evidence it belongs to this work. Do not overwrite another run's claim. |
| Authorized PR handoff | Set `ready_for_human` once required checks and any required review settle. Skip if already set. |
| Explicitly authorized release | Set an open claimed item to `blocked` or `ready_for_agent` by current dependencies; preserve scope. Never release a live PR. |
| `/land` closes | After confirmed merge and default-branch update, set ticket `done`, advance satisfied dependents, and close a parent only when every required child merged. |

Commit only changed tracker files under the existing authority, using `docs(tracker):` or `chore(tracker):` intent. Read back each publication. On partial failure preserve completed writes and report what remains; do not blindly retry. No claim, handoff, or ticket publication is a prerequisite for coding.

## Frontier

Scan all configured feature issue files and promoted single-slice specs. Ready means mapped `ready_for_agent` with every same-feature blocker `done` and any explicit cross-feature dependency satisfied. Missing acceptance/state or unverified merge evidence keeps an item out of the frontier. Local mode installs no lifecycle workflow or reconciler; file edits emit no issue events.
