# Add harness update

Branch: feat/update. Worktree: /Users/linkrs/Desktop/projects/harness. Source: user's supplied implementation plan. Authority covers local implementation, tests, commits, and independent review; no live upgrades, provisioning, push, or PR.

Implement `harness update [clients|ides|packages|mcps|skills]`. Inventory before mutation, preserve ownership and release channels, skip missing or unsupported installs, continue independent failures, and report observed outcomes. Bare order: packages, clients, mcps, skills, ides. Skills use the scratch updater with dirty locked-path protection shared with `skills update`. Machine groups work outside a checkout. MCP reconciliation must not install unrelated assets or restart clients.

Acceptance: fake executable and temporary-home tests at the coordinator/CLI and existing skills boundaries; all six repository checks; independent review. Live behavior remains unverified without separate authorization.

## Decisions

| Decision | Why | Evidence |
| --- | --- | --- |
| Unknown and prerelease npm channels use manual updates | An installed version does not identify its original dist-tag; never silently migrate preview to stable | User's channel-preservation requirement |
| MCP updates register servers directly without `engram setup` | Setup also installs hooks and skills outside the requested MCP scope | internal/install/clients.go |

## Status

Inspection complete; implementation in progress. Next: red CLI test, coordinator, scoped MCP helper, skill protection, tests, docs, checks, review.
