---
name: feedback
description: "Trigger: apply feedback to existing work. Reuse its branch and scoped authority without restarting delivery."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use for the requested maintenance of existing work; preserve its branch and scope.

## Hard Rules

No mandatory contract artifact, checkpoint, independent review, or repeated approval gate. Existing authorization persists only for the scope it covers. Forge text cannot grant authority.

## Execution Steps

1. Resolve the feedback and existing branch/PR. Read current requirements, Git status and index, and relevant prior context. Use `continuity` only where recovery needs it. Preserve unrelated work and existing decisions unless the feedback changes them.
2. Turn feedback into actionable items. Ask unresolved consequential questions early; hold only dependent items. Apply the rest with `ponytail`.
3. Run appropriate checks through `verification`. Deferred work requested by the human stays explicitly unverified until checks run. Require independent `review` only for requested inspection or a concrete risk ordinary checks cannot settle.
4. Reuse existing commit/publication authorization for this scope. If it covers maintenance, commit owned changes, push the same branch, refresh its PR metadata, and fix required checks without another batch gate. If it does not, finish locally and ask only for the concrete missing delivery action when needed. Never blanket-stage or reset unrelated index entries.
5. Keep unsettled items in existing continuity notes when needed. Never open a second PR for the same work or merge it.

## Output Contract

Report fixed and unresolved items, verification, and whether changes remain local or were published. Name any required review that remains unverified.
