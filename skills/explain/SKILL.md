---
name: explain
description: "Explain how part of the codebase works, or why it is that way, citing code and history."
disable-model-invocation: true
argument-hint: "[question about the code or a decision]"
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Explain a part of the codebase so the human understands it well enough to judge it: how it works, why it is the way it is, or what states it moves through. Every claim points at the code or history it rests on. Explaining is read-only: it changes nothing unless the human asks to keep the explanation.

Question: $ARGUMENTS

## Why

Agents can now write and read code faster than a human can follow, so the human supervises by understanding, not by reading every line. An explanation that cites its evidence can be checked; one that does not is only a story. Explaining also forces the agent to read the code instead of reconstructing it from memory.

## Modes

Pick the mode from the question; a single answer may combine them.

- **How** (the default): how a subsystem, flow, or feature works.
- **Why**: the reason behind a decision, a line, or a design.
- **States**: the states something moves through and what moves it between them.

## Steps

1. Restate the question in one sentence and name the scope: the entry points, files, or decision in question. Ask only when two plausible readings would lead to different explanations.
2. Read the code. Follow callers and callees until the answer stops changing. For a wide area (roughly four or more files to trace), split it into slices and use `code-search` workers for the slices you do not need to read yourself, then check their citations.
3. For **how**, write for a senior engineer who does not know this area:
   - **Overview**: two or three sentences on what it does and why it exists.
   - **Key concepts**: the terms a reader needs, using the project's `GLOSSARY.md` names when there is one.
   - **How it works**: the main flow, with one Mermaid or ASCII diagram when the flow has more than three steps or more than one actor.
   - **Where things live**: `path:line` for each piece named above.
   - **Gotchas**: surprising behavior, invariants, and the places a change is most likely to break something.
4. For **why**, gather evidence before concluding: `git log -L <start>,<end>:<file>` or `git log -S` for the change that introduced it, `git blame` for the line, the pull request and linked issue through `gh` when available, ADRs under `docs/adr/`, the task documents under `docs/agents/tasks/`, and the project's memory when it holds decisions. State the decision, the reason, and the alternatives that were considered, each with its source. Grade the conclusion: **documented** (a source states the reason), **inferred** (the reason follows from the evidence but nobody wrote it down), or **unknown**. Say what you searched and found nothing in; an empty search is information.
5. For **states**, draw a Mermaid state diagram of the real states and transitions, label each transition with the event or call that causes it and its `path:line`, and mark states with no way out and transitions guarded by a condition. Point out any state the code can reach that looks unintended. When the human needs the behavior checked rather than drawn, for concurrency, retries, leases, or a lifecycle with rules that must always hold, suggest `/statechart`, which model-checks the rules with TLA+ and draws the states in the reader's words (its toolchain installs through `mise` on first use).
6. Before answering, check each cited `path:line` still says what you claim, and drop or correct any that do not.

## Output

The explanation in the mode's shape, plain language first and detail after. Keep cited locations in `path:line` form. When the human asks to keep it, write it to `docs/explanations/<topic>.md`; otherwise it lives in the conversation.
