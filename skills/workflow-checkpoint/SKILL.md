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
  "handoff_key": "handoff/<branch>",
  "states": ["active", "completed", "abandoned", "cancelled"],
  "active_state": "active",
  "terminal_states": ["completed", "abandoned", "cancelled"],
  "fields": {
    "state": "string",
    "branch_tip": "string",
    "contract": "array",
    "checks": "array",
    "rationale": "string",
    "handoff": "string",
    "next_action": "string"
  },
  "require_non_empty": ["contract", "next_action"],
  "stale_invalidates": ["checks", "next_action"],
  "handoff_fields": {
    "slice": "string",
    "revision": "string",
    "changed_files": "array",
    "pending_files": "array",
    "checks": "array",
    "unresolved": "array",
    "artifacts": "array"
  },
  "handoff_never_grants_approval": true
}
```

## Topic key

Upsert one checkpoint per active implementation branch under `topic_key` with `scope: project`. Tickets sharing a branch share its checkpoint. Do not checkpoint a nonexistent branch, published spec, or unclaimed ticket. A worker handoff is a second record under `handoff_key`, never a second checkpoint.

## Record

Use exactly the canonical `fields` and their declared shapes:

| Field | Holds |
| --- | --- |
| `state` | One of the canonical `states`. |
| `branch_tip` | The git commit sha of the branch head at save time. |
| `contract` | Durable references, never copied text: a ticket URL or path, a pull request URL, or a prototype ledger path. `require_non_empty`. |
| `checks` | Resolved ladder results for this tip. Valid only while the tip matches. |
| `rationale` | The narrow exception for a decision that exists nowhere durable yet. Keep the key; set it empty when the contract already records the rationale. |
| `handoff` | The topic key of the retained worker handoff under `handoff_key`, or empty when no worker ran. |
| `next_action` | One step, not a plan. `require_non_empty`. |

Save the record for the branch tip it describes, and read it back before trusting it.

## Worker handoffs

A parent that delegates a slice owns the branch checkpoint; a worker owns only its handoff. The worker composes the handoff with the `handoff` skill into a recipient-focused document and returns it to the parent; the parent persists its unique context under `handoff_key` with `scope: project` and reads it back. A save with no read-back is unverified.

The handoff record uses exactly the canonical `handoff_fields`:

- `slice`: the one slice the worker built.
- `revision`: the git commit sha the handoff describes.
- `changed_files`: the files the worker changed.
- `pending_files`: the files left uncommitted or pending.
- `checks`: the checks the worker ran, each with its result.
- `unresolved`: the questions the worker could not settle.
- `artifacts`: durable references to the worker's artifacts, never copied text.

A handoff reports work and never grants approval; the parent or a human decides. The parent records the handoff's topic key in the checkpoint's `handoff` field. A `handoff` that names any key other than this branch's `handoff_key` is rejected on recovery. A handoff never overwrites or replaces the parent-owned checkpoint, which stays the branch's only continuity record.

Recovery reads the handoff by its key, never from recent memory, and validates it against the checkpoint, current Git state, and the contract. When `revision` differs from the current tip, or `pending_files` is non-empty, the handoff's `checks` are invalidated and re-derived. A handoff that fails to persist, or whose read-back does not match, is reported as an incomplete durable handoff, preserving whatever succeeded. Reviewers never receive the handoff; it carries implementation rationale and stays with the parent.

## Recovery

Retrieve the branch checkpoint explicitly; recent memory may belong to another branch.

1. Derive the topic key from the current branch and retrieve it. When its `handoff` field is set, retrieve the handoff at that key and validate it as the Worker handoffs section requires, against the checkpoint, Git state, and the contract.
2. Reject the record, reusing nothing, when any of these holds:
   - `state` is terminal or outside the canonical set (`active_state` plus `terminal_states`).
   - A field is missing or its value does not match its canonical shape, except `handoff`, which reads as empty when the record predates the field.
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
