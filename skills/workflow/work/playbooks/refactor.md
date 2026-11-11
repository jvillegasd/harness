# Refactor

**You own the contract.** The structure changes and the behavior does not. When the work turns up a bug or a missing feature, split it out and ship the structural change first against the pinned contract. A redesign is allowed, but name it and match it as a feature. Large or cross-cutting structural work is the [large](large.md) playbook.

1. Pin the behavior before moving anything. Run `how` over the affected subsystem to learn the contract, then write a characterization test, snapshot, or equivalence harness that captures current behavior. Type checks and lint are not a pin.
2. Name the structure the code is missing (`principle-model-the-domain`). The reshape deletes branches or invalid states; it does not add indirection. Code whose shape is already clear and local stays.
3. Name the target shape: the module layout, types, and call graph you would build today (`principle-foundational-thinking`, `principle-redesign-from-first-principles`). When it crosses a function boundary, run [design](design.md)'s step 2 first.
4. Subtract before adding: delete dead code, collapse one-caller wrappers, and drop redundant validators before the new shape goes in (`principle-subtract-before-you-add`, `principle-laziness-protocol`). Revert any speculative cleanup.
5. Move in small steps, each keeping the pin green. Migrate every caller and delete the old API in the same wave (`principle-migrate-callers-then-delete-legacy-apis`), with no shims or parallel paths. Check every rename against strings and prose, where usages hide. Hand mechanical edits to the `executor` role through `delegation`, with an exact scope: the paths, the names that move, and the behavior to hold.
6. Prove the behavior is unchanged on the real artifact (`principle-prove-it-works`, `verification`): the pin, plus an equivalence check for a larger reshape. "It compiles" is not proof.
7. Check that reader load went down somewhere (`principle-minimize-reader-load`). When it did not, revert.
8. Commit in order: the subtraction, then the reshape, then follow-on cleanup (`principle-sequence-verifiable-units`).
9. Run `change-map`, then `review` at the level `harness review level` reports.
10. When delivery is authorized, run `open-pr`. Otherwise stop with the work ready locally and offer delivery once.

**Reply:** the structure that changed, the pin it held against, the equivalence proof, the reader-load change, and what was reverted.
