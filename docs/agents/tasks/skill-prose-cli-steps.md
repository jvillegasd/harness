# Skill prose to CLI steps, first half

- Branch: `feat/skill-prose-cli-steps` (integration branch, base `main` at 13c8ff9; ticket-draft commits on it net to no diff)
- Status: shipped for review
- Request (`/work`, this session): "on the first half of the tickets"
- Scope: issues #192 to #201, the first 10 of the 19 published tickets #192 to #210. #202 to #210 are the second half.

## Slices

| Ticket | Title | Blocked by | Owner | Status | Commit |
| --- | --- | --- | --- | --- | --- |
| #192 | `harness pr status` verdict | none | parent | done | 92699c0 |
| #193 | `harness pr status --watch` | #192 | parent | done | e160550 |
| #194 | `harness review target` | none | parent | done | 4572a00 |
| #195 | `harness review covers` | none | parent | done | 840bd7e |
| #196 | `harness review worktree add/retire` | none | parent | done | e05809a |
| #197 | `harness review stamp` | #195 | parent | done | 978743d |
| #198 | `harness review snapshot` | none | executor | done | f26d7c2, a679764 |
| #199 | `harness land repair` | none | parent | done | 152c07d |
| #200 | `harness pr threads reply/resolve` | #192 | parent | done | 0b08e05 |
| #201 | `harness land sync/finish` | none | parent | done | 5ff9816 |

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
| Parent builds #194 to #197, #199, #201 inline too | one executor per slice | agent | every slice edits the same `command.go` and a critical skill (`land`, `review`), so slices could not run in parallel, and #199 rewrites pushed branches | file sets in the slice table |
| Patch comparison moves from `land` to `reviewledger` (`ChangedSince`, `Names`, `PatchID`) | a copy in each package | agent | #195 requires `covers` and `land check` to compare the same way, and `land` already imports `reviewledger` | `internal/reviewledger/patch.go` |
| `worktree.txt` gains a second line, the commit | path only | agent | `retire` needs the creation commit for a lossless detached retirement; remove and prune only test existence | `internal/reviewledger/worktree.go` |
| Snapshot stderr finding dropped, code unchanged | switching the diff to stdout-only output | agent | an autocrlf/safecrlf repro left the patch clean, and an unproven finding is dropped | review ledger |
| `pr status --watch` defaults to a 1800s timeout | no bound | agent | a background wait that never ends hides a stuck check | `internal/land/pr.go` |

## State and next action

Playbook: build, step 10 (open-pr)
Next: the human reads and merges the pull request; then `/land`. Review at medium found 7: 6 fixed in 92492a4, 1 dropped (no repro); `harness review stamp` prints `Reviewed: 13c8ff9..92492a4`.
Gotchas: the installed `harness` predates these commands; use `go run ./cmd/harness` until `harness install` runs. #198 asks `verification` step 3 to drop a diff-and-hash recipe it never had.

## Evidence

- `go vet ./...`, `test -z "$(gofmt -l .)"`, `go test ./...`, `go build ./...`, `git diff --check` at 92492a4: all pass.
- Review at medium (`review-bugs`, 1 of 2 calls): 7 findings, 6 fixed in 92492a4 with red-then-green tests, 1 dropped (no repro).
- Not run: any command against the real forge; every gh call is tested against a fake.
