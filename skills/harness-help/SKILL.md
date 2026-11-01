---
name: harness-help
description: "Map the harness workflow and say which skill fits a situation."
disable-model-invocation: true
effort: low
argument-hint: "[situation or question about the workflow]"
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Tell the human which harness skill fits their situation and how the skills fit together. This is a read-only guide to the map below; the skills themselves hold their procedures, so point at them instead of restating them.

Question: $ARGUMENTS

## Why

The harness has many independent skills and no required path through them, so a newcomer cannot tell which one to reach for or whether any is needed. The default is still to work directly: understand the request, implement the settled scope, run the relevant checks, and report. Each skill below earns its place only when its situation shows up, and none is a prerequisite for another.

## The workflow at a glance

Each stage is optional and entered when its situation appears; the order shows where work usually flows, not a sequence to follow.

```
understand   /explain                any time, read-only
    |
intent       grilling or /grill-with-docs  when intent is unclear
             /triage                       a ticket or production error into a plan
             proto                         when an experiment can settle a question
             /spec                         for multi-session or durable design (tickets on request)
             /sharpen                      when the task itself should go deeper
    |
build        inline with tdd and ponytail (the default)
             /build                        an unattended graph of tickets or spec slices
    |
prove        verification                  checks and the certainty ladder
             /e2e-verification             release runs and feature maps
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
| You need to understand code, or why it is the way it is | `/explain` | How, why, or states; every claim cites code or history. |
| A state machine, retry, lease, or concurrent flow must always keep a rule | `/statechart` | Model-checks the rules with TLA+ and draws the states; see `docs/agents/models/reconciler/`. |
| The intent or plan is still fuzzy | `grilling` | An interview that settles the decision tree. `/grill-with-docs` also records settled terms in `GLOSSARY.md` and ADRs: use it when a later session needs the decision and cannot rebuild it from code (a greenfield project, a multi-session design), and bare `grilling` when the pull request is record enough. |
| A question an experiment can answer | `proto` | A throwaway logic model or UI variants, to answer one stated question. |
| A design worth keeping across sessions | `/spec` | A local spec with agreed test seams; tickets only when asked. |
| Writing or changing code | `tdd`, `ponytail` | Test-first loop, and the simplest thing that works. |
| Something is broken, throwing, or slow | `diagnosing-bugs` | A diagnosis loop that reproduces before it fixes. |
| Latency or throughput must improve | `/optimization` | Measure the bottleneck, change it, and measure again. |
| An unknown that needs outside sources | `research` | A sourced finding ledger; read-only. |
| A ticket or production error must become a plan | `/triage` | Checks the premise first, then returns a plan brief without implementing. |
| The task should go deeper before work starts | `/sharpen` | Names what an expert would do, the rawest sources, the methods, and an outside check, then rewrites the prompt. |
| A claim carries the whole decision | `evidence` | Start with the query that could cancel the work. |
| UI work | `impeccable` | Frontend design and critique; the project's styling layer still wins. |
| Prove a change works | `verification`, `/e2e-verification` | Revision-bound local checks; a scripted browser run with screenshots for a release. |
| Tell the human what to read, or get an independent look | `change-map`, `review` | A tiered map of the change; review a named, concrete risk without the implementer's story. |
| The architecture feels shallow or hard to navigate | `/improve-codebase-architecture` | Finds deepening opportunities and walks through the one you pick. |
| Security questions or an audit | `security-audit` | Guidance by default; the full audit runs only when an audit or pen test is requested. |
| Work spans sessions, or you are handing it over | `continuity`, `/handoff` | One recoverable task document; a handoff document for a fresh agent. |

## Skills only a human starts

`land`, `abandon`, `setup-harness`, `build`, and `retro` run only when a human types them, because they merge, discard, provision, run unattended at model-budget scale, or read session records to change the harness itself. The workflow entry points `/spec`, `/triage`, `/explain`, `/optimization`, `/e2e-verification`, and `/harness-help` are human-started too, matching their Matt Pocock and pstack equivalents. The locked `/grill-with-docs`, `/sharpen`, `/statechart`, `/handoff`, and `/improve-codebase-architecture` carry an upstream guard of their own. Starting any of them is the human's decision, so name it for them and let them invoke it.

## Model by step

`harness models` assigns the delegated roles; the session's own model is the human's `/model` and `/effort` choice. When the human asks which to use, judgment steps get the strongest model and routine steps the cheaper one, because grilling and review depend on the model's own sense of how systems break while implementation mostly follows its context:

| Step | Session model and effort |
| --- | --- |
| `grilling`, `/spec`, `/triage`, `diagnosing-bugs`, triaging `review` findings | the strongest available, high effort |
| inline building with `tdd` and `ponytail` | the session default |
| `open-pr`, `address-review`, `/land` | a mid model at medium effort |

## Crossing phases

When the work reaches a boundary between phases, such as spec to implementation or implementation to review, `continuity` holds the choice of how to cross it.

## Output

Answer the human's question with the part of the map it needs: the one to three skills that fit, why, and what usually comes next. Give the whole map only when they asked for the overview. Name the invocation (`/name`) for the skills a human must start, and leave the starting of them to the human.
