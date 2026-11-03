# Command-line reference

This page explains command behavior that does not fit in `--help` output.

## Update installed tools

`harness update` updates packages, clients, MCPs, then locked skills. Pass one group name to update only that group:

| Group | Updates |
| --- | --- |
| `clients` | Claude Code, Codex, and OpenCode |
| `packages` | Node, `gh`, and `uv` |
| `mcps` | Engram, agent-browser, its browser runtime, and existing client registrations |
| `skills` | Locked upstream skill bodies and `skills-lock.json` |

The updater discovers installed tools before it changes anything. It updates recognized installations through their existing release source and skips tools that are not installed. If Harness cannot identify how a tool was installed, it reports that tool for a manual update. Homebrew updates name a formula or cask. Cask updates require Homebrew's `--no-quit` support. Orca-managed client copies remain under Orca's control.

The updater continues after failures. It skips actions that depend on a failed operation and exits with status 1 if any operation fails. Skipped tools and manual updates do not cause a failure. The summary reports version changes, unchanged versions, successful commands without an observable version change, skips, manual updates, and failures. Harness does not roll back completed updates.

The `clients`, `packages`, and `mcps` groups work from any directory. The `skills` group requires a Harness checkout. A bare `harness update` outside a checkout skips skills.

Both `harness update skills` and `harness skills update` refuse to run when a locked skill body or `skills-lock.json` has staged, unstaged, untracked, or ignored changes. Other local work stays untouched. Review updated skill bodies and the lockfile before running `harness install` to deploy them. The updater does not commit changes, publish them, install missing tools, or update Harness itself.
