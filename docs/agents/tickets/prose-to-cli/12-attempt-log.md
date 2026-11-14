# 12 Attempt log for measured loops

## What to build

`harness attempts add --log <file> --hypothesis <text> --change <text> --before <number> --after <number> --gate <pass|fail> --outcome <kept|reverted> [--note <text>]` appends one well-formed row to a hillclimb attempt log, a tab-separated file kept outside the tree. The command assigns the next attempt id, stamps the time, and computes the delta as an absolute and a percent change, so those are never typed by hand. It writes the header on first use, keeps every cell on one line, and neutralizes a cell that a spreadsheet would read as a formula.

`harness attempts list --log <file> [--json]` prints the rows, which is what the loop reads before each attempt.

A rejected value writes nothing. `hillclimb` steps 3 and 5 call these commands.

## Acceptance criteria

- The first `add` on a missing file creates it with the header and one row with id 1; the next `add` appends id 2 and leaves the first row byte for byte.
- `--before 200 --after 150` records a delta of -50 and -25%.
- `--before 0` records the absolute delta and leaves the percent empty instead of dividing by zero.
- A tab or newline inside any text value is stored as a space, and the row count grows by exactly one.
- A text value starting with `=`, `+`, `-`, or `@` is stored so a spreadsheet shows it as text.
- An unknown `--gate` or `--outcome`, or a non-numeric `--before` or `--after`, exits 2 and leaves the file unchanged.
- A log path inside the current checkout is refused.
- `list --json` returns every row with typed numbers.
- `hillclimb` names the commands and no longer lists the columns to fill by hand.

## Seam

Proposed, awaiting the human's confirmation: a small package's tests over temporary files settle the row rules; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Running the measurement or the gate. The task document's Decisions table.
