# Review ledger

The ledger is the local record of every finding and its disposition. `land` reads it for open findings, and `retro` and the evals read it to measure the reviewer. It lives only under `$dir` on this machine. A pull request carries nothing from it beyond `open-pr`'s `Reviewed:` line.

## Location

```sh
src=$(git remote get-url origin 2>/dev/null || git rev-parse --show-toplevel)
key=$(printf '%s' "$src" | sed -E 's#^[a-z+]+://##; s#^[^@/]*@##; s#/+$##; s#\.git$##; s#[:/]+#_#g; s#^_##')
dir="${XDG_DATA_HOME:-$HOME/.local/share}/harness/reviews/$key"
mkdir -p "$dir"   # ledger: $dir/ledger.jsonl
```

The key keeps the host, owner, and full repository path, so distinct repositories never share a ledger. It sits outside `~/.harness` because `harness install --uninstall` deletes that directory.

## Lines

Write one JSON object per line. When a finding's disposition changes, append a new line with the same `id`. The latest line for an `id` wins.

| Field | Value |
| --- | --- |
| `id` | `<short reviewed sha>-<run timestamp>-<axis>-<n>`, unique per finding across runs |
| `ts` | ISO 8601 time |
| `branch`, `base`, `sha` | The branch, the base revision, and the reviewed commit |
| `digest` | The snapshot patch hash when reviewed work stayed uncommitted, else `null` |
| `round`, `depth`, `axis` | `1` or `2`; `none`, `quick`, `standard`, or `deep`; the hunter's axis |
| `reviewer` | The hunter's client and model, as `harness models resolve` reports them |
| `path`, `lines`, `severity`, `class`, `summary` | From the finding line; `lines` is a string such as `"40"` or `"40-52"` |
| `disposition` | See below |
| `evidence` | The fixing commit and test patch, the verifier and refute output, or the human's words |

A review with no findings, or with depth `none`, writes one line with `path` set to `null`, the disposition `clean` or `no-review`, and a `summary` saying what was checked or which tier set the depth.

## Dispositions

- `confirmed`: proven, but the fix has not landed yet. Open.
- `unverified`: neither confirmed nor refuted. Open.
- `fixed`: confirmed and repaired.
- `refuted`: the verifier reported `green` or `could-not-run`, and the refute brief cited where the failure path is cut.
- `intentional`: the human, in the live session, says the behavior is wanted. `evidence` quotes their words.
- `wont-matter`: the human, in the live session, decides not to fix it. `evidence` quotes their words.
- `pre-existing`: the defect predates the diff.
- `nit`: reported, not acted on.

## Append and query

Run these in bash, or keep these variable names: in zsh, a variable named `path` overwrites `PATH`.

```sh
jq -nc --arg id "$id" --arg branch "$branch" --arg base "$base" --arg sha "$sha" \
  --arg digest "$digest" --argjson round "$round" \
  --arg depth "$depth" --arg axis "$axis" --arg reviewer "$reviewer" \
  --arg path "$file" --arg lines "$lines" \
  --arg severity "$severity" --arg class "$class" --arg summary "$summary" \
  --arg disposition "$disposition" --arg evidence "$evidence" \
  '{id:$id, ts:(now|todate), branch:$branch, base:$base, sha:$sha, digest:($digest | select(. != "") // null), round:$round,
    depth:$depth, axis:$axis, reviewer:$reviewer, path:($path | select(. != "") // null), lines:$lines, severity:$severity,
    class:$class, summary:$summary, disposition:$disposition, evidence:$evidence}' >> "$dir/ledger.jsonl" \
  && grep -q "\"id\":\"$id\"" "$dir/ledger.jsonl" || echo "ledger append failed for $id" >&2

# Latest line per finding on a branch, for reviewed commits that are ancestors of <head>
jq -c --arg b "$branch" 'select(.branch == $b and .path != null)' "$dir/ledger.jsonl" \
  | while read -r l; do git merge-base --is-ancestor "$(jq -r .sha <<<"$l")" "$head" 2>/dev/null && echo "$l"; done \
  | jq -s 'group_by(.id) | map(last)'
```

A finding is open when its latest disposition is `confirmed` or `unverified`. A failed append means the finding is not recorded. Treat it as open and report the failure.
