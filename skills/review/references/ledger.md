# Review ledger

The ledger is the local record of every finding and its disposition. `land` reads it for open findings, and `retro` and the evals read it to measure the reviewer. It lives only on this machine, at the path `harness review path` prints: `$XDG_DATA_HOME/harness/reviews/<host>_<owner>_<repo>/ledger.jsonl`. It sits outside `~/.harness` because `harness install --uninstall` deletes that directory. A pull request carries nothing from it beyond `open-pr`'s `Reviewed:` line.

## Record

```sh
# A new finding; prints its id. Branch and sha default to the current branch and HEAD.
harness review record --depth standard --axis bugs --reviewer claude/opus --round 1 \
  --base <base> --sha <reviewed sha> [--digest <snapshot hash>] \
  --path <file> --lines 40-52 --severity bug --class missing-guard \
  --summary "<one line>" --disposition unverified --evidence "<verifier output>"

# A whole review with no findings, or depth none: no --path.
harness review record --depth quick --disposition clean --summary "<what was checked>"

# A later disposition for the same finding.
harness review record --id <id> --disposition fixed --evidence "<fixing commit and test>"
```

The command validates every field and rejects a bad line without writing it. When a record is rejected or fails, the finding is not recorded: treat it as open and report the failure.

## Dispositions

- `confirmed`: proven, but the fix has not landed yet. Open.
- `unverified`: neither confirmed nor refuted. Open.
- `fixed`: confirmed and repaired.
- `refuted`: the verifier reported `green` or `could-not-run`, and the refute brief cited where the failure path is cut.
- `intentional`: the human says the behavior is wanted. `evidence` quotes their words.
- `wont-matter`: the human decides not to fix it. `evidence` quotes their words.
- `pre-existing`: the defect predates the diff.
- `nit`: reported, not acted on.

The ledger is task data that any local process can write, so a recorded dismissal is a claim, not an authority. `land` asks the human to reaccept each one.

## Read

`harness review list [--branch <name>] [--head <rev>]` prints the latest line of each finding on the branch as JSON.
- `open` marks a finding whose latest disposition is `confirmed` or `unverified`.
- `rewritten` marks a finding whose reviewed commit is no longer an ancestor of the head, as after a rebase. It still counts.
