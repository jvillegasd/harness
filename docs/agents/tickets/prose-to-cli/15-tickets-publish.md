# 15 Publish ticket drafts to GitHub

## What to build

`harness tickets publish <feature-slug> [--dry-run]` publishes a feature's drafts as GitHub issues in an order and form that leave no dangling edge and no duplicate after a failure.

- `--dry-run` prints the exact write set: each issue to create, each blocking edge, each label.
- Blockers are published before the tickets they block.
- Each issue takes the draft's title and its five sections as the body, with `Blocked by` rewritten from draft numbers to the published issues.
- Each blocking edge is recorded as a native GitHub dependency.
- The mapped `ready_for_agent` label goes only on a ticket with no unsatisfied blocker; the others get the mapped `blocked` label. Labels come from the repository's tracker configuration.
- The published issue is recorded in the draft the moment it is created, so a rerun after a partial failure skips what exists.
- Every write is read back. Once an issue matches its draft, the draft is removed, so one copy remains.

On any failure the command stops, reports what was created and what remains, and leaves the remaining drafts as they are. It writes to the forge, so it runs only when the human asked to publish; the command grants no authority.

`tickets`' publishing reference keeps settling the breakdown with the human and calls this command for the writes.

## Acceptance criteria

- `--dry-run` lists every issue, edge, and label and calls no forge write.
- With three drafts where 03 is blocked by 01 and 02, the forge receives the creates for 01 and 02 before 03.
- Each blocking edge is sent with the blocker's database id, not its number.
- A ticket with no blockers gets the mapped ready label; a blocked one gets the mapped blocked label.
- A forge failure on the second create leaves the first draft recording its issue and the others untouched; rerunning creates only the missing issues.
- A draft whose issue does not match on read-back is kept and reported.
- After a full success no draft remains and the command prints each issue's URL.
- A repository whose tracker configuration is not GitHub is refused before any write.
- Drafts that fail `harness tickets check` are refused before any write.
- `tickets`' publishing reference names the command and no longer spells out the order, the edge id, or the retry rule.

## Seam

Proposed, awaiting the human's confirmation: the tickets package's tests assert the exact forge calls and their order against a fake forge over fixture drafts; one command test in `cmd/harness` proves the wiring.

## Blocked by

13

## Out of scope

Publishing to a local-markdown tracker or any other tracker. Updating an issue that already exists and was edited on the forge. Parent issues.
