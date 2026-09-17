# Repository guidance

This file tells an agent how to navigate, prepare, and verify this repository. It carries repository facts and conventions only.

The harness policy in `POLICY.md` is linked into each client's global instruction path and governs every repository: workflow routing, the oracle ladder, planning, the escalation contract, and safety. Read that policy for process. This file never restates it, and where the two meet it is more specific: this repository's own commands, paths, and conventions win.

A nested `AGENTS.md` in a subtree, when one is added, scopes to that subtree and overrides this file there. Keep repository-wide rules here, not in a subtree, and do not widen a subtree's rules to the repository.

## What this repository is

The Harness AI installation that serves Claude Code and OpenCode, extracted from the author's dotfiles into a repository of its own. `README.md` is the user-facing overview of the workflow, the skill supply chain, and validation, and `POLICY.md` is the vendor-neutral global policy linked into each client. The product assets live at the repository root; there is no nested product directory.

## Directories

| Path | Holds |
| --- | --- |
| `go.mod`, `cmd/harness/` | The Go module and the `harness` command, the single entry point for operating the harness |
| `internal/state/` | The per-branch oracle ladder and rabbit-hole counters behind `harness state` |
| `internal/checkout/` | Checkout discovery and canonical asset locations, shared by the commands that resolve a checkout |
| `internal/install/` | The installer and bootstrap behind `harness install` and `harness bootstrap` |
| `internal/locks/` | The third-party skill locks behind `harness locks` |
| `internal/bench/` | The session-population comparison behind `harness bench` |
| `internal/agents/` | The role-agent renderer behind `harness agents` |
| `internal/scenarios/` | The artifact-based scenario runner and probes behind `harness scenarios` |
| `internal/dash/` | The live terminal dashboard behind `harness dash` |
| `POLICY.md` | The vendor-neutral global policy linked into each client |
| `commands/` | The workflow commands, invoked as `/name` in Claude Code and OpenCode |
| `skills/` | Protocol, vocabulary, and locked third-party bodies |
| `clients/` | Client settings and the role assignments the installer reconciles |
| `roles/` | Canonical roles no skill owns |
| `aliases.sh` | The managed yolo-mode shell aliases |
| `bench/` | Cohort config and the manual for `harness bench` |
| `scenarios/` | The scenario matrix, fixtures, and the manual for `harness scenarios` |
| `docs/agents/` | This repository's tracker and lifecycle configuration |
| `.github/workflows/`, `.github/scripts/` | The agent-lifecycle workflow and reconciler |
| `skills-lock.json` | Reviewed third-party skill locks |
| `.agents/skills`, `.claude/skills` | Harness skills and the Claude bookkeeping links |

## Prerequisites

macOS or Linux with zsh, Homebrew, and Go 1.26 or newer. The GitHub tracker operations need the `gh` CLI authenticated with issue write permission. Branch worktrees use Worktrunk (`wt`) 0.61 or newer, the first release with the `--config-set` flag `harness state` passes; Orca is optional.

## Task runners

None. There is no build or task-runner manifest at the root (`Makefile`, `justfile`, `package.json`, `pyproject.toml`), so run the commands below directly rather than through a wrapper.

## Deterministic local checks

These are safe and write no tracked files, and must all pass before a change is handed off:

```sh
go vet ./...
test -z "$(gofmt -l .)"
go test ./...
go build ./...
go run ./cmd/harness locks check
git diff --check
```

The first four are the Go rungs: vet, formatting, the command tests that build `harness` and run it against throwaway repositories and home directories, and a compile of every package. The installer tests compare every mode against the goldens in `cmd/harness/testdata/install/`, which list each managed link, so adding or removing a skill or command means rerecording them with `go test ./cmd/harness -run TestInstallerGolden -update` and reading the diff. The Go suite also carries the reconciler regression suite, including the byte-identical check between the harness copy and the deployed copy and a `go run` smoke run with no Go module. `go run ./cmd/harness locks check` syntax-checks the remaining repo shell, `aliases.sh`, and runs diff-hygiene checks over tracked files, built from this checkout rather than whatever the last install placed on PATH. `git diff --check` catches whitespace errors and conflict markers. The lifecycle workflow in `.github/workflows/agent-lifecycle.yml` installs Go and runs the reconciler remotely; the unit suite is its local equivalent.

## Behavioral acceptance

Some changes cannot be settled by the checks above, because they act on machine state or on a running client: installer symlink reconciliation, the tracked client settings, interactive authentication, and the session metrics a harness change is meant to move. Accept those by running the affected tool and observing it, never by static inspection alone.

`harness bootstrap` and `harness install` are provisioning surfaces. Run them when the intent is to change machine state, never merely to validate a change. `--no-backup` deletes collisions instead of archiving them and is unrecoverable.

The repository root is the product root and the nested product directory is gone, so a machine installed under the old layout is reinstalled cleanly rather than upgraded in place: uninstall it with the old checkout's own `harness install --uninstall` while that checkout still exists, then install from this one. The new installer claims a link only when it resolves into a managed asset namespace and ships no old-layout detection, migration, or compatibility sweep, so an old link is replaced or refused rather than swept. The Clean reinstall section of `README.md` names the leftovers.

`go run ./cmd/harness install --skip-packages` is the same provisioning surface with the package step and Worktrunk's shell integration removed. The binary embeds the policy, commands, skills, roles, client settings, and aliases, so `go run` installs the working tree: it unpacks those assets into `~/.local/share/harness`, links every managed path there, and copies itself to `~/.local/bin/harness`. An edit under those paths reaches the clients on the next install, not live. It still rewrites the Engram and agent-browser MCP registrations and the tracked client settings, so `engram`, `agent-browser`, and `claude` must already be installed. Run it when links need reconciling, not as a validation step.

Session-population measurement for a harness change is `harness bench --split <commit-date> --since <window-start>`, documented in `bench/README.md`.

## Conventions

Markdown prose is never hard-wrapped. `.editorconfig` owns indentation: two spaces for Markdown, JSON, and YAML.

Locked skill bodies under `skills/` are not edited in place; use `harness locks` and commit only the reviewed change. This repository's tracker is GitHub, and its lifecycle configuration lives in `docs/agents/harness.json`.
