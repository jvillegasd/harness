# Repository guidance

This file tells an agent how to navigate, prepare, and verify this repository. It carries repository facts and conventions only.

The harness policy in `POLICY.md` is linked into each client's global instruction path and governs every repository: composable capabilities, scoped authority, verification, recovery, and safety. Read that policy for process. This file never restates it, and where the two meet it is more specific: this repository's own commands, paths, and conventions win.

A nested `AGENTS.md` in a subtree, when one is added, scopes to that subtree and overrides this file there. Keep repository-wide rules here, not in a subtree, and do not widen a subtree's rules to the repository.

## What this repository is

The Harness AI installation that serves Claude Code, Codex, and OpenCode. `README.md` is the user-facing overview of the workflow, the skill supply chain, and validation, and `POLICY.md` is the vendor-neutral global policy linked into each client. The product assets live at the repository root; there is no nested product directory.

## Directories

| Path | Holds |
| --- | --- |
| `go.mod`, `cmd/harness/` | The Go module and the `harness` command, the single entry point for operating the harness |
| `internal/checkout/` | Checkout discovery and canonical asset locations, shared by the commands that resolve a checkout; `SkillDirs` is the one reader of the grouped skill tree |
| `internal/install/` | The installer behind `harness install` |
| `internal/skills/` | The third-party skill locks behind `harness skills`, and the invocation-guard check on local skills |
| `internal/bench/` | The session-population comparison behind `harness bench` |
| `internal/changemap/` | The deterministic floor tiers behind `harness change-map`, the first pass of the `change-map` skill |
| `internal/agents/` | The role-agent renderer the installer calls in-process |
| `internal/models/` | The per-machine role model presets behind `harness models` |
| `internal/statefile/` | Shared read and atomic write of `~/.harness/state.json`, whose top-level keys the installer, `harness models`, and `harness review level` each own |
| `internal/reviewledger/` | The per-branch local review ledger behind `harness review`, which records findings and their dispositions inside the Git directory, and the review level behind `harness review level` |
| `POLICY.md` | The vendor-neutral global policy linked into each client |
| `skills/<group>/<name>/` | Independent capabilities (also invoked as `/name`), grouped by type: `workflow` (what a normal session invokes), `support` (the shared helpers those skills and the policy call), `planning`, `design`, `assurance`, and `ops`. Locked bodies sit in their group beside local ones; clients see only the bare name |
| `skills/assurance/e2e-verification/SKILL.md`, `skills/assurance/e2e-verification/scripts/` | Release E2E QA: an isolated install, a Playwright suite with real screenshots per test, and the run kit and report builder it copies |
| `clients/` | Client settings and the role assignments the installer reconciles |
| `roles/` | Canonical roles no skill owns, the executor and code-search |
| `aliases.sh` | The managed yolo-mode shell aliases |
| `bench/` | Cohort config and the manual for `harness bench` |
| `docs/agents/` | This repository's tracker and lifecycle configuration |
| `.github/workflows/`, `.github/scripts/` | The agent-lifecycle workflow and reconciler |
| `skills-lock.json` | Reviewed third-party skill locks |
| `.agents/skills`, `.claude/skills` | One link per skill by bare name: `.agents/skills/<name>` points at the grouped body, `.claude/skills/<name>` at that link. Add a skill with `harness skills add` or link it by hand; `TestShippedSkillLinksResolve` fails on a missing or stale link |

## Prerequisites

macOS or Linux with zsh, Homebrew, and Go 1.26 or newer. The GitHub tracker operations need the `gh` CLI authenticated with issue write permission. A delegated role assigned to the session's own client runs as a native subagent; Orca, through the `orchestration` skill that Orca itself installs and updates, runs only a role assigned to another client. Inline work needs neither.

## Task runners

None. There is no build or task-runner manifest at the root (`Makefile`, `justfile`, `package.json`, `pyproject.toml`), so run the commands below directly rather than through a wrapper.

## Deterministic local checks

These are safe and write no tracked files, and must all pass before a change is handed off:

