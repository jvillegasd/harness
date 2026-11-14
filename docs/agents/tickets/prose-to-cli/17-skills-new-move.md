# 17 Create and move a local skill

## What to build

`harness skills new <name> <group> [--guarded]` creates a local skill in its group with a starting body and both bookkeeping links, so a new skill passes the link check without anything made by hand. `--guarded` adds all three invocation guards, one per client, so the skill is never started by the model on its own.

`harness skills move <name> <group>` moves a skill's body to another group, keeping its history, and repoints its link. A locked body keeps matching its lock.

Both refuse a name that already exists in any group, a group that does not exist, and a move onto the group the skill is already in. `AGENTS.md` stops telling the reader to link a skill by hand.

## Acceptance criteria

- After `skills new demo support`, the body exists in the `support` group, both links resolve to it, and `harness skills verify` passes.
- After `skills new demo ops --guarded`, the skill carries all three guards and the guard check reports nothing missing.
- Without `--guarded`, the skill carries none of the three.
- `new` with a name that exists in another group is refused and writes nothing.
- After `skills move demo ops`, the body is in `ops`, both links resolve, and nothing is left in the old group.
- Moving a locked skill leaves `harness skills verify` reporting no drift.
- `move` to an unknown group, or to the skill's current group, is refused and changes nothing.
- `AGENTS.md` names the commands in place of the hand-linking instructions.

## Seam

The skills package's tests over throwaway checkouts settle creation and moves; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Writing the skill's content. Adding a skill to the list that must always be guarded. Third-party skills, which stay with `harness skills add`.
