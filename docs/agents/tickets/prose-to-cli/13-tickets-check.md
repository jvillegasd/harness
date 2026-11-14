# 13 Check ticket drafts

## What to build

`harness tickets check [<feature-slug>]` validates the ticket drafts of one feature, or of every feature when none is named, and prints each problem as `path:line: message`. It exits 0 when there is none and 1 otherwise.

A draft is a file named `<NN>-<slug>.md` in the feature's draft directory with a title and five sections: What to build, Acceptance criteria, Seam, Blocked by, and Out of scope. The check requires each section to be present and non-empty, each number to be unique within the feature, `Blocked by` to hold `none` or numbers of tickets that exist in the same feature, and the blocking edges to contain no cycle.

`tickets` pins this draft format and runs the check as the done condition of its writing step. `build` step 1 runs it in place of looking for a cycle by hand.

## Acceptance criteria

- The drafts under `docs/agents/tickets/prose-to-cli/` pass.
- A draft missing a section, or with an empty one, is reported with its path and the section's name.
- Two drafts sharing a number are both reported.
- `Blocked by` naming a ticket that does not exist is reported with the missing number.
- A cycle across three tickets is reported once, naming the tickets in it.
- A file in the directory that does not match `<NN>-<slug>.md` is reported as not a draft.
- With no draft directory at all, the command prints nothing and exits 0.
- `tickets` states the format and names the command; `build` names it.

## Seam

Proposed, awaiting the human's confirmation: a small package's tests over fixture draft directories settle the rules; one command test in `cmd/harness` proves the wiring, and one test runs the check over this repository's own drafts.

## Blocked by

none

## Out of scope

Judging whether a ticket is a good tracer bullet or its acceptance criteria are red on the base. Published tickets in any tracker.