```sh
go vet ./...
test -z "$(gofmt -l .)"
go test ./...
go build ./...
git diff --check
git diff --cached --check
```

The first four are the Go rungs: vet, formatting, command tests against throwaway repositories and home directories, and a compile of every package. The installer tests cover ownership, collision handling, preservation of edited copies, and provisioning decisions at focused boundaries. The Go suite also carries the reconciler regression suite, including the byte-identical check between the harness copy and the deployed copy and a `go run` smoke run with no Go module. `TestAliasesShellSyntax` runs `bash -n` over `aliases.sh`, the remaining repo shell. `git diff --check` and `git diff --cached --check` catch whitespace errors and conflict markers in unstaged and staged changes. The lifecycle workflow in `.github/workflows/agent-lifecycle.yml` installs Go and runs the reconciler remotely; the unit suite is its local equivalent.

## Core validation contract

The suite protects one behavior set per retained boundary, at the lowest seam that can still fail for the right reason. A boundary's package tests settle its rules; its command tests prove the CLI is wired to them, not the rules again. Adding a second copy of a rule at a higher seam is how the suite grows without protecting more.

| Boundary | Deterministic seam | What it proves |
| --- | --- | --- |
| Managed installation | `cmd/harness/install_checks_test.go`, `cmd/harness/install_test.go` | Install and uninstall touch only unchanged recorded copies, archive collisions, refuse a missing or relative location, and leave edited copies, private entries, and user worktrees alone. Every guarded skill reaches the shared skill namespace, while installation preserves edited and private copies. Codex is configured in `~/.codex` whatever `CODEX_HOME` says, and its uninstall leaves the copies other clients use. |
| Role and model configuration | `internal/models`, `internal/agents`, `cmd/harness/models_test.go` | Presets and per-role overrides resolve to a client, model, and effort; a rejected value writes nothing; install renders the resolved assignment into each client and widens no role's access. |
| Orca adapter | `cmd/harness/launch_args_test.go` | Preparation validates the installed role and returns its client argv without launching, refuses a missing or stale agent, and the generated permission hooks fail closed. A Codex role's argv carries its installed instructions and the network-on workspace-write sandbox. |
| GitHub lifecycle | `skills/ops/setup-harness/scripts/reconcile_agent_lifecycle_test.go` | Label transitions follow blockers and merge evidence, a parent closes only when every required child merged, a parent is never released to ready for an agent, a run reconciles the frontier of the event issue's open enrolled children and siblings, a scheduled sweep reconciles every open enrolled issue, re-reconciling a ready ticket leaves it unchanged, an issue from another repository is never relabeled and counts as merged only on its own repository's evidence, and the deployed reconciler copy is byte identical to the harness copy. |
| Skill integrity | `internal/skills`, `cmd/harness/skills_test.go` | The guard parser counts a guard only where its client reads it, a local skill carries all three guards (Claude Code's and OpenCode's frontmatter keys and Codex's `agents/openai.yaml` policy) or none, the must-guard skills (`land`, `abandon`, `setup-harness`, `build`, `spec`, `triage`, `e2e-verification`, `local-tracker`) carry all three, a retired role override is dropped on load, lock verification separates drift from unverifiable, an unsafe removal is refused, skills.sh runs only in a scratch project whose bodies land in their group, and every shipped skill has both bookkeeping links. |
| Review ledger | `internal/reviewledger`, `cmd/harness/review_test.go` | Each branch has its own directory in the Git directory, a `/` in its name stays inside one directory name, every worktree of the repository reads the same branch's file, and a detached head is refused; a bad field is rejected without a write; an update changes only a finding's disposition, evidence, and commit, so a bug cannot be relabelled to close it; the latest line per finding wins; a fix names a commit after the reviewed one and stays open until that commit is in the head; a finding whose reviewed commit was rewritten stays listed and open; the review level defaults to `medium`, a rejected level writes nothing, setting it keeps every other state key, and an install keeps it. |
| CLI behavior | `cmd/harness/cli_test.go` | Help and invalid invocations exit 0 and 2, spawn no subprocess, write nothing, and read no stdin; retired commands are unlisted and unknown. |
| Status line | `cmd/harness/statusline_test.go`, `internal/statusline` | Recorded payloads render byte for byte against the goldens captured from the shell renderer, and an empty payload reaches neither the keychain nor the usage cache. |
| Checkout discovery | `internal/checkout` | The nearest enclosing checkout is resolved by the policy marker beside the Harness module, a different module is refused rather than substituted, managed ownership is the explicit asset namespaces alone, and the skill tree flattens groups to bare names while refusing a name two groups share or a skill outside a group. |
| Bench accounting | `internal/bench`, `cmd/harness/bench_test.go` | Collection deduplicates clients, windows by date, folds Codex subagent rollouts into their top-level session, and survives damaged or oversized records; the reports match the baselines recorded on the fixture population. |
| Change classification | `internal/changemap`, `cmd/harness/change_map_test.go` | Floor tiers are deterministic and first match wins, configured critical globs beat every other rule, mechanical is assigned only by the lockfile and generated rules and to a locked skill body under `skills/` (flat or grouped) that still matches its lock hash, collapsed to one entry while `skills-lock.json` itself is critical and an edited body is raised and named, code under `skills/` takes the ordinary rules, `criticalPaths` validation names the bad entry, and the changed-file listing covers committed, staged, unstaged, untracked, and deleted files. |

The suite does not snapshot role, skill, or policy prose. It checks executable boundaries such as frontmatter guards, permission hooks, installation ownership, and configuration validation; instruction quality belongs in behavioral evaluation.

What deterministic tests cannot settle: whether a client actually loads a rendered role, whether an Orca worker's retained output survives release, and fresh-parent recovery through each client launch mechanism. Those are behavioral acceptance below, and the live ones need a separately authorized model budget. A fixture or read-back check never stands in for them, and an unavailable check is recorded as unverified rather than as a pass.

`harness bench` measures cost and friction in real sessions, nothing else. It does not measure delivery quality, correctness, or the effect of changing the suite itself. Deleting or consolidating tests is not evidence of a speedup, and neither is a single timed run.

## Behavioral acceptance

Some changes cannot be settled by the checks above, because they act on machine state or on a running client: installer copy reconciliation, the tracked client settings, interactive authentication, and the session metrics a harness change is meant to move. Accept those by running the affected tool and observing it, never by static inspection alone.

`harness install` is a provisioning surface. Run it when the intent is to change machine state, never merely to validate a change. `--no-backup` deletes collisions instead of archiving them and is unrecoverable.

The repository root is the product root and the nested product directory is gone, so a machine installed under the old layout is reinstalled cleanly rather than upgraded in place: uninstall it with the old checkout's own `harness install --uninstall` while that checkout still exists, then install from this one. The new installer claims a link only when it resolves into a managed asset namespace and ships no old-layout detection, migration, or compatibility sweep, so an old link is replaced or refused rather than swept. The Clean reinstall section of `README.md` names the leftovers.

`go run ./cmd/harness install --skip-packages` is the same provisioning surface without the package step. The binary embeds the policy, skills, roles, client settings, and aliases, so `go run` installs the working tree: it copies those assets into each client, records every copy in `~/.harness/state.json`, and copies itself to `~/.local/bin/harness`. An edit under those paths reaches the clients on the next install, not live. It still rewrites the Engram and agent-browser MCP registrations and the tracked client settings, so `engram`, `agent-browser`, and `claude` must already be installed. Run it when copies need reconciling, not as a validation step.

Session-population measurement for a harness change is `harness bench --split <commit-date> --since <window-start>`, documented in `bench/README.md`.

## Conventions

Markdown prose is never hard-wrapped. `.editorconfig` owns indentation: two spaces for Markdown, JSON, and YAML.

Locked skill bodies under `skills/` are not edited in place; use `harness skills` (`add` takes the group) and commit only the reviewed change. Moving a skill to another group is a `git mv` plus repointing its `.agents/skills` link. This repository's tracker is GitHub, and its lifecycle configuration lives in `docs/agents/harness.json`.
