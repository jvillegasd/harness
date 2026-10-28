---
name: spec
description: "Trigger: spec this out or write a specification. Draft an optional local specification without automatic publication or implementation."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use when the human requests a specification or durable design reasoning is useful. A spec is optional; no tracker configuration is required for a local draft.

## Hard Rules

Approval of the draft does not publish it, create tickets, or start implementation. Preserve user requirements verbatim where exact wording matters. Do not turn uncertainty into an invented agreement.

## Execution Steps

1. Read the settled discussion and relevant code, domain terms, and ADRs. Reuse grounded evidence. Ask only unresolved consequential questions while drafting independent settled sections.
2. Reuse an existing document; otherwise choose the repository's spec location or `docs/agents/tasks/<feature>.md`. Combine continuity information here when the work needs recovery, rather than creating a second ledger.
3. Draft the problem, intended behavior, acceptance checks and meaningful test seams, scope boundaries, decisions, and unresolved risks. Include exact examples and identifiers. Use domain/design guidance only where it helps resolve uncertainty; no compulsory grilling.
4. Return the local draft. Publish only when explicitly requested for that destination and scope, using tracker configuration when applicable. Read back any authorized publication. Ticket creation and implementation each need their own request; do not transition automatically.

## Output Contract

Return the document locator, settled behavior, and consequential open questions. Clearly distinguish a draft from anything published.
