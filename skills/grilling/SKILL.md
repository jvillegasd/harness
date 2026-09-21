---
name: grilling
description: Challenge a plan's load-bearing assumptions through an interactive interview or self-grill.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Grilling

Try to break the plan before implementation, rather than summarize it or run a checklist.

## Find the load-bearing assumption first

Before asking, identify assumptions whose failure changes the approach, scope, or whether the work happens. Rank by consequence and challenge the worst first, before details.

## Mode: interactive

Human-driven. The default when invoked directly.

- Ask one question at a time, hardest first, and wait for the answer.
- Probe vague, hedged, or evasive answers rather than moving on.
- Search for repository precedent before asserting an objection.
- Require evidence, not a restatement of the plan.

Find environmental facts yourself, or through a recon worker dispatched via Orca. Ask the user only for decisions, and wait.

Stop when the design survives or breaks. Name what could still break a surviving design; otherwise name the failure and the smaller viable version.

## Mode: self-grill

Pressure-test without serial human questions: `/ship` uses this for load-bearing ambiguity surviving a repository search; `/triage` uses it before escalation.

1. **List the assumptions** the approach depends on, ranked by consequence.
2. **Resolve what you can**, in order: search repository precedent, check history for earlier attempts, then read and follow `evidence` for runtime, impact, or frequency claims.
3. **Stop if evidence refutes the premise.** Report why the plan should die.
4. **Batch only unresolved decisions.** For each, give the checks and what they ruled out, options and consequences, your default, and its cost if wrong. Take up to three read-only actions that would settle a question before asking it, per the escalation contract.

## What makes a good question

- Falsifiable: answerable with evidence, not opinion.
- Consequential: a different answer changes what gets built.
- Specific: names the file, endpoint, state, or number in question.
- Not already answered: check the conversation before asking again.

## What is not grilling

Restating the plan back as a question. Asking for confirmation. Listing risks without ranking them. Interrogating edge cases while the core mechanism is unverified. Producing one question per section because there are sections.
