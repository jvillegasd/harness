# Harness AI

Shared policy, skills, and tools for Claude Code, Codex, and OpenCode.

Ask the agent to do the work. It inspects the code, makes changes on a task branch, runs checks, and reports the result. Skills help with specific jobs; they are not a required sequence.

## Install

Requires macOS or Linux, zsh, Homebrew, and Go 1.26 or newer.

```sh
go run ./cmd/harness install
```

The installer installs missing tools, copies the policy, skills, and role agents into the clients, configures Engram and agent-browser, and adds shell aliases. Sign in to each client yourself; installation does not authenticate accounts.

To apply checkout changes without installing packages:

```sh
go run ./cmd/harness install --skip-packages
```

This still changes live client settings and MCP registrations. Engram, agent-browser, and Claude Code must already be installed. Editing this repository alone does not update installed clients.

The installed binary is `~/.local/bin/harness`. It carries the assets it was built from. Managed files are recorded in `~/.harness/state.json`; collisions are archived under `~/.dotfiles-backups/harness/`. Private entries are left alone. Tracked settings are reapplied on installation, so some in-app preferences may be overwritten.

```sh
harness install --uninstall
```

Uninstall asks first, preserves edited copies, and leaves installed packages and user data. Use `--help` for client and component filters. `--no-backup` deletes installation collisions; `--clear-backups` deletes archives. Both are destructive.

See [installation and your files](docs/installation.md) for how installation and removal handle existing copies.

## Everyday use

| Need | Ask for |
| --- | --- |
| Implement a feature, fix, or one ticket with TDD | `/implement <task or ticket>` |
| Fix something broken or slow | Diagnosis and a fix |
| Settle an unclear idea | `/grill-with-docs` |
| Write a specification | `/spec` |
| Build test-first | `tdd` |
| Keep frontend or backend journey tests | `journey-verification` |
| Review changes | `review` |
| Publish the work | “Open a PR” or “ship this” |
| Clean up after you merge a PR | `/land` (or `/land merge` to have it merge) |

Local work stays local until you request publication. You merge PRs yourself, after reading the readiness report the PR ends with; an agent merges only when you run `/land merge`. An explicit request naming work to discard authorizes that scope; ambiguity or additional work requires clarification.

```mermaid
stateDiagram-v2
    direction TB
    state "Local changes" as Local
    state "Ready locally" as LocalReady
    state "Preparing PR" as Preparing
    state "PR open" as PR
    state "Ready to merge" as MergeReady
    state "Needs your input" as Waiting

    [*] --> Local: Plain request or /implement
    Local --> Local: Agent fixes failed checks
    Local --> LocalReady: Agent completes work and local checks
    LocalReady --> Preparing: You request publication
    Preparing --> PR: Agent satisfies local checks and applicable review, then publishes
    PR --> PR: Agent fixes failed checks or addresses feedback
    PR --> MergeReady: Required checks and review satisfied
    MergeReady --> PR: New changes require checks or review
    MergeReady --> Merged: You merge, or invoke /land merge and the agent rechecks the gates and merges
    Merged --> [*]: You invoke /land and the agent cleans up

    Local --> Waiting: Agent needs a decision or required check is unavailable
    Preparing --> Waiting: Agent needs a decision or required check or review is unavailable
    PR --> Waiting: Agent needs a decision or required check or review is unavailable
    Waiting --> Local: You resolve blocker and agent resumes local work
    Waiting --> Preparing: You resolve blocker and agent resumes PR preparation
    Waiting --> PR: You resolve blocker and agent resumes PR work
```

**Ready locally is a complete outcome.** Stop there if you do not want a PR. A publication request can come with the original task or later. From **Needs your input**, work resumes at the stage that was blocked. See [the workflow guide](docs/workflow.md#local-work-and-delivery) for details.

Review defaults to `medium`. Set it yourself with `harness review level off|medium|high`. Medium sends PRs beyond docs or tests, and local changes with a concrete risk, to one independent reviewer; High adds one or two specialists where the change needs them. Every reviewer runs on the model you set for the `reviewer` role with `harness models`. Asking for a review always runs one at your level. Only problems a run proves come to you; the rest are counted as dropped. See [how review works](docs/review.md) for what happens during a review, and [`harness review`](docs/cli.md#review-commands) for reading the results.

Read [the workflow guide](docs/workflow.md) for optional planning, builds, and tracker setup, or [the delegation guide](docs/subagents.md) for worker roles.

## Models and aliases

Use `harness models --help` to inspect presets and per-role assignments. Changing an assignment requires installation to refresh the rendered agents. Roles assigned to the current client run natively; roles assigned to another client run through T3 Code's `delegate_task`.

| Alias | Starts |
| --- | --- |
| `ccyolo` | Claude Code with permission checks bypassed |
| `cxyolo` | Codex with approvals and sandbox bypassed |
| `ocyolo` | OpenCode with edit, shell, and web access allowed |

The installer sources `aliases.sh` from `~/.zshrc`. These launchers grant broad access. Claude Code also gets a harness status line; Codex uses its own footer. Each client also gets a compaction hook that runs `harness resume`, so after a long session is compacted the agent gets back the branch, its latest commit, and the task document's next action. Codex runs it only after you trust it once in its `/hooks` screen.

## Update installed tools

`harness install` installs missing tools and reconciles Harness assets. It leaves installed package versions alone. To upgrade installed tools explicitly:

```sh
harness update            # All groups
harness update clients    # Claude Code, Codex, OpenCode
harness update packages   # Node, gh, uv
harness update mcps       # Engram, agent-browser, registrations
harness update skills     # Locked upstream skills and lockfile
```

A bare update runs packages, clients, MCPs, then skills. It discovers ownership before upgrading and inventories Homebrew once. Upgrades name individual formulae or casks and preserve the installation and release channels. Orca-managed clients stay owned by Orca. Recognized native Claude Code and OpenCode installations use vendor updaters. Stable npm Codex installations update through their owning prefix; prerelease or ambiguous installations need manual updates.

MCP updates refresh agent-browser's browser runtime and client registrations without reinstalling Harness policy, settings, skills, or roles. See the [CLI reference](docs/cli.md#update-installed-tools) for update behavior and failure handling.

## Maintain skills

Skills live in `skills/<group>/<name>/`; clients invoke them by name. Groups organize the repository, not the workflow.

```sh
harness skills add <owner/repo> <name> <group>
harness skills update
harness skills remove <name>
harness skills verify
```

Review upstream changes before committing. Locked third-party skills change through these commands, never by editing their bodies. `skills-lock.json` records their hashes. Installation does not update them.

## Develop

| Path | Purpose |
| --- | --- |
| `POLICY.md` | Authority and safety rules shared by clients |
| `cmd/harness/`, `internal/` | CLI and implementation |
| `skills/` | Skills, grouped by purpose |
| `clients/`, `roles/` | Client settings and role definitions |
| `docs/agents/` | This repository's tracker configuration |
| `bench/` | Real-session cost and friction measurement |

Run all local checks before handing off a change:

```sh
go vet ./...
test -z "$(gofmt -l .)"
go test ./...
go build ./...
git diff --check
git diff --cached --check
```

[AGENTS.md](AGENTS.md) describes the test boundaries. Tests check executable behavior, not whether an agent follows prose. Live behavioral evaluation needs an explicitly authorized model budget. [Bench](bench/README.md) compares session cost and friction; it does not prove correctness.
