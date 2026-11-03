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

Implementation complete. Observed red CLI help, dirty locked-path, Homebrew ownership, and MCP prerequisite tests before implementation. The preflight summary initially omitted stderr diagnostics; its existing regression test caught this and the fix restores diagnostics. Focused checks, full checks, and independent review pending. No machine upgrades or installation performed.

Next: pass focused tests, commit the repair, run all repository checks, and independent bugs review at medium/Quick.
