---
name: adversarial-review
description: Review before pushing with requirements-aware and diff-only reviewers; triage findings and adjudicate contested blockers.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Adversarial Review

Withhold implementation rationale so reviewers judge the diff independently.

## Invocation

Launch each reviewer as its named agent, in parallel, each with a fresh context: `reviewer-a` and `reviewer-b`. The installer generates them from `roles/reviewer-a.md` and `roles/reviewer-b.md` beside this skill, which own each reviewer's standing instructions, lenses, and output format.

Never pass the conversation, implementation rationale, implementation plan, or prior rounds.

Read the locked `orchestration` skill before launching a role, and load the version-matched guide from the Orca executable it resolves. The coordinator uses Orca commands directly and follows that guide for the complete supervised loop, including worker messages, settlement, recovery, retries, output retrieval, and cleanup. Do not implement a separate polling loop, reason taxonomy, or retry policy in harness code or prose.

Resolve the role's assignment with `harness models resolve <role> --json`. Preserve its named agent, assigned client/model/effort, and permissions when following Orca's documented custom-terminal adoption path. Run `harness models launch-args <role> --orca-command <executable>` to validate the installed role and obtain its client argv as JSON. Pass the exact executable selected by the locked skill; never resolve a second executable. Keep the preparation binary available for the worker’s lifetime, because its permission hook calls it. Use that argv in the guide's custom-terminal adoption path, quoting each argument for the terminal's shell. This read-only preparation launches nothing and implements no Orca protocol. The read-only role's hook permits only literal `orchestration send`, `ask`, and `check` commands for that executable. Follow the live preamble's exact authority arguments; these three command permissions grant no coordinator powers or task scope. If the preamble requires a different executable or operation, report the role unavailable instead of weakening the guard. Keep message prose free of shell syntax, even when quoted.

Launch independent reviewers before running the coordinator's conformance pass, and process their Orca messages while they work. Keep the existing reviewer input boundaries: Orca's live lifecycle preamble is transport context, never implementation rationale or extra task scope. Do not repair or commit until all dispatched reviewers have settled and their complete reports have been retrieved through Orca.

When Orca reports an unavailable or uncertain operation, follow its version-matched recovery guide and receipt. Preserve its task, dispatch, and request identities. A required review waits or escalates; it never becomes inline or switches backend. An executor may work inline only when the policy permits and Orca has established that no earlier worker can still act. Inspect partial edits before repeating a failed implementation slice. Report unavailable review evidence under `## Checks`, or in the `/proto` ledger.

A missing or stale named agent requires a human to run `harness install`; never provision the machine to unblock a review or substitute a general agent. Preserve completed output through Orca's retained references, verifying retrieval before and after release. When retention is not guaranteed, save the complete original report as a durable artifact outside the disposable terminal and worktree before release, and retain its locator. Treat a clipped, missing, or unreadable report as unavailable evidence, never as a successful review.

The named agents' restrictions are best effort: each client enforces what its own controls support, and `harness install` reports the gaps.

Give Reviewer A the requirements, the diff snapshot path, and the exact absolute path of the project's own instruction file as its standards, never the implementation rationale. Nothing else.

Give Reviewer B only the diff snapshot path and the exact absolute paths of its review standards: the loaded `ponytail` body and the project's own instruction file. No requirements, no rationale, no ticket.

Standards are paths, never requirements or rationale, so supplying them leaves each reviewer's information boundary where it was. Count a standards file as read only when the reviewer's `loaded:` line names it: supplying a path, or a reviewer recalling what a standard says, is not evidence that it was read.

The project's own instruction file is `AGENTS.md` at the repository root, or `CLAUDE.md` there when no `AGENTS.md` exists, and is left out when neither does. Where the client is known to refuse a standards path, such as OpenCode for a file outside the project, paste that file's text into the first launch instead of its path. When a supplied standards file is missing from a reviewer's `loaded:` line, relaunch that reviewer once with that file's text pasted in; the relaunch replaces that reviewer's output for the same round and never counts toward the cap. A reviewer names a pasted standard as `inline:<absolute path>`. If a standard is still unread, disclose it under the pull request's `## Checks`, or in the requirements ledger under `/proto`.

This block defines permitted inputs. Withhold everything else; use absolute standards paths.

