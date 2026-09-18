# scenarios

`harness scenarios` answers a different question than `harness bench`. The bench reports whether a harness change moved cost and friction across real sessions. It cannot see whether a workflow preserved its requirements or produced the intended artifacts. A scenario does: it declares the artifacts and assertions it expects up front, runs in a disposable repository, and retains every attempt for review. `compare` then puts two retained batches side by side, correctness before cost.

## Usage

```sh
harness scenarios validate
harness scenarios run
harness scenarios report
harness scenarios compare --before <dir> --after <dir>
```

| Command | Effect |
| --- | --- |
| `validate` | Reject any declaration that names no artifacts or assertions, before a session runs. |
| `run` | Execute each scenario and write one attempt per run under `attempts/`. |
| `report` | Print `local-tooling` and `behavioral-acceptance` as separate lines. |
| `compare` | Report behavioral pass rates first, then efficiency among passing runs only. |

`harness scenarios` runs inside a harness checkout, from any directory in it. `--matrix` and `--out` override the declaration file and the attempts directory. `run --scenario <id>` narrows to one scenario; an id that matches no declared scenario is rejected before any attempt is written, and `compare` requires both `--before` and `--after`.

## Attempts

Each attempt under `attempts/<id>/<stamp>-<digest>/` retains:

| File | Holds |
| --- | --- |
| `prompts.txt` | The declared prompt. |
| `provenance.json` | The input digest and the executor that produced the attempt. |
| `events.jsonl` | Tool and command events. |
| `artifacts.json` | One result per declared artifact or assertion. |
| `transcript.jsonl` | The recorded session or command output. |
| `metrics.json` | Normalized efficiency: tokens, tool calls, output bytes, wall time, model, terminal status. |
| `status.json` | The computed state and its reasons. |
| `forge.log` | Every `gh` call the attempt made, one per line. Written only under a publication fixture. |

## States

`evaluate` returns the first state that holds:

| State | Meaning |
| --- | --- |
| `incomplete` | A required evidence file is missing or unreadable. |
| `stale` | The scenario or a declared harness input changed since the attempt. |
| `interrupted` | The recorded run was killed by a signal rather than failing on its own. |
| `fail` | An artifact check failed, including a blocked forge call. |
| `unreviewed` | Artifacts passed but no `review.json` record exists. |
| `pass` | Artifacts passed and the transcript has a review record. |
| `unavailable` | No attempt exists for the scenario, so nothing ran in this batch. |

A transcript review is a `review.json` beside the attempt with `{"reviewed": true}`. It is the record that a human read the transcript, which artifact checks alone cannot supply.

`run` additionally reports `not-run` for a live scenario that has neither a recording nor `--allow-paid`. That is not an evaluated state: nothing ran, so no attempt is written.

## Metrics

Every attempt carries `metrics.json`, normalized from the recorded events so two runs of the same scenario and model are comparable:

| Field | Holds |
| --- | --- |
| `scenario`, `model` | What ran. `model` is `null` when the recording names none. |
| `input_tokens`, `output_tokens`, `cached_input` | Token usage, `null` when nobody measured it. |
| `tool_calls` | Tool invocations the run made. |
| `command_output_bytes` | Bytes the run's commands returned. |
| `wall_seconds` | First to last of the executor. |
| `terminal_status` | `ok`, `failed`, `interrupted`, `model-unavailable` when a live session failed without using a token because the client reported the model as not found, or `unknown` when nothing was executed. |

An absent token count is `null`, never `0`: an unmeasured field and a measured zero are different claims, and collapsing them would invent evidence.

## Compare

`compare --before <dir> --after <dir>` puts two retained batches from different harness versions side by side. It reports behavioral pass rates first, counting every state so failed, interrupted, stale, unavailable, and unreviewed attempts stay visible. It then reports efficiency metrics among passing runs only, pairing one scenario and model at a time, so unlike models or scenarios are never pooled and a failed baseline can never read as an efficiency win. A comparison resting on a single successful pair prints `smoke`: one pair is a smoke result, not a savings claim.

## Isolation

Every scenario runs in a fresh temporary repository with no git remote. A `gh` shim is placed first on `PATH`; it logs each call and exits nonzero, so a scenario cannot reach a real forge or tracker. `GH_TOKEN` and `GITHUB_TOKEN` are cleared. Any logged call fails the attempt under `no-external-forge`.

## Publication

A scenario that declares `"publication": true` gets a publication fixture instead, so an oracle can judge what a workflow actually published rather than what its transcript claims. The repository is seeded with one commit, pushed to a local bare `origin`. The `origin` remote uses a `scenario::` URL, and git is allowed only that protocol, whose helper always connects to the seeded origin whatever the URL says, so a push to any other path or host is refused. The `gh` shim becomes a substitute: it logs each call to the attempt's `forge.log`, one line per call, and succeeds, answering `pr create` with a placeholder URL. The protocol allowlist is an environment variable a session could override, so `no-git-remote` gives way to `local-origin-only`, which fails if any remote URL is not `scenario::origin` after the run. `no-external-forge` applies only when the scenario declares neither `forge_allowed` nor `forge_forbidden`, and then it ignores the `pr create` calls a declared `pull_requests` counts. A recording replays no git or forge calls, so a publication scenario with a recording is refused rather than judged vacuously.

Side-effect assertions are valid only under a publication fixture:

| Assertion | Passes when |
| --- | --- |
| `commits: <n>` | Exactly `n` commits exist after the seed, across every ref except the stash. |
| `push: true` or `false` | Some ref on the origin moved, or none did. |
| `pull_requests: <n>` | The forge log holds exactly `n` `pr create` calls. |
| `forge_allowed: [<op>, ...]` | Every logged call starts with one of the listed operations, such as `pr view`. |
| `forge_forbidden: [<op>, ...]` | No logged call starts with a listed operation. Each operation is its own result. |

Operations match on whole words, so `pr create` does not match `pr created`.

## Paid execution

A `live` scenario needs a recording under `fixtures/<id>/` or `--allow-paid`. Paid sessions are capped per batch: 32 by default, or `--cap <n>`, which requires `--allow-paid` and is otherwise an incompatible invocation rejected before any attempt is written. The first paid session retains the cap in `batch.json` under `--out`, and changing it mid-batch is refused; a new `--out` directory is a new batch and a new budget.

The count is read from the batch's own attempts, so `run` refuses a session once the recorded paid attempts reach the cap and reports the count. A paid attempt's provenance is written before its session starts, so a session killed mid-run still counts; other executors write their attempt only once they finish. Failed and interrupted sessions count; a `model-unavailable` one, which failed without using a token, does not, because it spent nothing. Attempts are never deleted to free budget: a failure stays on the record rather than being erased by a rerun. An unreadable `batch.json` is refused rather than reset.

## Fixtures

A fixture is a recorded attempt. Its `repo/` holds the files the recorded session produced, and those files are materialized into the disposable repository; its `events.jsonl` and `transcript.jsonl` become the attempt's evidence. An absent `repo/` means the session wrote nothing, which is exactly what `tree_unchanged: true` asserts for a read-only command. The shipped `ask-read-only` fixture exercises the read-only scenario offline. It is test data, not a claim about a specific session.
