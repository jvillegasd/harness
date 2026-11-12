# Command-line reference

This page explains command behavior that does not fit in `--help` output.

## Components

`harness install`, `harness update`, and `harness uninstall` select the same components by name. Name one or more to act on just those, or none to act on the command's default. `--client claude`, `--client codex`, or `--client opencode` (repeatable) limits a run to those clients.

| Component | What it covers | Kind |
| --- | --- | --- |
| `clients` | Claude Code, Codex, and OpenCode | Tool |
| `packages` | node, gh, and uv | Tool |
| `mcps` | Engram and agent-browser, and their registration in each client | Tool and managed config |
| `skills` | The skills, copied into each client | Managed config |
| `agents` | The role agents, rendered for each client | Managed config |
| `floor` | The floor, as each client's global instructions | Managed config |
| `settings` | Tracked client settings, compaction hooks, and command rules | Managed config |
| `aliases` | The yolo aliases and the line that loads them from your login shell's startup file | Managed config |

A tool is a program on the machine. Managed config is what the harness writes into a client and owns, recorded in `~/.harness/state.json` so it can tell its own work from your edits.

## Install and uninstall

```sh
harness install                          # every component, every client
harness install skills agents            # just those, every installed client
harness install --client claude          # every component, Claude Code only
harness uninstall                        # all managed config; tools stay
harness uninstall mcps --client codex    # Codex's MCP registrations
harness uninstall --all                  # everything, then the harness binary
```

A choice of components or clients applies to that run only. A later `harness install` with no names installs everything again, including a client you removed with `harness uninstall --client`.

`harness install` treats a tool as installed when it is on `PATH`, however it got there, and never reinstalls or upgrades it. It uses Homebrew, npm, or Anthropic's installer, and the network, only for a missing tool, and records each tool it adds. Without `--client`, it configures every client that is installed or that the run installs; naming a client that is not installed stops the run before any write. Installing `mcps` also reconciles `settings`, because Engram's setup rewrites `settings.json` and `opencode.json`, and the output says so.

`harness uninstall` with no names removes the managed config of every component and leaves every tool installed. Naming `clients`, `packages`, or `mcps`, or passing `--all`, also removes the tools of those components, but only the ones the harness recorded installing, and through the same method that installed them. A tool that was on the machine before the harness stays, and the summary names it. A tool every client shares (node, gh, uv, Engram, agent-browser) stays while any client still has harness config. No uninstall deletes a tool's data (`~/.engram`, `~/.agent-browser`, `~/.claude`, `~/.codex`) or `~/.dotfiles-backups`.

`~/.harness`, including your `harness models` and review level choices, is deleted only when nothing recorded remains: no client keeps a component and no tool is recorded. `--all` then also deletes `~/.local/bin/harness`. Machines installed before tool records existed have none, so all their tools count as present before the harness. Records written under the old component names `policy` and `mcp` are read as `floor` and `mcps`.

`harness uninstall --clear-backups` deletes the archives installs left under `~/.dotfiles-backups/harness/` and runs on its own.

## Update installed tools

`harness update` updates packages, clients, MCPs, then locked skills. Name one or more of those components to update only them. `--client` limits `clients` and the `mcps` registrations to the named clients; `packages` and `skills` serve every client. The other components carry no tool, so `harness update` refuses them and points to `harness install`.

| Component | Updates |
| --- | --- |
| `clients` | Claude Code, Codex, and OpenCode |
| `packages` | Node, `gh`, and `uv` |
| `mcps` | Engram, agent-browser, its browser runtime, and existing client registrations |
| `skills` | Locked upstream skill bodies and `skills-lock.json` |

The updater discovers installed tools before it changes anything. It updates recognized installations through their existing release source and skips tools that are not installed. If Harness cannot identify how a tool was installed, it reports that tool for a manual update. Homebrew updates name a formula or cask. Cask updates require Homebrew's `--no-quit` support. Orca-managed client copies remain under Orca's control.

The updater continues after failures. It skips actions that depend on a failed operation and exits with status 1 if any operation fails. Skipped tools and manual updates do not cause a failure. The summary reports version changes, unchanged versions, successful commands without an observable version change, skips, manual updates, and failures. Harness does not roll back completed updates.

The `clients`, `packages`, and `mcps` groups work from any directory. The `skills` group requires a Harness checkout. A bare `harness update` outside a checkout skips skills.

Both `harness update skills` and `harness skills update` refuse to run when a locked skill body or `skills-lock.json` has staged, unstaged, untracked, or ignored changes. Other local work stays untouched. Review updated skill bodies and the lockfile before running `harness install` to deploy them. The updater does not commit changes, publish them, install missing tools, or update Harness itself.

## Review commands

`harness review` sets how much review agents do on their own and lets you read what a review found. See [how review works](review.md) for the review itself.

### Set the review level

```sh
harness review level            # print the current level
harness review level high       # set it: off, medium (default), or high
harness review level --json     # machine-readable, used by agents
```

The level is stored in `~/.harness/state.json` and applies to every repository on this machine. Only you set it; agents read it but never change it. `/land` checks the level in force when it runs, not the one in force when the PR was opened.

### Read what a review found

```sh
harness review list                     # findings on the current branch, as JSON
harness review list --branch feat/x     # another branch
harness review path                     # where the ledger file lives
```

`list` shows the latest state of each finding. `[]` means nothing was recorded for that branch on this machine. Two flags matter most:

- `open: true`: the finding still needs attention. That covers confirmed findings, unverified ones from older reviews, and fixes whose commit is not in the branch yet.
- `rewritten: true`: the reviewed commit is no longer in the branch history, as after a rebase. The finding still counts.

The ledger lives inside the Git directory, so Git never tracks it and every worktree of the repository shares it. `/land` deletes it once the branch merges.

### What each disposition means

| Disposition | Meaning | Open? |
| --- | --- | --- |
| `confirmed` | Proven real; the fix has not landed yet | Yes |
| `fixed` | Repaired by a named commit | Until that commit is in the branch |
| `dropped` | No run proved it; counted in the review, not shown one by one | No |
| `intentional` | You said the behavior is wanted (your words are kept) | No |
| `wont-matter` | You decided not to fix it (your words are kept) | No |
| `pre-existing` | The problem existed before this change | No |
| `nit` | A style point, reported but not acted on | No |
| `clean` / `no-review` | A whole review that found nothing, or a review that was skipped | No |

Older reviews also recorded `unverified` (open) and `refuted` (closed). `/land` asks you to reconfirm every `refuted`, `intentional`, and `wont-matter`, because any local process can write the ledger.

### Record a finding by hand

Agents call `harness review record` as they review. You rarely need it, except to record your own decision on a finding:

```sh
harness review record --id <id> --disposition intentional --evidence "We want retries to stop after 3."
```

Run `harness review record --help` for every flag.
