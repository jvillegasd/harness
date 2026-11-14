# 06 Print the pull request's review line

## What to build

`harness review stamp [--branch <key>] [--head <rev>]` prints the single review line a pull request body carries, taken from the ledger, so the line `harness land check` parses is never composed by hand.

- When a `clean` record covers the head and no finding is open, it prints `Reviewed: <base>..<sha>` with that record's range.
- When the latest whole-review record says no review ran, it prints `Review: not run (level <level>)`.
- Otherwise it prints nothing on stdout, exits non-zero, and names the reason: a finding still open, no `clean` record (a review that left a part unreviewed writes none), a change the review never read, or no review recorded.

`open-pr` step 4 and the pull request body template call this command for the line.

## Acceptance criteria

- After a `clean` record at the head, the command prints `Reviewed: <base>..<head>`.
- After a rebase that changes no file's patch, it prints the line with the recorded range, and `harness land check` reads that line as `current`.
- With a finding still `confirmed`, it prints nothing, exits non-zero, and names the finding.
- With findings recorded and no `clean` record, it prints nothing and says the review is not complete.
- With a later commit that changes a file's patch, it prints nothing and names the files that changed.
- With a `no-review` record at level `medium`, it prints `Review: not run (level medium)`.
- A pull request body carrying the printed `Reviewed:` line reads as review source `body` with status `current` in `harness land check`.
- `open-pr` and its body template name the command and no longer describe how to choose `<sha>`.

## Seam

The review ledger package's tests settle which line is printed; one landing-package test proves the printed line round-trips through the parser `land check` uses; one command test in `cmd/harness` proves the wiring.

## Blocked by

04

## Out of scope

Editing the pull request body. Any other review content in the body, which stays in the local ledger.
