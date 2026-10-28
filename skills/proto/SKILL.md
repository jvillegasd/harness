---
name: proto
description: "Trigger: prototype, spike, or mock up an idea. Run optional local experiments to resolve a specific uncertainty."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use for requested experimentation or when a local experiment helps answer an unsettled question. Ordinary implementation is not routed here merely because some decisions remain open.

## Hard Rules

No mandatory ledger acceptance, WIP commits, ticket claims, or graduation into another capability. Commits and publication need explicit authority. Prototype status does not exempt concrete risks from independent review or meaningful verification.

## Execution Steps

1. Name the uncertainty, settled requirements, explicit assumptions, and smallest useful experiment. Inspect existing work and preserve unrelated changes. Reuse the branch; isolate only when useful.
2. Use `ponytail` and the project's existing styling layer. Build the slice and run appropriate local checks. Test-first sequencing is optional. Keep assumptions distinct from confirmed requirements.
3. Show what to try, what the evidence supports, and what remains unknown. Wait when human testing is the next source of evidence; continue already authorized independent experiments otherwise.
4. Incorporate feedback and retain requirements through `continuity` only when recovery needs it. Report prototype-quality code or missing coverage that retained implementation must address.
5. Finish with the experimental result or continue implementation when requested. An explicit delivery request uses `open-pr`; there is no mandatory promotion gate. A disproved premise does not authorize deletion: use `abandon` only for an explicitly requested discard.

## Output Contract

Report the result, how to try it, evidence and limitations, and any unresolved decision. Create no ledger solely to certify completion.
