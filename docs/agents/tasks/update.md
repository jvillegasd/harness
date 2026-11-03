# Add harness update

Branch: feat/update. Worktree: /Users/linkrs/Desktop/projects/harness. Source: user's supplied implementation plan. Authority covers local implementation, tests, commits, and independent review; no live upgrades, provisioning, push, or PR.

Implement `harness update [clients|ides|packages|mcps|skills]`. Inventory before mutation, preserve ownership and release channels, skip missing or unsupported installs, continue independent failures, and report observed outcomes. Bare order: packages, clients, mcps, skills, ides. Skills use the scratch updater with dirty locked-path protection shared with `skills update`. Machine groups work outside a checkout. MCP reconciliation must not install unrelated assets or restart clients.

Acceptance: fake executable and temporary-home tests at the coordinator/CLI and existing skills boundaries; all six repository checks; independent review. Live behavior remains unverified without separate authorization.

## Decisions

| Decision | Why | Evidence |
| --- | --- | --- |
| Unknown and prerelease npm channels use manual updates | An installed version does not identify its original dist-tag; never silently migrate preview to stable | User's channel-preservation requirement |
| MCP updates register servers directly without `engram setup` | Setup also installs hooks and skills outside the requested MCP scope | internal/install/clients.go |
| Require Homebrew `--no-quit` for cask upgrades | Homebrew normally quits and reopens cask apps during upgrades; older versions receive manual fallback | Homebrew Manpage upgrade options |

## Status

Implementation complete. Focused tests observed red then green for CLI validation, dirty locked paths, ownership, MCP prerequisites, and the review repairs. No machine upgrades or installation performed.

Independent review at medium/Quick of a6db275 found missing prerequisite guards for skills after Node failure and MCP registrations after client failure. An external scratch repro and `TestUpdateSkipsFailedNodeAndClientDependents` confirmed both. `TestUpgradePreservesHomebrewRefreshPolicy` also reproduced forced suppression of metadata refresh. Repairs add the guards and preserve Homebrew's caller refresh policy. Findings are retained in the branch review ledger. Repair validation by a second reviewer does not run at medium, so the repair delta remains independently unverified.

All six required checks passed on c3ae47fd497ad72f639e35e61ba84a102e942146: `go vet ./...`, `test -z "$(gofmt -l .)"`, `go test ./...`, `go build ./...`, `git diff --check`, and `git diff --cached --check`. The full CLI package passed in 160.954 seconds; this is check evidence, not a performance claim. Live package upgrades, MCP acceptance, and IDE trials remain unverified and require a separate execution request. The initial full suite passed on an earlier snapshot and is stale for the repair.

Complete locally. No push, PR, live update, or provisioning performed. The final documentation-only commit records this evidence; the implementation snapshot is c3ae47f.

## Review map

Flow: validate invocation, discover ownership and prerequisites, run selected groups sequentially, skip failed dependents, print observed outcomes.

Tiers after raising the deterministic floor: critical 5, behavioral 1, tests-docs 5, structural 0, mechanical 0.

Read line by line:

- `internal/update/discovery.go`: package-manager ownership, resolved executable paths, channel preservation, and cask `--no-quit` gating. Fake executable tests ran against real coordinator code.
- `internal/update/update.go`: validation, execution order, dependency skips, summaries, and scoped MCP refresh. Failure regression tests ran red before the repair and green afterward.
- `internal/install/mcp.go` and `internal/install/clients.go`: shared registration helper, no broad setup or asset installation. MCP-only tests and existing installer tests passed.
- `internal/skills/skills.go`: refusal before replacement of locked paths, including ignored files, and random staging directories that avoid deleting a pre-existing staging path. Dirty-path and unrelated-index preservation tests passed.

Behavioral: `cmd/harness/main.go` adds dispatch and help, covered by the no-subprocess/no-write invocation matrix.

Skim: `internal/update/update_test.go`, `cmd/harness/skills_test.go`, `cmd/harness/cli_test.go`, `README.md`, and this task document. No mechanical changes.

Delivery is local and reversible. Running the new command later upgrades installed software and replaces reviewed skill bodies; completed package upgrades are not rolled back. The repair delta was regression-tested but has not received independent re-review under the configured medium level.

The decisions table records the implementation choices. No dependency or provisioning change was introduced. A standing critical classification for `internal/update/**` was offered to the human but has not been added without a response.
