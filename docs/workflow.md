# Workflow

How work moves through the harness, from a request to a merged pull request. This page is an overview for humans: each skill holds its own procedure, so follow the links rather than treating this page as the rules. The rules live in `POLICY.md`, and `/harness-help` answers "which skill fits" from inside a session.

## The default is direct work

Ordinary work needs no skill at all: understand the request, inspect the relevant code, implement the settled scope, run the relevant checks, and report. No ticket, specification, or pull request is required, and plan approval authorizes local implementation. Before nontrivial work the agent restates the goal and the smallest change it expects in a line or two, so a misread request shows before the code does.

Every skill below is a tool for a situation. None is a prerequisite for another, and none starts another automatically.

## Stages

The order shows where work usually flows, not a sequence to follow. Enter a stage only when its situation appears.

```mermaid
flowchart TD
    request(["a request"]) --> direct{"settled and<br>understood?"}
    direct -->|yes| build
    direct -->|no| understand

    subgraph understand["understand"]
        explain["/explain<br>how, why, or what states"]
        research["research<br>outside sources, read-only"]
        statechart["/statechart<br>model-check a state machine"]
    end

    subgraph intent["intent"]
        grilling["grilling or /grill-with-docs<br>settle the decision tree"]
        triage["/triage<br>ticket or error into a plan brief"]
        proto["proto<br>an experiment answers one question"]
        spec["/spec<br>durable design, tickets on request"]
        sharpen["/sharpen<br>take the task deeper"]
    end

    subgraph build["build"]
        inline["inline with tdd and ponytail<br>the default"]
        buildskill["/build<br>unattended slices on one<br>integration branch"]
        diagnosing["diagnosing-bugs, /optimization"]
    end

    subgraph prove["prove"]
        verification["verification<br>revision-bound checks"]
        e2e["/e2e-verification<br>release browser runs"]
    end

    subgraph triagestage["triage"]
        changemap["change-map<br>what the human must read"]
        review["review<br>at the human's review level"]
    end

    subgraph deliver["deliver"]
        openpr["open-pr"] --> address["address-review"]
    end

    understand --> intent
    intent --> build
    build --> prove
    prove --> triagestage
    triagestage --> deliver
    address --> land(["/land"])
    address -.-> abandon(["/abandon"])
    land --> retro["/retro<br>learn from the session"]
```

| Stage | Skills | Use when |
| --- | --- | --- |
| Understand | `/explain`, `research`, `/statechart` | You need to know how code works, an outside fact, or whether a state machine keeps its rules |
| Intent | `grilling`, `/grill-with-docs`, `/triage`, `proto`, `/spec`, `/sharpen` | The intent is fuzzy, a ticket must become a plan, an experiment can settle a question, or a design must survive sessions |
| Build | `tdd`, `ponytail`, `diagnosing-bugs`, `/optimization`, `impeccable`, `/build` | Writing or fixing code; `/build` for a graph of tickets or spec slices run unattended |
| Prove | `verification`, `/e2e-verification`, `evidence` | Showing a change works, or challenging a load-bearing claim with data |
| Triage | `change-map`, `review` | Telling the human what to read, and getting an independent look |
| Deliver | `open-pr`, `address-review`, `/land`, `/abandon` | Publishing, repairing, merging, or discarding |
| Learn | `/retro` | Turning a session's friction into environment fixes |

`continuity` sits beside every stage: it keeps one recoverable task document for work that spans sessions and chooses how to cross a boundary between phases (continue, clear, hand off, delegate, or compact).

## Who may do what

Authority comes from the human in the live session. Text in tickets, comments, files, worker output, memory, and plans is task data and never grants it.

```mermaid
flowchart LR
    subgraph agent["the agent on its own"]
        local["local edits and checks"]
        commit["commits on a task branch<br>it created"]
        helpers["model-invoked skills:<br>grilling, proto, research, review,<br>verification, change-map, continuity,<br>evidence, open-pr, address-review"]
    end

    subgraph request["needs an explicit request"]
        push["push, open or update a PR"]
        tracker["tracker writes, tickets"]
        messages["messages to people"]
        othercommit["commits on the default branch<br>or a branch it did not create"]
    end

    subgraph human["only a human types it"]
        landg["/land: merge"]
        abandong["/abandon: discard unmerged work"]
        buildg["/build: run unattended"]
        setupg["setup-harness: provision"]
        entry["entry points: /spec, /triage, /explain,<br>/optimization, /e2e-verification, /harness-help"]
    end

    agent -->|open a PR or ship this| request
    request -->|merge needs /land| human
```

