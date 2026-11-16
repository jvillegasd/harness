# Review evals

Measure whether the reviewer finds real defects without noise, using cases from this repository's own history. Cases and runs live in `"$(git rev-parse --path-format=absolute --git-common-dir)/harness-review/evals"` as `evals.jsonl` and `runs/<date>.jsonl`. That is inside the Git directory, so they never go into the repository, and it sits outside the `branches/` directories that `land` deletes.

Each case is one JSON line, and refers to commits rather than copying code. An optional `paths` list scopes a large introducing commit to the files around the defect; hunters then get only that part of the diff:

```json
{"id": "land-stacked-boundary", "kind": "bug", "base": "<sha>", "head": "<sha>", "expected": [{"path": "skills/land/SKILL.md", "lines": "40-52", "class": "wrong-boundary"}], "source": "<fixing sha>", "confirmed_by": "human, 2026-11-01"}
```

## Seed

1. **Bug cases.** `harness review evals seed [<range>] [--max <n>]` prints one candidate per commit that introduced lines a `fix:` or revert commit changed, with those lines as they stood in that commit, which is where hunters cite them. A finding a verdict lists as fixed, on a pull request not yet merged, is a candidate too.
2. **Clean cases.** Take merged changes whose lines no later commit fixed or reverted.
3. Show each candidate to the human. A case counts only once they confirm it, and `confirmed_by` records that.

## Run

A run spends model budget, so it starts only when the human asks and names its scope: which cases, at which depth.

1. For each case, create a detached worktree at `head` and run `review`'s Hunt and Verify on `base..head` from a session started inside that worktree. Reviewers read only their session's project, so a session started elsewhere would read today's fixed code. When no session can start there, a fresh subagent whose brief confines every read to the worktree path stands in, and the run line says so. A case's worktree has a detached head, so pass `--branch eval/<case id>` to every `harness review` call; write each confirmed and dropped finding to the run file as one JSON line with `case`, `path`, `lines`, `severity`, and `disposition`, post no verdict, and delete that branch directory after scoring.
2. Score it with `harness review evals score <run file> --depth <depth> --model <role>=<model>...`, which counts recall, false positives, dropped findings, and dropped catches, appends the summary line, and compares it with the last run on the same cases; `harness review evals --help` gives the rules.
3. Keep a change to `review`, its references, or its `roles/` only when it reports `regression` false. Otherwise, report the regression to the human.
