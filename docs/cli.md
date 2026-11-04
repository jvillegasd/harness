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

## Choose a review level

Ask your agent to review code. The `harness review` commands set your review preference and manage recorded findings; they do not start a review.

```sh
harness review level
harness review level --json
harness review level high
```

The first command prints the current level. The second also reports whether it came from a saved preference or the default. The third changes your preference to `high`; replace it with `off` or `medium` to choose another level.

| Level | Automatic reviews |
| --- | --- |
| `off` | Agents start no reviews. You can still explicitly request one. |
| `medium` | The default. Agents review PRs that change more than tests or documentation, and local work that is critical or has a concrete security, data-loss, compatibility, or concurrency risk. |
| `high` | Agents also review structural and behavioral local changes. Review depth follows the change's risk, with batched verification. |

Your preference is stored in `~/.harness/state.json`. Changing it preserves other settings and needs no reinstall. Only you choose the level. At `medium` and `high`, landing a PR that changes more than tests or documentation requires review of its current patch.

## Inspect review findings

```sh
harness review list
harness review path
```

`list` prints the current branch's recorded findings as JSON. Status is evaluated against `--head`, which defaults to the current checkout's `HEAD`. An `open` finding still needs attention. A `rewritten` finding was reviewed against a commit outside that head's history and needs reassessment.

To inspect another branch, pass its name to both `--branch` and `--head`. For example, for a branch named `feat/example`:

```sh
harness review list --branch feat/example --head feat/example
harness review path --branch feat/example
```

`path` prints the ledger's location inside the Git directory. Records are local to this machine and shared by worktrees of the same repository. They are not uploaded with a PR. Landing a branch deletes its review directory.
