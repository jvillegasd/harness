# Review evals

Measure whether the reviewer finds real defects without noise, using cases from this repository's own history. Cases and runs live beside the ledger (see `ledger.md`) as `$dir/evals.jsonl` and `$dir/runs/<date>.jsonl`. They never go into the repository.

Each case is one JSON line, and refers to commits rather than copying code:

```json
{"id": "land-stacked-boundary", "kind": "bug", "base": "<sha>", "head": "<sha>", "expected": [{"path": "skills/land/SKILL.md", "lines": "40-52", "class": "wrong-boundary"}], "source": "<fixing sha>", "confirmed_by": "human, 2026-11-01"}
```

## Seed

1. **Bug cases.** For each `fix:` or revert commit, run `git blame` on its changed lines at its parent to find the commit that introduced them. The introducing commit's diff (`<intro>^..<intro>`) is the case. The expected finding is the fixed lines mapped back to `<intro>` with `git log -L` or `git blame --reverse`, because hunters cite lines at the case's head. For a fix that only adds lines, such as a missing guard, use the lines around the insertion. Ledger entries with the disposition `fixed` are candidates too.
2. **Clean cases.** Take merged changes whose lines no later commit fixed or reverted.
3. Show each candidate to the human. A case counts only once they confirm it, and `confirmed_by` records that.

## Run

A run spends model budget, so it starts only when the human asks and names its scope: which cases, at which depth.

1. For each case, create a detached worktree at `head` and run `review`'s Hunt and Verify on `base..head` from a session started inside that worktree. Reviewers read only their session's project, so a session started elsewhere would read today's fixed code. Write the findings to the run file, not the ledger.
2. Score each case:
   - A bug case is **caught** when a confirmed `bug` or `risk` falls within an expected path and line range.
   - A clean case scores one **false positive** per confirmed `bug` or `risk`.
   - Count unverified findings for both kinds.
3. Append a summary line: date, the harness revision of `skills/review`, depth, reviewer model, recall, false positives, and unverified count.
4. Compare it with the last run on the same cases. Keep a change to `review`, its references, or `reviewer.md` only when recall does not drop and false positives do not rise. Otherwise, report the regression to the human.
