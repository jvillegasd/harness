# Skill prose to CLI steps, second half

- Branch: `feat/prose-cli-second-half` (integration branch, base `main` at db43919)
- Status: in progress
- Request (`/work`, this session): "on remaining issues"
- Scope: issues #202 to #210, the open half of the published tickets #192 to #210.

## Slices

| Ticket | Title | Blocked by | Owner | Status | Commit |
| --- | --- | --- | --- | --- | --- |
| #202 | worktree audit and review prune `--forge` | none | parent | todo | |
| #203 | `harness attempts add/list` | none | executor | todo | |
| #204 | `harness tickets check` | none | parent | todo | |
| #205 | `harness tickets frontier` | #204 | parent | todo | |
| #206 | `harness tickets publish` | #204 | parent | todo | |
| #207 | `harness setup probe/tracker` | none | parent | todo | |
| #208 | `harness skills new/move` | none | executor | todo | |
| #209 | `harness eval plant/clean` | none | executor | todo | |
| #210 | `harness eval run` | #209 | executor | todo | |

Executors build Go code and tests only. The parent writes every skill-prose edit, the `AGENTS.md` rows, and the `cmd/harness/main.go` wiring during integration.

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
| Executors take #203, #208, #209, #210, new or self-contained packages | parent builds all | agent | parallel work on packages no other slice touches; the parent keeps the tickets lane, #202 (shares the review ledger `land` reads), and #207 (critical paths) | file sets in the slice table |

## State and next action

Playbook: build, step 4
Next: launch the #203 executor alone; build #204 inline meanwhile.

## Evidence
