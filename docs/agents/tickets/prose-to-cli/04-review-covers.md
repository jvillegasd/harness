# 04 One answer to "is this patch already reviewed"

## What to build

`harness review covers --base <base> --sha <sha> --level <medium|high> [--branch <key>] [--digest <hash>]` says whether a recorded review already covers a patch, so `review` can stop before launching anyone. It prints JSON: `covered`, and when true the covering review's depth, its reviewed commit, and each finding still open.

A recorded review covers the patch when its base is the given base, its depth is the given level or deeper, it left nothing unreviewed, and its reviewed commit is the given commit or every commit after it is one the ledger names as a `fixed` commit. A review recorded with a digest covers only uncommitted work with that same digest.

`review`'s Snapshot step 2 calls this command and keeps only what to do with the answer.

## Acceptance criteria

- A `clean` medium review of `<base>..<sha>` covers a medium request for the same range and does not cover a high one.
- A review of an earlier commit covers the head when every later commit is a ledger `fixed` commit, and does not when one later commit is not.
- A review recorded against a different base does not cover.
- A digest review covers only a request carrying the same digest.
- A review that recorded an unreviewed part does not cover.
- A covered answer lists the findings still open at the given commit.
- The command writes nothing.
- `review`'s Snapshot step 2 names the command and no longer restates the rule.

## Seam

Proposed, awaiting the human's confirmation: the review ledger package's tests over ledger files and throwaway repositories settle the rule; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Deciding the review level or which hunters run. Changing how `harness land check` judges a review's status, beyond the open question below.

Open questions for the human, each with a default the worker takes when unanswered:

- **Rebase.** Today `review` compares commits and `land check` compares patches, so after a rebase that changes no content `review` would hunt again while `land check` calls the earlier review current. Default: compare patches the way `land check` does, so both agree.
- **Unreviewed parts.** The ledger has no field that says a slice stayed unreviewed; it lives in free-text summaries. Default: add one optional field to a whole-review record and treat its presence as "does not cover". This reverses part of the earlier trimming of ledger fields, so it needs your word.
