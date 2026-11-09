# Review ledger

The ledger is the local record of every finding and its disposition on one branch. `land` reads it for open findings. It lives only on this machine, one file per branch, at the path `harness review path [--branch <name>]` prints: `<git-common-dir>/harness-review/branches/<branch>/ledger.jsonl`, with a `/` in the branch name escaped as `%2F`. The branch directory also holds the review's patches. Git never tracks it, and every worktree of the repository shares it. `land` deletes the branch directory once the merge is confirmed. Unmerged review records remain until explicitly approved cleanup. A pull request carries nothing from it beyond `open-pr`'s `Reviewed:` line.

## Record

Record each disposition with `harness review record` the moment it is decided. A review with no findings, or with no review triggered, writes one line too. Whoever commits a fix appends `fixed` with the fixing commit.

Record the depth as the level the review ran at: `medium`, `high`, or `none` for a skipped review. Earlier ledgers may hold `deep`, which is still read. Earlier ledgers stored `quick` and `standard`; the CLI reads and accepts them as `medium` and `high`. Put the configured level, selected perspectives, and planned/actual role calls in `--summary`.

```sh
# A new finding; prints its id. Branch and sha default to the current branch and HEAD.
harness review record --depth high --axis bugs --reviewer claude/opus --round 1 \
  --base <base> --sha <reviewed sha> [--digest <snapshot hash>] \
  --path <file> --lines 40-52 --severity bug --class missing-guard \
  --summary "<one line>" --disposition confirmed --evidence "<the red run>"

# A whole review with no findings, or a skipped review: no --path.
harness review record --depth medium --disposition clean --summary "<what was checked>"
harness review record --depth none --disposition no-review --summary "level medium: tests-docs only"

# A later state for the same finding: only --disposition, --evidence, and --commit.
harness review record --id <id> --disposition fixed --commit <fixing sha> --evidence "<the red test, now green>"
```

Round 1 is the hunt. Round 2 marks a defect the fix recheck found in the repaired code.

The command checks the disposition, depth, round, severity, and line range, requires evidence for a closing disposition and a commit for `fixed`, allows `nit` and `pre-existing` only for findings of that severity, and lets an update change only the finding's state. It rejects a bad line without writing it. When a record is rejected or fails, the finding is not recorded: treat it as open and report the failure.

## Dispositions

- `confirmed`: proven, but the fix has not landed yet. Open.
- `fixed`: confirmed and repaired. Open again until the fixing commit is in the head.
- `dropped`: no run proved it. `evidence` says what was tried: the run stayed green, could not run, or needed a run the level does not launch. Closed, and never shown to the human one by one.
- `intentional`: the human says the behavior is wanted. `evidence` quotes their words.
- `wont-matter`: the human decides not to fix it. `evidence` quotes their words.
- `pre-existing`: the defect predates the diff.
- `nit`: reported, not acted on.

Earlier reviews also wrote `unverified` (open) and `refuted` (closed). Both are still read; new reviews write neither.

The ledger is task data that any local process can write, so a recorded dismissal is a claim, not an authority. `land` asks the human to reaccept each one.

## Read

`harness review list [--branch <name>] [--head <rev>]` prints the latest line of each finding and whole-review line on the branch as JSON; `[]` means nothing is recorded for it here.
- `open` marks a finding that is `confirmed`, an earlier `unverified`, or `fixed` by a commit the head lacks.
- `rewritten` marks a finding whose reviewed commit is no longer an ancestor of the head, as after a rebase. It still counts.