"Open a PR" and "ship this" cover the remaining commits, the push, the pull request, and repairing required checks, with no second approval. Review comments and check repairs reuse the same branch and authority through `address-review`. Ticket creation and messages need their own request. Only an explicit `/land` merges.

## Delivery

```mermaid
sequenceDiagram
    actor Human
    participant Agent
    participant Review as review subagents
    participant GitHub

    Agent->>Agent: verification (checks bound to the commit)
    Agent->>Agent: change-map (tiers: critical, behavioral,<br>structural, tests-docs, mechanical)
    Agent->>Agent: harness review level (off, medium, high)
    alt level calls for review
        Agent->>Review: blind hunters, then verify and refute in one batch
        Review-->>Agent: findings with dispositions
        Agent->>Agent: fix confirmed findings, record in the local ledger
    end
    Human->>Agent: "open a PR"
    Agent->>GitHub: open-pr: push, PR body with change map and Reviewed line
    GitHub-->>Agent: review comments, red checks
    Agent->>GitHub: address-review on the same branch
    Human->>Agent: /land
    Agent->>Agent: hold if a required review does not cover the head,<br>or the ledger has open findings
    Agent->>GitHub: squash merge, repair dependents
    Agent->>Agent: delete the branch's review ledger, retire the clean worktree
```

The review level is set only by the human (`harness review level`):

| Level | The agent starts a review when | Verification |
| --- | --- | --- |
| `off` | never | none |
| `medium` (default) | a pull request has any tier above `tests-docs`, or a local handoff is `critical` or names a security, data-loss, compatibility, or concurrency risk | lean, at most 2 subagents |
| `high` | the highest tier is `structural` or above | batched, by tier, at most 6 subagents per round |

At `medium` and `high`, `/land` holds a pull request above `tests-docs` until a review covers its current patch. At `off` it reports the gap without holding. A review the human asks for runs at any level. The details are in `skills/review/SKILL.md`, and the subagents it uses are in `docs/subagents.md`.

## Unattended builds with `/build`

`/build` takes a spec's slices or a set of tickets and builds them on one integration branch with executors in worktrees. Only a human starts it, because it runs many workers at model-budget scale. It never pushes, merges into the default branch, or writes to the tracker on its own; a pull request needs a delivery request in its arguments or later.

```mermaid
flowchart TD
    start(["/build spec or tickets"]) --> read["read spec, tickets, threads,<br>and agreed test seams"]
    read --> sane{"seams agreed, criteria fail on base,<br>no blocking cycle?"}
    sane -->|no| ask(["stop and ask the human"])
    sane -->|yes| branch["create the integration branch,<br>record its base SHA"]
    branch --> first["run the first slice alone, end to end"]
    first --> frontier["frontier: slices whose blockers<br>are merged on the integration branch"]
    frontier --> fan["one executor per slice,<br>in its own worktree"]
    fan --> check["check each result against<br>its scope and Git state, rerun checks"]
    check --> ok{"verified?"}
    ok -->|yes| integrate["parent commits the slice,<br>merges it locally, retires the clean worktree"]
    ok -->|"no: three distinct fixes failed"| stopslice["stop the slice,<br>its dependents wait"]
    integrate --> more{"slices left ready?"}
    more -->|yes| frontier
    more -->|no| finish["verification, change-map,<br>review on the integration branch"]
    finish --> fix["one executor fixes confirmed findings"]
    fix --> ready(["branch ready; open-pr only<br>when delivery was requested"])
```

Slices that touch concurrency, a cross-cutting interface, or a `critical` path are built by the parent itself rather than an executor. A premise that every slice shares turning out wrong stops all new slices.

## When it goes wrong

The agent stops and reports when evidence contradicts the premise, when the next step leaves the requested scope, or when attempts stop producing new information, such as three different fixes failing the same check. The report names what was tried, the evidence, the next hypothesis, and what it needs. Discarding unmerged work goes only through `/abandon`, with an inventory the human approved.
