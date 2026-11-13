# Command-line reference

This page covers `harness` command behavior that does not fit in `--help`. For the flags themselves, run `harness <command> --help`.

## Components

`harness install`, `harness update`, and `harness uninstall` all accept the same component names. Name one or more to act on only those. Name none to act on the command's default.

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

The two kinds:

- **A tool** is a program on your machine.
- **Managed config** is what the harness writes into a client. The harness records it in `~/.harness/state.json` so it can tell its own files from your edits.

To limit a run to some clients, add `--client claude`, `--client codex`, or `--client opencode`. You can repeat the flag.

## Install and uninstall

```sh
harness install                          # every component, every client
harness install skills agents            # only these, in every installed client
harness install --client claude          # every component, Claude Code only
harness uninstall                        # all managed config; tools stay
harness uninstall mcps --client codex    # Codex's MCP registrations
harness uninstall --all                  # everything, then the harness binary
```

A choice of components or clients applies to that run only. A later `harness install` with no names installs everything again, including a client you removed with `harness uninstall --client`.

### How install treats tools

- **A tool on your `PATH` counts as installed**, however it got there. Install never reinstalls or upgrades it. Use `harness update` for upgrades.
- **A missing tool is installed** with Homebrew, npm, or Anthropic's installer. Only then does install use the network. It records each tool it adds.
- **Without `--client`**, install configures every client that is installed or that this run installs.
- **Naming a client that is not installed** stops the run before it writes anything.
- **Installing `mcps` also reconciles `settings`**, because Engram's setup rewrites `settings.json` and `opencode.json`. The output says when this happens.

### How uninstall treats tools

- **With no names**, uninstall removes the managed config of every component and leaves every tool installed.
- **Naming `clients`, `packages`, or `mcps`, or passing `--all`**, also removes those components' tools. It removes only the tools the harness recorded installing, and uses the same method that installed them.
- **A tool you had before the harness stays**, and the summary names it.
- **A shared tool** (node, gh, uv, Engram, agent-browser) stays while any client still has harness config.
- **No uninstall deletes a tool's data** (`~/.engram`, `~/.agent-browser`, `~/.claude`, `~/.codex`) or `~/.dotfiles-backups`.

### When the harness removes itself

`~/.harness`, which holds your `harness models` choices and your review level, is deleted only when nothing recorded remains: no client keeps a component, and no tool is recorded. With `--all`, uninstall then also deletes `~/.local/bin/harness`.

Two notes for older installs:

- Machines installed before tool records existed have none. Uninstall treats all their tools as present before the harness, so it keeps them.
- Records written under the old component names `policy` and `mcp` are read as `floor` and `mcps`.

### Delete backups

`harness uninstall --clear-backups` deletes the archives that installs left under `~/.dotfiles-backups/harness/`. It runs on its own, without other arguments, and the files cannot be recovered.

## Update installed tools

`harness update` upgrades tools in this order: packages, clients, MCPs, then locked skills. Name components to update only those.

| Component | Updates |
| --- | --- |
| `packages` | Node, `gh`, and `uv` |
| `clients` | Claude Code, Codex, and OpenCode |
| `mcps` | Engram, agent-browser, its browser runtime, and existing client registrations |
| `skills` | Locked upstream skill bodies and `skills-lock.json` |

The other components carry no tool, so `harness update` refuses them and points you to `harness install`. `--client` limits `clients` and the `mcps` registrations to the named clients. `packages` and `skills` serve every client, so `--client` does not affect them.

### How the updater upgrades a tool

The updater first finds out which tools are installed and how. Then:

- It updates each tool through the source it came from, such as its Homebrew formula or cask or its vendor updater. It keeps the release channel you are on.
- It skips tools that are not installed.
- If it cannot tell how a tool was installed, it asks you to update that tool by hand.
- Copies of a client managed by Orca stay under Orca's control.
- Cask updates need Homebrew's `--no-quit` support.
- Codex installed through npm updates through the npm prefix that owns it, but only on a stable release. A prerelease or an unclear install needs a manual update.

### When an update fails

The updater keeps going after a failure. It skips any step that depends on the failed one, and at the end it exits with status 1 if anything failed. Skipped tools and manual updates do not count as failures. Completed updates are not rolled back.

The summary lists version changes, unchanged versions, commands that succeeded without a visible version change, skips, manual updates, and failures.

### Where you can run it

- `clients`, `packages`, and `mcps` work from any directory.
- `skills` needs a Harness checkout. A bare `harness update` outside a checkout skips skills.

### Updating skills safely

`harness update skills` and `harness skills update` both refuse to run if a locked skill body or `skills-lock.json` has any local changes: staged, unstaged, untracked, or ignored. Your other work stays untouched.

After an update, read the changed skill bodies and the lockfile, then run `harness install` to deploy them. The updater does not commit or publish anything. It does not install missing tools or update the harness itself.

## Review commands

`harness review` sets how much review happens on its own, and lets you read what a review found. [How review works](review.md) explains the review itself.

### Set the review level

```sh
harness review level            # print the current level
harness review level high       # set it: off, medium (default), or high
harness review level --json     # machine-readable, used by agents
```

The level is stored in `~/.harness/state.json` and applies to every repository on this machine. Only you set it. Agents read it but never change it. `/land` uses the level in force when it runs, not the one in force when the PR was opened.

### Read what a review found

```sh
harness review list                     # findings on the current branch, as JSON
harness review list --branch feat/x     # findings on another branch
harness review path                     # where the ledger file lives
harness review remove --branch feat/x   # delete a merged branch's ledger and patches
```

`list` shows the latest state of each finding. An empty list, `[]`, means nothing was recorded for that branch on this machine. Two fields matter most:

- **`open: true`:** the finding still needs attention. This covers confirmed findings and fixes whose commit is not in the branch yet.
- **`rewritten: true`:** the reviewed commit is no longer in the branch history, for example after a rebase. The finding still counts.

The findings live in a ledger file inside the Git directory. Git never tracks it, and every worktree of the repository shares it. `/land` deletes it once the branch merges, with `harness review remove`. That command leaves a directory alone while it still records a review worktree.

### What each disposition means

| Disposition | Meaning | Still open? |
| --- | --- | --- |
| `confirmed` | Proven real, and the fix has not landed yet | Yes |
| `fixed` | Repaired by a named commit | Until that commit is in the branch |
| `dropped` | No run proved it. Counted in the review, not listed one by one | No |
| `accepted` | You said the behavior is wanted, or decided not to fix it. Your words are kept | No |
| `nit` | A style point, or a problem that existed before this change. Reported but not acted on | No |
| `clean` or `no-review` | A whole review that found nothing, or a review that was skipped | No |

Any local process can write to the ledger, so `/land` asks you to reconfirm every `accepted` finding.

### Record your own decision on a finding

Agents call `harness review record` while they review. You rarely need it yourself, except to record your decision on a finding:

```sh
harness review record --id <id> --disposition accepted --evidence "We want retries to stop after 3."
```

Run `harness review record --help` for every flag.
