# Review ledger

The ledger is a local record of every finding and its disposition. `land` reads it for open findings, and `retro` and the evals read it to measure the reviewer. It stays on this machine: it never goes into a commit, a pull request body, or a tracker.

## Location

```sh
url=$(git remote get-url origin 2>/dev/null)
key=$(printf '%s' "${url:-$(basename "$(git rev-parse --show-toplevel)")}" | sed -E 's#\.git$##; s#^.*[:/]([^/]+)/([^/]+)$#\1-\2#')
dir="${XDG_DATA_HOME:-$HOME/.local/share}/harness/reviews/$key"
mkdir -p "$dir"   # ledger: $dir/ledger.jsonl
```

It sits outside `~/.harness` because `harness install --uninstall` deletes that directory.

## Lines

Write one JSON object per line. When a finding's disposition changes, append a new line with the same `id`. The latest line for an `id` wins.

| Field | Value |
| --- | --- |
| `id` | `<short reviewed sha>-r<round>-<n>` |
| `ts` | ISO 8601 time |
| `branch`, `base`, `sha` | The branch, the base revision, and the reviewed commit |
| `digest` | The content digest when reviewed work stayed uncommitted, else `null` |
| `round`, `depth`, `axis` | `1` or `2`; `none`, `quick`, `standard`, or `deep`; the hunter's axis |
| `reviewer` | The hunter's client and model, as `harness models resolve` reports them |
| `path`, `line`, `severity`, `class`, `summary` | From the finding line |
| `disposition` | See below |
| `evidence` | The fixing commit and test path, the verifier and refute output, or the human's words |

A review with no findings, or with depth `none`, writes one line with `path` set to `null`, the disposition `clean` or `no-review`, and `summary` saying what was checked or which tier set the depth.

## Dispositions

- `confirmed`: proven, but the fix has not landed yet. Open.
- `unverified`: neither confirmed nor refuted. Open.
- `fixed`: confirmed and repaired.
- `refuted`: the verifier stayed green and the refute brief cited the cut.
- `intentional`: the human says the behavior is wanted.
- `wont-matter`: the human decides not to fix it.
- `pre-existing`: the defect predates the diff.
- `nit`: reported, not acted on.

## Append and query

Run these in bash, or name shell variables as below: in zsh, a variable named `path` overwrites `PATH`.

```sh
jq -nc --arg id "$id" --arg branch "$branch" --arg base "$base" --arg sha "$sha" \
  --arg path "$file" --argjson line "$line" --arg severity "$severity" --arg class "$class" \
  --arg summary "$summary" --arg disposition "$disposition" --arg evidence "$evidence" \
  --arg depth "$depth" --arg axis "$axis" --arg reviewer "$reviewer" --argjson round "$round" \
  '{id:$id, ts:(now|todate), branch:$branch, base:$base, sha:$sha, digest:null, round:$round,
    depth:$depth, axis:$axis, reviewer:$reviewer, path:$path, line:$line, severity:$severity,
    class:$class, summary:$summary, disposition:$disposition, evidence:$evidence}' >> "$dir/ledger.jsonl"

# Open findings on a branch
jq -s --arg b "$branch" 'map(select(.branch == $b and .path != null)) | group_by(.id) | map(last)
  | map(select(.disposition == "confirmed" or .disposition == "unverified"))' "$dir/ledger.jsonl"
```
