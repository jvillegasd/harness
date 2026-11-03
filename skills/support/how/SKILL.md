---
name: how
description: Use for code walkthroughs, runtime flows, architecture, and ownership or layering questions. Explain how a subsystem works; use why for its motivation.
---

# How

Answer the question by tracing the code at the depth it needs. State an interpretation if the target is ambiguous, then explore so the user can redirect.

1. Find the entry points and the relevant components. Trace the flow from trigger to result, including data transformations, decisions, boundaries, inputs and outputs. Follow callers and dependencies where they matter; distinguish what the code shows from observed runtime behavior.
2. Note non-obvious behavior, failure paths, and open questions. For ownership questions, compare the responsibilities and dependencies of the candidate locations. Cite the files and lines that support the answer.
3. Synthesize a mental model: start with what the subsystem does, then walk through the relevant flow and where to find its important pieces. Define unfamiliar concepts. Use a diagram when it clarifies relationships; omit sections that do not help the question.

Explore and synthesize inline by default. When a bounded independent search has a named benefit, use the configured `code-search` role under Harness delegation policy and check its citations. Reconcile contradictions against the source and name any gaps that remain.
