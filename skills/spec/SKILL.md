---
name: spec
description: "Trigger: spec this out or write a specification. Draft an optional local specification without automatic publication or implementation."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Draft a local specification when the human asks for one or when durable design reasoning is worth keeping. A spec is optional, and a local draft needs no tracker configuration.

## Why

A spec records what was settled and what was not, so later work starts from agreed behavior instead of a reconstruction. That only helps if exact requirements stay verbatim and uncertainty stays visible as uncertainty.

## Steps

1. Read the settled discussion and the relevant code, domain terms, and ADRs, and reuse grounded evidence. Ask only the unresolved consequential questions, and draft the independent settled sections meanwhile.
2. Reuse an existing document; otherwise use the repository's spec location or `docs/agents/tasks/<feature>.md`. When the work needs recovery, fold the `continuity` information into this document instead of starting a second one.
3. Draft the problem, the intended behavior, acceptance checks and meaningful test seams, scope boundaries, decisions, and unresolved risks. Include exact examples and identifiers, and preserve the user's wording where it matters. Use domain and design guidance only where it helps resolve uncertainty.
4. Return the local draft. Approving it does not publish it, create tickets, or start implementation; each needs its own request. When publication is requested, use the tracker configuration for that destination and read back the result.

## Output

The document locator, the settled behavior, and the consequential open questions, with the draft clearly distinguished from anything published.
