# Skill prose to CLI steps, first half

- Branch: `feat/skill-prose-cli-steps` (integration branch, base `main` at 13c8ff9; ticket-draft commits on it net to no diff)
- Status: in progress
- Request (`/work`, this session): "on the first half of the tickets"
- Scope: issues #192 to #201, the first 10 of the 19 published tickets #192 to #210. #202 to #210 are the second half.

## Slices

| Ticket | Title | Blocked by | Owner | Status | Commit |
| --- | --- | --- | --- | --- | --- |
| #192 | `harness pr status` verdict | none | parent | todo | |
| #193 | `harness pr status --watch` | #192 | parent | todo | |
| #194 | `harness review target` | none | executor | todo | |
| #195 | `harness review covers` | none | executor | todo | |
| #196 | `harness review worktree add/retire` | none | executor | todo | |
| #197 | `harness review stamp` | #195 | executor | todo | |
| #198 | `harness review snapshot` | none | executor | todo | |
| #199 | `harness land repair` | none | executor | todo | |
| #200 | `harness pr threads reply/resolve` | #192 | parent | todo | |
| #201 | `harness land sync/finish` | none | executor | todo | |

Executors build Go code and tests only. The parent writes every skill-prose edit (`skills/workflow/land/**` and `skills/workflow/review/**` are critical paths) and the `AGENTS.md` rows during integration.

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
| First half is #192 to #201 (10 tickets) | #192 to #200 (9) | agent | 19 tickets round up to 10, and #201 has no blocker | `gh issue list` |
| Reuse `feat/skill-prose-cli-steps` as the integration branch | a new branch from main | agent | it was created for this work and its diff against main is empty | `git diff main..HEAD --stat` |
| Parent builds the `pr` family (#192, #193, #200) inline | executors | agent | #192 reshapes the gate code `land check` shares, a cross-cutting interface | `internal/land/check.go` |

## State and next action

Playbook: build, step 3
Next: launch #198 as the first executor slice; build #192 inline meanwhile.
