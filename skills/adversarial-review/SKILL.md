---
name: adversarial-review
description: Review before pushing with one blind reviewer that holds the requirements but never the implementation rationale; triage its findings.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Adversarial Review

Withhold implementation rationale so the reviewer judges the diff independently.

## Invocation

Launch the named `reviewer` agent once per round with a fresh context, following the policy's **Launching a role**. The installer generates it from `roles/reviewer.md` beside this skill, which owns its lenses and output format. Never pass the conversation, the implementation rationale, the plan, or prior rounds.

Give the reviewer:

- the requirements;
- the complete diff snapshot;
- its standards: the `ponytail` body and the project's own instruction file (`AGENTS.md` at the repository root, else `CLAUDE.md` there, else none).

Give a standards file inside the project by absolute path. Paste the text of one outside it, such as the installed `ponytail` body, because a read-only role cannot read outside the project. Count a standard as read only when the reviewer's `loaded:` line names it; relaunch once with a missing file's text pasted in (the relaunch replaces that round's output and never counts toward the cap), and disclose a standard still unread under `## Checks`.

```json
{
  "inputs": {"reviewer": ["requirements", "diff_snapshot"]},
  "standards": {"reviewer": ["ponytail", "project_instruction_file"]},
  "optional_standards": ["project_instruction_file"],
  "loaded_prefix": "loaded: ",
  "inline_prefix": "inline:"
}
```

A missing or stale named agent requires a human to run `harness install`; never substitute a general agent. A clipped, missing, or unreadable report is unavailable evidence, never a passing review: rerun once, then escalate.

## Diff delivery

Snapshot the complete diff once per round into a temporary file outside the worktree, and paste its complete text into the launch prompt. Never truncate it: when a launch prompt cannot carry the complete snapshot, stop and escalate per the escalation contract. The reviewer never edits files, and its findings never reach the forge.

## Triage, by the implementer

```json
{
  "severities": {
    "bug": {"blocking": true, "actions": ["fix", "reject", "escalate"]},
    "risk": {"blocking": true, "actions": ["fix", "reject", "escalate"]},
    "nit": {"blocking": false, "actions": ["fix_when_smaller_than_argument", "drop"]},
    "q": {"blocking": false, "actions": ["answer", "reclassify_on_evidence"]}
  },
  "reclassify_to": ["bug", "risk"],
  "second_round": {"trigger": "fixed_blocking_finding", "scope": "repair_delta"},
  "conformance_repair": "counts_as_fixed_blocking_finding",
  "conformance_repair_after_cap": "recheck_and_disclose",
  "max_rounds": 2,
  "new_blocking_in_repaired_code": "escalate",
  "reject_requires": "path_line_evidence"
}
```

Only `bug:` and `risk:` block. For each, exactly one of:

- Fix it, with the smallest change that resolves it, per `ponytail`.
- Reject it, only with concrete `path:line` evidence that the finding is wrong, recorded in the pull request body under `## Rejected review findings`, or in the `/proto` ledger. Disagreement without evidence is not a rejection.
- Escalate it, per the escalation contract, when it is a product decision or the evidence is inconclusive.

A `nit:` is fixed only when the fix is smaller than the argument, and otherwise dropped. A nit fix never starts another round. A `q:` is answered; it becomes a `bug:` or `risk:` only when evidence exposes a defect.

## Loop cap

At most two rounds. A second round runs only after a fixed blocking finding, and it reviews the repair delta alone. A new blocking finding in round two, in code that round one's fixes touched, is a rabbit-hole trip: stop and escalate rather than starting round three.

A conformance repair counts as a fixed blocking finding for the second-round trigger. After round two, a conformance repair is rechecked by the conformance pass alone and disclosed under `## Checks`, never given a third round.
