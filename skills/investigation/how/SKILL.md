---
name: how
description: Use for code walkthroughs, runtime flows, architecture, and ownership or layering questions. Explain how a subsystem works; use why for its motivation.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# How

Explore the codebase and explain how the target works so an engineer unfamiliar with it can start working on it confidently. If the scope is ambiguous, state your interpretation and proceed.

Trace the relevant flow and responsibilities, cite the files and lines, and explain the non-obvious behavior. Distinguish what the code shows from observed runtime behavior. Use a diagram when it clarifies the explanation.

Read inline when the answer needs a few files. When it needs a search across many files, history, or logs that would crowd this session's context (`principle-guard-the-context-window`), or an independent search has another named benefit, use `delegation` for the `code-search` role on the model `harness models role resolve code-search --json` returns (`principle-launch-on-the-assigned-model`), keep its cited result rather than the raw reads, and verify its citations before synthesizing.
