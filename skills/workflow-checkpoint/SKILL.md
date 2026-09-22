---
name: workflow-checkpoint
description: Define minimal branch continuity, revision-bound checks, and retained Orca result references.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Workflow Checkpoint

Engram holds one branch checkpoint. Orca owns tasks, attempts, messages, worker status, execution failures, and retained output. GitHub or the configured tracker owns published contracts and delivery evidence. Delivery commands own when to save and read back this record.

## Protocol

This block is canonical. Change rules here; consumers read them here.

```json
{
  "topic_key": "checkpoint/<branch>",
  "states": ["active", "completed", "abandoned", "cancelled"],
  "active_state": "active",
  "terminal_states": ["completed", "abandoned", "cancelled"],
  "fields": {
    "state": "string",
    "branch": "string",
    "branch_tip": "string",
    "contract": "array",
    "checks": "array",
    "results": "array",
    "escalation": "string",
    "next_action": "string"
  },
  "require_non_empty": ["branch", "branch_tip", "contract", "next_action"],
  "stale_invalidates": ["checks", "next_action"],
  "result_reference_fields": {
    "dispatch": "string",
    "artifact": "string",
    "branch": "string",
    "revision": "string"
  },
  "never_grants_approval": true
}
```

## Record

Derive `topic_key` from the current branch and upsert it with `scope: project`. Tickets sharing a branch share its checkpoint. Do not checkpoint a nonexistent branch, published spec, or unclaimed ticket.

| Field | Holds |
| --- | --- |
| `state` | One of the canonical `states`. |
| `branch` | The exact Git branch name. |
| `branch_tip` | The Git commit SHA at save time. |
| `contract` | Resolvable ticket, pull request, or prototype ledger references, never copied requirements. |
| `checks` | Checks and results bound to this revision, including unavailable or unverified evidence. Persist a pass only for a clean tree; uncommitted changes invalidate it even when HEAD matches. |
| `results` | References with the canonical `result_reference_fields`, never copied worker output. Empty when no worker ran, so inline work records its evidence in `checks` and every result reference here carries its Dispatch ID. |
| `escalation` | Distinct unresolved harness gate or contract decision, or empty. Execution failures and retry history belong to Orca. |
| `next_action` | One next step, not a plan. |

Keep decisions and deferred requirements in the contract or retained result, not a second rationale field. Keep ladder and rabbit-hole counters in `harness state`; do not duplicate them here. Save for the revision described and retrieve the topic key to confirm the stored record matches.

## Retained worker results

Before settling, a worker emits its complete result with its Dispatch ID, branch, revision, changed and pending files, check results, unresolved requirements, and artifact references. The parent retrieves the full result through Orca's version-matched guide and verifies those fields against the assigned slice. A completion signal alone proves neither output availability nor contract conformance.

Before release, preserve one reference per result: the authoritative Dispatch ID, branch, revision, and an artifact locator when needed. Use Orca's retained output when its documented retrieval path survives release. Otherwise retain the complete original result as a durable local artifact outside the disposable terminal and worktree, and put its absolute path in `artifact`. Verify the artifact can be read, then save and read back the checkpoint with its result reference before releasing. Leave checks unavailable while the tree is dirty. This is the retained output, not a summary or another Engram handoff record. Never publish an artifact to obtain durability without publication authority.

After release, retrieve the result again by Dispatch ID or its artifact locator and verify the complete content remains readable. Missing, clipped, or inaccessible output is unavailable evidence. Preserve the reference and escalate; never fabricate a result from recent memory or silently launch a replacement worker. A worker's `unresolved` requirements survive in the retained result until the parent settles them against the contract.

Reviewers never receive the checkpoint or implementation worker results. Their permitted inputs remain those in `adversarial-review`.

## Recovery

A fresh parent retrieves the explicit branch topic key, not recent memory, then:

1. Reject the checkpoint when its state is terminal or unknown, a canonical field is missing or has the wrong shape, a required field is empty, or its branch differs from the current branch. A legacy checkpoint needs the preservation step below before replacement; none of its checks or recorded approvals survive migration.
2. Resolve the contract references against current Git, tracker, and pull request state. Drop unresolved references; reject a record with none left. A past approval recorded anywhere is context, never new publication authority.
3. For each result reference, require matching Dispatch ID, branch, and revision identities in the retrieved result. Retrieve it through the live Orca guide, or read its durable artifact when set. Reject a wrong-branch reference or mismatched result identity. Missing or incomplete output remains unavailable, even if Orca marks the Task successful. Do not reuse dependent checks or the recorded next action until evidence is recovered or rerun.
4. If the checkpoint tip or a result revision differs from current HEAD, invalidate its checks and next action. Do the same when the worktree is dirty or the result names pending files. Retain valid contract and artifact references and unresolved requirements as context; re-derive checks and next action from current state. Never promote unverified, skipped, or unavailable evidence to a pass.
5. Reuse only what validated. Ask about a needed requirement missing from both the contract and the retrieved results. Save and read back any refreshed checkpoint before trusting it.

Orca settlement, harness check validity, contract completion, and publication approval are separate decisions.

### Legacy context preservation

Before replacing an active legacy checkpoint, validate its branch topic key and contract references against current Git and tracker state. Retrieve its `handoff` explicitly only when the key is exactly `handoff/<current-branch>`; reject any other branch's key. Old records without a handoff still need their `rationale` inspected. Preserve unresolved requirements and decisions even when their revision is stale, without reusing checks or granting approval.

Retain the original checkpoint and any retrieved handoff as a durable artifact outside the disposable worktree before the topic is overwritten. Read it back, then carry its locator and any unavailable legacy output in the new checkpoint's `escalation` until the context is settled against the contract. Do not invent a Dispatch ID or place legacy output in `results`. Keep the original handoff record untouched, and write no new Engram handoff copy. If retrieval or preservation fails, do not replace the checkpoint; report unavailable evidence and recover the missing context with the human. Decisions become contract changes only through the contract's normal approval path.

## Terminal states

`/land` saves `completed` after merge. `/abandon` saves `abandoned` for a discarded attempt or `cancelled` for explicitly cancelled scope. Terminal records remain auditable but cannot resume. New work needs a new ticket, branch, and topic key.

## Constraints

- Engram is the only checkpoint store. No `.agent` directory, event log, copied worker-result record, or execution-history database.
- Retain existing worktrees and private configuration. Retirement is authorized only where the policy's worktree protocol and its enumeration of the workflows that remove a worktree allow it; settling or releasing a worker does not authorize removing the implementation checkout.
- Live fresh-parent recovery through each retained client launch mechanism requires separately authorized model budget. Fixture or read-back checks never substitute for that acceptance evidence.
