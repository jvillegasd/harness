# 09 Reply to and resolve review threads

## What to build

`harness pr threads reply <pr> <thread id> --body-file <file>` posts a reply on one review thread, reading the text from a file so comment text never passes through a shell command line. `harness pr threads resolve <pr> <thread id>...` resolves the named threads. Each command reads the thread back and prints its resulting state.

Thread ids are the ones `harness pr status` lists. Both commands write to the forge, so they run only under the authority `address-review` already has for its pull request; the commands grant none.

`address-review` step 5 calls these commands and keeps the judgment about which threads are settled.

## Acceptance criteria

- `reply` posts the file's exact text, including quotes, backticks, and newlines, as a reply on the named thread.
- `resolve` marks each named thread resolved and prints each as resolved after reading it back.
- A thread id that does not belong to the pull request is refused before any write, and the other ids in the same call are not resolved either.
- Resolving a thread that is already resolved exits 0 and writes nothing.
- A missing or empty body file is refused before any write.
- After a thread is resolved, `harness pr status` no longer lists it.
- `address-review` names the commands.

## Seam

The landing package's tests assert the exact forge calls against its fake forge; one command test in `cmd/harness` proves the wiring.

## Blocked by

01

## Out of scope

Classifying a thread as fix, dismiss, or ask. Top-level pull request comments, which `review` posts on its own terms.
