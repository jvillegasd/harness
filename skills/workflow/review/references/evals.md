# Review evals

Measure whether the reviewer finds real defects without noise, using cases from this repository's own history. Cases and runs live in `"$(git rev-parse --path-format=absolute --git-common-dir)/harness-review/evals"` as `evals.jsonl` and `runs/<date>.jsonl`. That is inside the Git directory, so they never go into the repository, and it sits outside the `branches/` directories that `land` deletes.

Each case is one JSON line, and refers to commits rather than copying code. An optional `paths` list scopes a large introducing commit to the files around the defect; hunters then get only that part of the diff:

```json
{"id": "land-stacked-boundary", "kind": "bug", "base": "<sha>", "head": "<sha>", "expected": [{"path": "skills/land/SKILL.md", "lines": "40-52", "class": "wrong-boundary"}], "source": "<fixing sha>", "confirmed_by": "human, 2026-11-01"}
```

## Seed

1. **Bug cases.** For each `fix:` or revert commit, run `git blame` on its changed lines at its parent to find the commit that introduced them. The introducing commit's diff (`<intro>^..<intro>`) is the case. The expected finding is the fixed lines mapped back to `<intro>` with `git log -L` or `git blame --reverse`, because hunters cite lines at the case's head. For a fix that only adds lines, such as a missing guard, use the lines around the insertion. Ledger entries with the disposition `fixed`, while their branch is unmerged, are candidates too.
2. **Clean cases.** Take merged changes whose lines no later commit fixed or reverted.
3. Show each candidate to the human. A case counts only once they confirm it, and `confirmed_by` records that.

## Run

A run spends model budget, so it starts only when the human asks and names its scope: which cases, at which depth.

1. For each case, create a detached worktree at `head` and run `review`'s Hunt and Verify on `base..head` from a session started inside that worktree. Reviewers read only their session's project, so a session started elsewhere would read today's fixed code. When no session can start there, a fresh subagent whose brief confines every read to the worktree path stands in, and the run line says so. A case's worktree has a detached head, so pass `--branch eval/<case id>` to every `harness review` call; write the findings to the run file, not the ledger, and delete that branch directory after scoring.
2. Score each case:
   - A bug case is **caught** when a confirmed `bug` or `risk` falls within an expected path and line range.
   - A clean case scores one **false positive** per confirmed `bug` or `risk`.
   - Count dropped findings for both kinds. A dropped finding inside an expected range of a bug case is a **dropped catch**: a real defect the run rule cut.
3. Append a summary line: date, the harness revision of `skills/workflow/review`, depth, each hunter's resolved model, recall, false positives, dropped count, and dropped catches.
4. Compare it with the last run on the same cases. Keep a change to `review`, its references, or its `roles/` only when recall does not drop and false positives do not rise. Otherwise, report the regression to the human.
