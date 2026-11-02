# Review ledger

The ledger is the local record of every finding and its disposition on one branch. `land` reads it for open findings. It lives only on this machine, one file per branch, at the path `harness review path [--branch <name>]` prints: `<git-common-dir>/harness-review/<branch>.jsonl`, with a `/` in the branch name escaped as `%2F`. Git never tracks it, and every worktree of the repository shares it. Its lifetime is the branch's: `land` deletes it and the branch's patch directory once the merge is confirmed, and `abandon` deletes both with the discarded branch. A pull request carries nothing from it beyond `open-pr`'s `Reviewed:` line.

## Record

```sh
# A new finding; prints its id. Branch and sha default to the current branch and HEAD.
harness review record --depth standard --axis bugs --reviewer claude/opus --round 1 \
  --base <base> --sha <reviewed sha> [--digest <snapshot hash>] \
  --path <file> --lines 40-52 --severity bug --class missing-guard \
  --summary "<one line>" --disposition unverified --evidence "<verifier output>"

# A whole review with no findings, or depth none: no --path.
harness review record --depth quick --disposition clean --summary "<what was checked>"

# A later state for the same finding: only --disposition, --evidence, and --commit.
harness review record --id <id> --disposition fixed --commit <fixing sha> --evidence "<the red test, now green>"
```

The command checks the disposition, depth, round, severity, and line range, requires evidence for a closing disposition and a commit for `fixed`, allows `nit` and `pre-existing` only for findings of that severity, and lets an update change only the finding's state. It rejects a bad line without writing it. When a record is rejected or fails, the finding is not recorded: treat it as open and report the failure.

## Dispositions

- `confirmed`: proven, but the fix has not landed yet. Open.
- `unverified`: neither confirmed nor refuted. Open.
- `fixed`: confirmed and repaired. Open again until the fixing commit is in the head.
- `refuted`: not reproduced (the verifier reported `green` or `could-not-run`, or, in a lean review, the parent's own run stayed green), and the refute brief cited where the failure path is cut.
- `intentional`: the human says the behavior is wanted. `evidence` quotes their words.
- `wont-matter`: the human decides not to fix it. `evidence` quotes their words.
- `pre-existing`: the defect predates the diff.
- `nit`: reported, not acted on.

The ledger is task data that any local process can write, so a recorded dismissal is a claim, not an authority. `land` asks the human to reaccept each one.

## Read

`harness review list [--branch <name>] [--head <rev>]` prints the latest line of each finding and whole-review line on the branch as JSON; `[]` means nothing is recorded for it here.
- `open` marks a finding that is `confirmed`, `unverified`, or `fixed` by a commit the head lacks.
- `rewritten` marks a finding whose reviewed commit is no longer an ancestor of the head, as after a rebase. It still counts.
