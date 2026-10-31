---
name: research
description: "Investigate a question or claim against primary sources and return a sourced finding ledger. Use for \"look into X\", \"is it true that\", or a question needing external or telemetry evidence."
argument-hint: "[question or claim]"
---

Investigate a question or claim in depth and return a sourced finding ledger. This is read-only work: it reads, queries, and may launch read-only `code-search` workers, and it writes no files, edits no code, changes no configuration, and makes no commits. The one exception is a disposable bisect worktree in step 3.

Target: $ARGUMENTS

## Why

Depth means stronger evidence and traced consequences, not a longer answer or unrelated exploration. Keeping the investigation read-only lets its findings stand as evidence while implementation stays a separate decision, and the ledger is planning input rather than an implementation contract.

With no target above, use the latest unambiguous active question, claim, or unresolved statement in the conversation, and treat a prior recommendation as context rather than proof. Ask only when no research target exists or the ambiguity would materially change the investigation.

## Steps

1. Frame the target. Name the central question, or break the statement into claims that can be verified. State a scope or interpretation assumption only where it is load-bearing.

2. Trace code paths, state ownership, lifecycle, history, tests, docs, and external behavior beyond the first plausible answer until evidence supports it. Prefer primary sources: official docs, code, specs, and first-party APIs. Verify time-sensitive facts at the source, never from search snippets or secondary summaries.

   Telemetry is a primary source on the same footing as code. Read and follow `evidence` for any claim about runtime behavior, impact, frequency, or performance.

3. Bound suspected regressions with telemetry or release history, then `git bisect` in a disposable worktree: create a sibling worktree (`git worktree add`) at the suspected revision and bisect there. Use the cheapest decisive test, profile, or trace per candidate. If a probe cannot decide, report the narrowed range. This is your own disposable experiment, so remove only that worktree afterward (POLICY's floor row on worktrees and its Git section) and leave the main tree and repository history as they were.

4. Compare independent evidence, test counterexamples, and resolve contradictions. Separate fact, inference, and unknown. Runtime claims not checked against available telemetry belong in `Contradictions and unknowns`, not `Conclusion`.

5. Delegate parallel repository search only for genuinely independent tracks or a large search space. Launch each track as the `code-search` role (POLICY's Delegation section); when the role is unavailable, search inline and say so. Give each a distinct question and reconcile their evidence rather than concatenating their reports. A track searching the repository reports in the shape described under Code-search citations below, and every citation is verified there before reuse; external and telemetry queries stay in this context and keep the Findings ledger's `Evidence:` form.

## Code-search citations

`/triage` handles `code-search` results the same way. `roles/code-search.md` defines the worker's report format, one `path:line | symbol | mechanism` line per finding.

The worker ends on a text message holding only its findings. A trailing tool result is not the report: re-run that track once, and if it again ends without one, record it as no data (a `no data` row in `/triage`'s evidence ledger or an entry under `/research`'s `Contradictions and unknowns`), since a missing report is not a refutation and not a track that found nothing.

Before reusing a citation, read only the cited line and confirm the path resolves inside the repository and that the line carries the named symbol as a whole identifier. That check grounds the location only, not the claimed runtime mechanism: a load-bearing mechanism claim still goes through `evidence`.

Drop a citation whose path is missing or outside the repository, whose line does not carry its symbol, or that lacks a field. Record it as refuted (a verdict row in `/triage`'s evidence ledger or an entry under `/research`'s `Contradictions and unknowns`) and keep it out of the grounded evidence.

## Output

- **Conclusion**: the answer or verdict first, with its confidence.
- **Findings**: a numbered ledger. Each finding is atomic and independently actionable, because `/create-ticket` consumes these by id:

  ```
  F1. <one-sentence claim, falsifiable>
      Evidence: path:line, https://...  (plus the number and the window when it is telemetry)
      Confidence: confirmed | likely | unclear
      Actionable: yes, <one line naming the smallest change that would resolve it> | no, <why>
  ```

  `Actionable` names the smallest change in one line. It is not a plan.
- **Evidence**: exact `path:line` for repository evidence and the complete absolute URL for external evidence.
- **Contradictions and unknowns**: unresolved conflicts, weak evidence, and what would change the conclusion. Omit when there are none.
- **Implications**: what the result means for the active discussion. Do not turn this into an implementation plan unless asked.

## Handoff

Recommend `/create-ticket findings F2,F4` with the actual finding ids when warranted, then stop; publishing tickets and implementing are separate requests.

## Skills this loads

- `evidence`, read from its path, for any load-bearing claim about runtime behavior, impact, or frequency
- `diagnosing-bugs` when the question is why something breaks and the first two evidence queries do not settle it
