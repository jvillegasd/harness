---
name: harness-help
description: "Trigger: which skill should I use, how does this harness work, what's the workflow, /harness-help. Map the harness's workflow and skills for someone deciding what to reach for, and answer with the part of the map their situation needs."
argument-hint: "[situation or question about the workflow]"
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Tell the human which harness skill fits their situation and how the skills fit together. This is a read-only guide to the map below; the skills themselves hold their procedures, so point at them instead of restating them.

Question: $ARGUMENTS

## Why

The harness has many independent skills and no required path through them, so a newcomer cannot tell which one to reach for or whether any is needed. The default is still to work directly: understand the request, implement the settled scope, run the relevant checks, and report. Each skill below earns its place only when its situation shows up, and none is a prerequisite for another.

## The workflow at a glance

Each stage is optional and entered when its situation appears; the order shows where work usually flows, not a sequence to follow.

```
understand   explain                 any time, read-only
    |
intent       grill-with-docs or grilling   when intent is unclear
             proto                         when an experiment can settle a question
             spec                          for multi-session or durable design (tickets on request)
    |
build        inline with tdd and ponytail (the default)
             /build                        an unattended graph of tickets or spec slices
    |
prove        verification                  checks and the certainty ladder
             e2e-verification             release runs and feature maps
    |
triage       change-map                    what the human must read
             review                        on the critical items
    |
deliver      open-pr -> address-review -> the human's /land    (or /abandon)
    |
learn        /retro
```

## Which skill when

| Situation | Skill | Note |
| --- | --- | --- |
| You need to understand code, or why it is the way it is | `explain` | How, why, or states; every claim cites code or history. |
| The intent or plan is still fuzzy | `grilling` | An interview that settles the decision tree. `/grill-with-docs` also records settled terms in the glossary and ADRs. |
| A question an experiment can answer | `proto` | A throwaway logic model or UI variants, to answer one stated question. |
| A design worth keeping across sessions | `spec` | A local spec with agreed test seams; tickets only when asked. |
| Writing or changing code | `tdd`, `ponytail` | Test-first loop, and the simplest thing that works. |
| Something is broken, throwing, or slow | `diagnosing-bugs` | A diagnosis loop that reproduces before it fixes. |
| Latency or throughput must improve | `optimization` | Measure the bottleneck, change it, and measure again. |
| An unknown that needs outside sources | `research` | A sourced finding ledger; read-only. |
| A ticket or production error must become a plan | `triage` | Checks the premise first, then returns a plan brief without implementing. |
| A claim carries the whole decision | `evidence` | Start with the query that could cancel the work. |
| UI work | `impeccable` | Frontend design and critique; the project's styling layer still wins. |
| Prove a change works | `verification`, `e2e-verification` | Revision-bound local checks; a scripted browser run with screenshots for a release. |
| Tell the human what to read, or get an independent look | `change-map`, `review` | A tiered map of the change; review a named, concrete risk without the implementer's story. |
| The architecture feels shallow or hard to navigate | `improve-codebase-architecture` | Finds deepening opportunities and walks through the one you pick. |
| Security questions or an audit | `security-audit` | Guidance by default; the full audit runs only when an audit or pen test is requested. |
| Work spans sessions, or you are handing it over | `continuity`, `handoff` | One recoverable task document; a handoff document for a fresh agent. |

## Skills only a human starts

`land`, `abandon`, `setup-harness`, `grill-with-docs`, `build`, and `retro` run only when a human types them, because they merge, discard, provision, start a human-led interview, run unattended at model-budget scale, or read session records to change the harness itself. Starting one is the human's decision, so name it for them and let them invoke it. The locked `handoff` and `improve-codebase-architecture` carry an upstream guard of their own and are started by name the same way.

## Crossing phases

When the work reaches a boundary between phases, such as spec to implementation or implementation to review, `continuity` holds the choice of how to cross it.

## Output

Answer the human's question with the part of the map it needs: the one to three skills that fit, why, and what usually comes next. Give the whole map only when they asked for the overview. Name the invocation (`/name`) for the skills a human must start, and leave the starting of them to the human.
