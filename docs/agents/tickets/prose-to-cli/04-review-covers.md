# 04 One answer to "is this patch already reviewed"

## What to build

`harness review covers --base <base> --sha <sha> --level <medium|high> [--branch <key>] [--digest <hash>]` says whether a recorded review already covers a patch, so `review` can stop before launching anyone. It prints JSON: `covered`, and when true the covering review's depth, its reviewed commit, and each finding still open.

The whole-review `clean` record is the one marker of a complete review. `review` records it for every review that ends with no finding left `confirmed` and no part unreviewed, at the head that holds the reviewed content: the repaired head when fixes landed. A review that left a part unreviewed records no `clean` line, so it never covers.

A patch is covered when the branch has a `clean` record at the given level or deeper whose change is the same change, file by file, as the given one. The comparison is by patch, the one `harness land check` already makes, so a rebase that changes no content keeps its review and the two commands cannot disagree about the same head. A review recorded with a digest covers only uncommitted work with that same digest.

`review`'s Snapshot step 2 calls this command and keeps only what to do with the answer, and its Record section states the `clean` rule for every review, not only a foreign one.

## Acceptance criteria

- A `clean` medium record of `<base>..<sha>` covers a medium request for the same range and does not cover a high one.
- After a rebase onto a moved base that changes no file's patch, the earlier `clean` record still covers the new head.
- A commit after the reviewed one that changes a file's patch means the record no longer covers.
- A branch whose review recorded findings and no `clean` line is not covered, whatever the findings' dispositions.
- A digest record covers only a request carrying the same digest.
- A covered answer lists the findings still open at the given commit.
- For a pull request whose only review source is the ledger, `covered` is true exactly when `harness land check` reports the review `current`.
- The command writes nothing.
- `review`'s Snapshot step 2 names the command and no longer restates the rule, and its Record section tells every complete review to record `clean`.

## Seam

The review ledger package's tests over ledger files and throwaway repositories settle the rule, sharing the patch comparison the landing package already uses; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Deciding the review level or which hunters run. Any new ledger field. Changing what `harness land check` reports.
