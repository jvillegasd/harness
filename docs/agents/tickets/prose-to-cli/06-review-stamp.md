# 06 Print the pull request's review line

## What to build

`harness review stamp [--branch <key>] [--head <rev>]` prints the single review line a pull request body carries, taken from the ledger, so the line `harness land check` parses is never composed by hand.

- When a review covers the head and no finding is open, it prints `Reviewed: <base>..<sha>`, where `<sha>` is the commit that holds exactly the reviewed content.
- When the latest whole-review record says no review ran, it prints `Review: not run (level <level>)`.
- Otherwise it prints nothing on stdout, exits non-zero, and names the reason: a finding still open, a part left unreviewed, commits the review never read, or no review recorded.

`open-pr` step 4 and the pull request body template call this command for the line.

## Acceptance criteria

- After a `clean` review of the head, the command prints `Reviewed: <base>..<head>`.
- After a review whose findings were all fixed by ledger `fixed` commits now in the head, it prints the line with the repaired head.
- With a finding still `confirmed`, it prints nothing, exits non-zero, and names the finding.
- With a commit after the reviewed one that the ledger does not name as a fix, it prints nothing and names that commit.
- With a `no-review` record at level `medium`, it prints `Review: not run (level medium)`.
- A pull request body carrying the printed `Reviewed:` line reads as review source `body` with status `current` in `harness land check`.
- `open-pr` and its body template name the command and no longer describe how to choose `<sha>`.

## Seam

Proposed, awaiting the human's confirmation: the review ledger package's tests settle which line is printed; one landing-package test proves the printed line round-trips through the parser `land check` uses; one command test in `cmd/harness` proves the wiring.

## Blocked by

04

## Out of scope

Editing the pull request body. Any other review content in the body, which stays in the local ledger.
