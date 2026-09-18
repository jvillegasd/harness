---
name: workflow-checkpoint
description: Define branch-scoped Engram checkpoints: keys, schema, recovery validation, and terminal states.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Workflow Checkpoint

Use this shared Engram protocol for continuity across sessions. Delivery commands own when to save and read back; they reference this schema rather than duplicate it.

## Protocol

This block is canonical. Change rules here; implementations read them here.

```json
{
  "topic_key": "checkpoint/<branch>",
  "states": ["active", "completed", "abandoned", "cancelled"],
  "active_state": "active",
  "terminal_states": ["completed", "abandoned", "cancelled"],
  "fields": {
    "state": "string",
    "branch_tip": "string",
    "contract": "array",
    "checks": "array",
    "rationale": "string",
    "next_action": "string"
  },
  "require_non_empty": ["contract", "next_action"],
  "stale_invalidates": ["checks", "next_action"]
}
```

## Topic key

Upsert one checkpoint per active implementation branch under `topic_key` with `scope: project`. Tickets sharing a branch share its checkpoint. Do not checkpoint a nonexistent branch, published spec, or unclaimed ticket.

## Record

Use exactly the canonical `fields` and their declared shapes:

| Field | Holds |
| --- | --- |
| `state` | One of the canonical `states`. |
| `branch_tip` | The git commit sha of the branch head at save time. |
| `contract` | Durable references, never copied text: a ticket URL or path, a pull request URL, or a finalized prototype ledger path. `require_non_empty`. |
| `checks` | Resolved ladder results for this tip. Valid only while the tip matches. |
| `rationale` | The narrow exception for a decision that exists nowhere durable yet. Keep the key; set it empty when the contract already records the rationale. |
| `next_action` | One step, not a plan. `require_non_empty`. |

Save the record for the branch tip it describes, and read it back before trusting it.

## Recovery

Retrieve the branch checkpoint explicitly; recent memory may belong to another branch.

1. Derive the topic key from the current branch and retrieve it.
2. Reject the record, reusing nothing, when any of these holds:
   - `state` is terminal or outside the canonical set (`active_state` plus `terminal_states`).
   - A field is missing or its value does not match its canonical shape.
   - A `require_non_empty` field is empty.
   - No `contract` reference resolves against current Git, tracker, or pull request state. Drop unresolved references.
3. When the record survives validation, drop the `stale_invalidates` fields if `branch_tip` differs from the current head, and re-derive them from the surviving contract. The contract references and the rationale survive a moved tip.
4. Reuse only what validated. When a requirement the work needs is missing from the validated record and the contract, ask about it rather than inferring it.
5. After saving, retrieve the topic key again and confirm the record matches what was written. A save with no read-back is unverified.

## Terminal states

`/land` saves `completed` after merge. `/abandon` saves `abandoned` for a discarded attempt or `cancelled` for explicitly cancelled scope. Terminal records remain auditable but cannot resume. New work needs a new ticket, branch, and topic key.

## Constraints

- No `.agent` directory, event log, or second continuity store. Engram is the only store.
- Do not duplicate a ticket, pull request, or prototype contract into the checkpoint; reference it.
- Do not create a checkpoint for a published spec or an unclaimed ticket.
