---
name: grill-with-docs
description: Run grilling and record settled terms and decisions in the glossary and ADRs during the interview.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Grill With Docs

Read and follow `grilling` and load `domain-modeling`. Keep the interview protocol unchanged. Record settled terms and decisions in `CONTEXT.md` or an ADR before the next round, while their context is available.

## When this instead of bare `grilling`

Use this for decisions a later session needs but cannot reconstruct from code: a greenfield project without `CONTEXT.md`, a multi-session design, or another durable design discussion.

Use bare `grilling` for an immediate decision or a small plan whose pull request is sufficient record.

## What gets written

Follow `domain-modeling`'s formats. Create `CONTEXT.md` at the first resolved term and `docs/adr/` at the first decision needing an ADR. If nothing settles, write nothing.

Record the decision and its reason. The rejected alternatives belong in an ADR only when they are worth remembering.
