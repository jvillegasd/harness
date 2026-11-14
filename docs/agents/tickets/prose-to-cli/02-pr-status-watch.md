# 02 Watch a pull request until it settles

## What to build

`harness pr status --watch` repeats the status pass until the verdict is terminal: ready, or any blocker other than pending required checks. It then prints that verdict and exits with its code. `--interval <seconds>` sets the pause between passes (default 60) and `--timeout <seconds>` bounds the wait; a timeout prints the last verdict and exits 8.

It replaces the two-step wait in `open-pr` and `address-review`, which today run `gh pr checks <n> --required --watch` in the background and then read mergeability separately. Both skills run this command in the background instead.

## Acceptance criteria

- With a forge that reports pending checks twice and then passing ones, the command makes three passes, prints `ready: true` once, and exits 0.
- A check that turns red while watching ends the watch at that pass with exit 5.
- An unresolved thread or a conflict present on the first pass ends the watch at once, without sleeping.
- `--timeout` shorter than the pending period exits 8 and prints the last verdict.
- A forge read that fails is retried on the next pass; three consecutive failures exit 1 naming the failure.
- `open-pr` step 5 and `address-review` step 4 name the command and no longer name `gh pr checks --watch`.

## Seam

Proposed, awaiting the human's confirmation: the landing package's tests drive the loop with a scripted fake forge and an injected clock, so no test sleeps; one command test in `cmd/harness` proves the flags are wired.

## Blocked by

01

## Out of scope

Watching a merge queue or a stack as it lands. Retriggering or classifying a failed check, which stays `address-review`'s judgment.
