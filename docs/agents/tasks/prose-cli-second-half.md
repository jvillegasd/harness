# Skill prose to CLI steps, second half

- Branch: `feat/prose-cli-second-half` (integration branch, base `main` at db43919)
- Status: ready for delivery
- Request (`/work`, this session): "on remaining issues"
- Scope: issues #202 to #210, the open half of the published tickets #192 to #210.

## Slices

| Ticket | Title | Blocked by | Owner | Status | Commit |
| --- | --- | --- | --- | --- | --- |
| #202 | worktree audit and review prune `--forge` | none | parent | done | ce85839 |
| #203 | `harness attempts add/list` | none | executor | done | e30738d, merged 5cb15f5 |
| #204 | `harness tickets check` | none | parent | done | 6b8bed4 |
| #205 | `harness tickets frontier` | #204 | parent | done | 0cc779c |
| #206 | `harness tickets publish` | #204 | parent | done | b6cb73d |
| #207 | `harness setup probe/tracker` | none | parent | done | 9933116 |
| #208 | `harness skills new/move` | none | parent | done | 49b00c2 |
| #209 | `harness eval plant/clean` | none | parent | done | fd1dea0 |
| #210 | `harness eval run` | #209 | parent | done | e30f166 |

Executors build Go code, tests, and the command wiring (`cmd/harness/main.go`, `cli_test.go`). The parent writes every skill-prose edit and the `AGENTS.md` rows during integration.

## Playbook steps (build)

1. Read the spec and tickets and seams: done
2. Integration branch and task document: done
3. Compute the frontier
4. First slice alone end to end
5. Launch frontier slices with non-overlapping file sets
6. Check each executor result against scope and Git state
7. Integrate each verified slice, retire its worktree, recompute
8. Stop rule per slice
9. verification, change-map, review on the integration branch
10. End with the branch ready; offer delivery once

Finish: change-map, review at `harness review level` (medium), open-pr only when delivery is authorized.

## Decisions

| Decision | Instead of | By | Why | Evidence |
| --- | --- | --- | --- | --- |
| Rename the thread's `fix/remaining-issues` to `feat/prose-cli-second-half` | keep the name | agent | every slice is a feature, and the branch is this thread's own and unpushed | `git branch -m` |
| An executor takes #203 alone first; the parent builds the rest inline | one executor per self-contained slice | agent | build fans out only after the first executor slice succeeds, and the parent finished #204 to #210 before #203 reported, so no second executor was needed | slice table |
| `tickets publish` removes matched drafts only when no kept draft still names them as a blocker | removing each draft as soon as it matches | agent | a kept draft names blockers by draft number, so removing its blocker's draft would fail `tickets check` on the rerun | `internal/tickets/publish.go` |
| `setup tracker` lists labels and creates only missing ones | `gh label create --force` for all four every run | agent | the ticket requires a second run to report nothing | `internal/setup/setup.go` |
| `eval run` gives each later `--model` fresh copies of the planted directories | running several models in one directory | agent | the ticket wants 8 sessions from 2 models and 2 runs per arm, and two sessions in one directory would see each other's edits | `internal/eval/run.go` |
| `eval` keeps the key, manifest, outputs, and session record under `~/.harness/eval/<run id>/` | beside the candidates in the scratch root | agent | a candidate can list its parent directory | `internal/eval/plant.go` |

## State and next action

Playbook: build, step 10 (offer delivery)
Next: the human asks for a PR (`open-pr`, body carries `Reviewed: db43919..b7f3ea3`), then `/land`. Gotcha: the installed `harness` predates these commands; use `go run ./cmd/harness` until `harness install` runs.

## Evidence

- `go vet ./...`, `test -z "$(gofmt -l .)"`, `go build ./...`, `go test ./...`, `git diff --check` at e30f166, 9b958ab, and b7f3ea3: all pass.
- Review at medium (`review-bugs` on sonnet, 1 hunter call plus 1 fix executor, cap 2): 7 raised, 4 confirmed with red tests and fixed in b7f3ea3, 3 nits reported, 0 dropped.
- `opencode` 2.3.0: a dash-led prompt prints usage; after `--` it parses (checked with a bogus model, no session started).
- Not run: any command against the real forge or a real client session; every gh call and client is a fake or a stand-in.