```json
{
  "inputs": {
    "reviewer-a": ["requirements", "diff_snapshot_path"],
    "reviewer-b": ["diff_snapshot_path"],
    "review-refuter": ["contested_finding", "diff_snapshot_path"]
  },
  "standards": {
    "reviewer-a": ["project_instruction_file"],
    "reviewer-b": ["ponytail", "project_instruction_file"],
    "review-refuter": []
  },
  "optional_standards": ["project_instruction_file"],
  "loaded_prefix": "loaded: ",
  "inline_prefix": "inline:",
  "verdict_prefix": "verdict: ",
  "unreadable_prefix": "unreadable: "
}
```

Scale to blast radius:

- Use Reviewer A alone for a single-file diff under roughly 50 changed lines touching no data, auth, concurrency, migration, or public-contract surface. Keep its input boundary unchanged.
- Two reviewers otherwise.
- Never three.

## Diff delivery

Snapshot the complete diff once per round into a readable temporary file outside the worktree, and give each reviewer that path. A reviewer that answers `unreadable: <path>` gets the same snapshot inline instead. Never truncate the diff: when neither the file nor inline text can carry the complete snapshot, stop and escalate per the escalation contract.

Reviewers and the refuter never edit files, and neither findings nor verdicts reach the forge.

## Triage, by the implementer

Every consumer follows this canonical triage block. The prose explains it without adding rules.

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
  "refuter": {
    "applies_to": ["bug", "risk"],
    "required_before": "reject",
    "invocations_per_finding": 1,
    "counts_as_round": false,
    "escalation_closes_on": "human_decision",
    "evidence_required": ["corroborated", "refuted"],
    "verdicts": {
      "corroborated": "fix",
      "refuted": "reject",
      "inconclusive": "escalate",
      "missing": "escalate",
      "malformed": "escalate",
      "unavailable": "escalate"
    }
  }
}
```

Only `bug:` and `risk:` block. For each, exactly one of:

- Fix it, with the smallest change that resolves it, per `ponytail`.
- Reject it, only on the refuter's `refuted` verdict or a human decision on its escalation, recording that verdict and its evidence, or that decision, in the pull request body under `## Rejected review findings`, or in the requirements ledger under `/proto`.
- Escalate it, per the escalation contract, when it is a product decision.

A `nit:` is fixed only when the fix is smaller than the argument, and otherwise dropped. A nit fix never starts another review round.

A `q:` is answered. It becomes a `bug:` or `risk:` only when evidence exposes a defect, and is then triaged as one. A question alone never blocks delivery.

## Refuter

Before rejecting a contested `bug:` or `risk:`, launch the named `review-refuter` agent once for that finding, with a fresh context. The installer generates it from `roles/review-refuter.md` beside this skill, and it runs on its central assignment, launched through the Orca orchestration instructions above. A refuter that cannot be launched is the `unavailable` verdict, which escalates.

Give the refuter only the contested finding, verbatim, and the path of the diff snapshot that finding was raised against. Never the implementation rationale, the requirements, your reason for contesting it, or the conversation. A refuter that answers `unreadable: <path>` gets that snapshot inline under Diff delivery, and the relaunch completes the same invocation rather than adding one.

Its verdict decides the finding:

- `corroborated`: fix it, subject to the escalation contract's scope and safety rules.
- `refuted`: reject it, recording the verdict and the refuter's `path:Lline:` evidence.
- `inconclusive`, `missing`, `malformed`, or `unavailable`: escalate for evidence or a human decision. Never dismiss the finding, and never make a speculative fix.

A finding requiring an unseen requirement or standard is `inconclusive`. Judge the snapshot, never a later worktree. A human's decision on an escalated finding closes it, recorded like a rejection.

A result is `missing` if empty; `malformed` if its first line is not a `verdict:` line naming `corroborated`, `refuted`, or `inconclusive`, or a `corroborated` or `refuted` verdict lacks a `path:Lline:` evidence line; `unavailable` if the refuter cannot be launched or the inline retry cannot carry the snapshot. Allow only one `unreadable:` answer, preceding its retry, per invocation.

The refuter is not a reviewer and not a round: it changes neither count nor cap and runs once per contested finding, never for a second opinion. A fix after `corroborated` triggers the usual fixed-blocker rule.

Do not claim contest frequency, adjudication cost, or verdict accuracy without evidence. Session cost and friction do not prove correctness.

## Loop cap

At most two rounds. A second round runs only after a fixed blocking finding, and it reviews the repair delta alone. A new blocking finding in round two, in code that round one's fixes touched, is a rabbit-hole trip: stop and escalate rather than starting round three.

A conformance repair counts as a fixed blocking finding for the second-round trigger. After round two, a conformance repair is rechecked by the conformance pass alone and disclosed under `## Checks`, never given a third round.
