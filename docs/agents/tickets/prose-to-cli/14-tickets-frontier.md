# 14 Compute the ready frontier

## What to build

`harness tickets frontier <feature-slug> [--done <numbers>]` prints, as JSON, which tickets of a feature are ready to start, which are done, and which are blocked with the blockers each still waits on. A ticket is ready when it is not done and every ticket it is blocked by is done.

Done comes from the caller's `--done` list, which is how `build` reports the slices already integrated on its branch, and from a draft's own `Status: done` line where a local-markdown tracker keeps one.

`build` step 3 calls this command after each integration, and `local-tracker`'s Frontier section calls it for same-feature blockers.

## Acceptance criteria

- With nothing done, exactly the tickets blocked by `none` are ready.
- Marking a blocker done moves its dependents to ready only when their other blockers are done too.
- A blocked ticket lists the blockers it still waits on.
- A `--done` number that is not a ticket of the feature exits 2.
- A draft carrying `Status: done` counts as done without being named in `--done`.
- A feature whose drafts fail `harness tickets check` is refused with the check's problems.
- `build` and `local-tracker` name the command and no longer describe the computation.

## Seam

Proposed, awaiting the human's confirmation: the tickets package's tests over fixture draft directories settle the computation; one command test in `cmd/harness` proves the wiring.

## Blocked by

13

## Out of scope

Deciding whether a slice is integrated, which `build` reads from the merged work. Cross-feature dependencies, which stay prose in the ticket. Tickets published to GitHub, whose frontier the lifecycle workflow already keeps.
