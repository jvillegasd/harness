# Repository guidance

This file tells an agent how to navigate, prepare, and verify this repository. It carries repository facts and conventions only.

The floor in `FLOOR.md` is installed as each client's global instruction file and holds the rules that apply on every turn in every repository: authority, preservation, secrets, untrusted text, working, and prose. Procedures live in the skills. This file never restates either, and where they meet it is more specific: this repository's own commands, paths, and conventions win.

A nested `AGENTS.md` in a subtree, when one is added, scopes to that subtree and overrides this file there. Keep repository-wide rules here, not in a subtree, and do not widen a subtree's rules to the repository.

## What this repository is

The Harness AI installation that serves Claude Code, Codex, and OpenCode. `README.md` is the user-facing overview of the workflow, the skill supply chain, and validation, and `FLOOR.md` is the floor installed into each client. The product assets live at the repository root; there is no nested product directory.

## Directories

| Path | Holds |
| --- | --- |
| `go.mod`, `cmd/harness/` | The Go module and the `harness` command, the single entry point for operating the harness |
| `internal/checkout/` | Checkout discovery and canonical asset locations, shared by the commands that resolve a checkout; `SkillDirs` is the one reader of the grouped skill tree |
| `internal/install/` | The installer behind `harness install` and `harness uninstall` |
| `internal/skills/` | The third-party skill locks behind `harness skills`, and the invocation-guard check on local skills |
| `internal/bench/` | The session-population comparison behind `harness bench` |
| `internal/changemap/` | The deterministic floor tiers behind `harness change-map`, the first pass of the `change-map` skill |
| `internal/agents/` | The role-agent renderer the installer calls in-process |
| `internal/models/` | The per-machine role model presets behind `harness models` |
| `internal/statefile/` | Shared read and atomic write of `~/.harness/state.json`, whose top-level keys the installer, `harness models`, and `harness review level` each own |
| `internal/resume/` | The task context `harness resume` prints for the clients' compaction hooks |
| `internal/reviewledger/` | The per-branch local review ledger behind `harness review`, which records findings and their dispositions inside the Git directory, resolves a review target, answers whether a recorded review covers a patch, and prints the pull request's review line (`harness review target`, `covers`, and `stamp`, sharing the patch comparison `land` uses), and holds the review level behind `harness review level` |
| `internal/land/` | The landing verdict behind `harness land check`: the lane, mode, forge gates, review coverage, and dependents that the `land` skill acts on instead of restating; the default-branch sync, post-merge cleanup, and stacked-dependent repair behind `harness land sync`, `harness land finish`, and `harness land repair`; and the readiness verdict and thread writes behind `harness pr status` and `harness pr threads`, built on the same gates and review comparison |
| `internal/worktree/` | The worktree audit and the lossless retirement behind `harness worktree`, which `land`, `review`, `build`, `research`, and `cleanup` call instead of restating its checks |
| `FLOOR.md` | The floor: the rules installed into each client that hold on every turn |
| `GLOSSARY.md`, `docs/adr/` | The harness's domain terms and its decision records, which `domain-modeling` reads and writes |
| `skills/<group>/<name>/` | Independent capabilities (also invoked as `/name`), grouped by type: `workflow` (what a normal session invokes), `support` (the shared helpers those skills and the floor call), `principles` (the locked engineering principles), `planning`, `design`, `assurance`, and `ops`. Locked bodies sit in their group beside local ones; clients see only the bare name |
| `skills/assurance/e2e-verification/SKILL.md`, `skills/assurance/e2e-verification/scripts/` | Targeted E2E runs on a feature, bugfix, PR, or path over the project's `verify-<app>` skill, with real screenshots per test, and the run kit and report builder it copies |
| `clients/` | Client settings, the Codex and OpenCode compaction hook files, Codex's command rules and OpenCode's permission rules, and the role assignments the installer reconciles |
| `roles/` | Canonical roles no skill owns, the executor and code-search |
| `aliases.sh` | The managed yolo-mode shell aliases |
| `bench/` | Cohort config and the manual for `harness bench` |
| `docs/agents/` | This repository's tracker and lifecycle configuration, the task documents `continuity` keeps in `tasks/`, and the ticket drafts `tickets` writes to `tickets/` |
| `.github/workflows/`, `.github/scripts/` | The agent-lifecycle workflow and reconciler |
| `skills-lock.json` | Reviewed third-party skill locks |
| `.agents/skills`, `.claude/skills` | One link per skill by bare name: `.agents/skills/<name>` points at the grouped body, `.claude/skills/<name>` at that link. Add a skill with `harness skills add` or link it by hand; `TestShippedSkillLinksResolve` fails on a missing or stale link |

## Prerequisites

macOS or Linux with zsh or bash, Homebrew, and Go 1.26 or newer. The GitHub tracker operations need the `gh` CLI authenticated with issue write permission. A delegated role assigned to the session's own client runs as a native subagent; a role assigned to another client runs only in a T3 Code session, through its `delegate_task` tool. Inline work needs neither.

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
| Managed installation | `cmd/harness/install_checks_test.go`, `cmd/harness/install_test.go` | Install and uninstall touch only unchanged recorded copies, archive collisions, refuse a missing or relative location, and leave edited copies, private entries, and user worktrees alone. Every guarded skill reaches the shared skill namespace, while installation preserves edited and private copies. Codex is configured in `~/.codex` whatever `CODEX_HOME` says, and its uninstall leaves the copies other clients use. Codex gets the command rules as its own rules file, and OpenCode gets the command and secret rules in `opencode.json` after the user's own, since its last matching rule applies; the uninstall takes out only the harness's. A named component or `--client` confines a run to those copies and clients, a named client that is not installed is refused before any write, and a tool on `PATH` is never installed over. Only a tool the install recorded adding is uninstalled, only when its component is named or `--all` is given, a shared one only once no client keeps harness config, and old `policy` and `mcp` records read as `floor` and `mcps`. The alias line goes into the startup file the login shell reads (none for another shell, with a warning naming `~/.harness/aliases.sh`), and an uninstall strips it only from files carrying the harness marker. |
| Role and model configuration | `internal/models`, `internal/agents`, `cmd/harness/models_test.go` | Presets and per-role overrides resolve to a client, model, and effort, and a review lens follows the reviewer's assignment until it is overridden; a definition that extends another renders the base's text before its own, only one level deep and only at the same access; a rejected value writes nothing; install renders the resolved assignment into each client and widens no role's access. |
| Read-only guard | `internal/models/guard_test.go` | The read-search roles' permission hook allows only read and search commands on paths inside the working directory, and denies writes, other programs, paths or links leading out, credential-shaped files, and malformed requests. |
| GitHub lifecycle | `skills/ops/setup-harness/scripts/reconcile_agent_lifecycle_test.go` | Label transitions follow blockers and merge evidence, a parent closes only when every required child merged, a parent is never released to ready for an agent, a run reconciles the frontier of the event issue's open enrolled children and siblings, a scheduled sweep reconciles every open enrolled issue, re-reconciling a ready ticket leaves it unchanged, an issue from another repository is never relabeled and counts as merged only on its own repository's evidence, and the deployed reconciler copy is byte identical to the harness copy. |
| Skill integrity | `internal/skills`, `cmd/harness/skills_test.go` | The guard parser counts a guard only where its client reads it, a local skill carries all three guards (Claude Code's and OpenCode's frontmatter keys and Codex's `agents/openai.yaml` policy) or none, the must-guard skills (`land`, `setup-harness`, `build`, `work`, `spec`, `tickets`, `triage`, `e2e-verification`, `local-tracker`, `cleanup`) carry all three, an obsolete role override is dropped on load, lock verification separates drift from unverifiable, an unsafe removal is refused, skills.sh runs only in a scratch project whose bodies land in their group, every shipped skill has both bookkeeping links, and no Markdown in the floor, the roles, or a local skill carries an em-dash. |
| Review ledger | `internal/reviewledger`, `internal/land/covers_test.go`, `cmd/harness/review_test.go`, `cmd/harness/review_covers_test.go`, `cmd/harness/review_stamp_test.go`, `cmd/harness/review_target_test.go` | Each branch has its own directory in the Git directory, a `/` in its name stays inside one directory name, every worktree of the repository reads the same branch's file, and a detached head is refused; a bad field is rejected without a write; an update changes only a finding's disposition, evidence, and commit, so a bug cannot be relabelled to close it; the latest line per finding wins; a fix names a commit after the reviewed one and stays open until that commit is in the head; a finding whose reviewed commit was rewritten stays listed and open; removing a branch deletes its directory alone, does nothing when there is none, and is refused untouched while a review worktree is still recorded; pruning removes only directories whose branch exists neither locally nor as a remote-tracking branch, keeps one that records a review worktree, and a dry run deletes nothing; the review level defaults to `medium`, a rejected level writes nothing, setting it keeps every other state key, and an install keeps it. `harness review covers` counts only the latest `clean` record at the asked depth or deeper, keeps it across a rebase that changes no file's patch, matches a digest record only to the same digest, lists the findings still open, writes nothing, and agrees with `land check`'s `current` for a ledger-only review. `harness review stamp` prints `Reviewed:` only from a covering `clean` record with no finding open, `Review: not run` from a `no-review` record, and otherwise nothing with the reason, and its line reads back through `land check` as a current body review. `harness review target` resolves the current branch, a pull request (refusing a fetched head the forge does not report), a branch, or a range to its base, commit, and key, keys a pull request as `land check` does, a fork's as `pr-<n>`, and creates no branch or tag. |
| Worktree retirement | `internal/worktree`, `cmd/harness/worktree_test.go` | A retirement removes a linked worktree and its branch only when nothing is lost: uncommitted work, ignored files, a branch whose content is not on the `--into` ref, a detached head no ref holds, a locked worktree, and the main worktree are each refused with nothing changed; a squash merge counts as landed and a default branch that rewrote the same lines does not; `--save-patch` writes the work before a dirty removal, `--at` accepts a loose head only at the named commit, `--keep-branch` removes the worktree alone, and the branch `--into` names is never deleted; the audit reports the same state for every linked worktree and removes nothing. |
| Landing verdict | `internal/land`, `cmd/harness/land_test.go`, `cmd/harness/pr_test.go` | Only an explicit merge request opens the merge lane, and a draft or closed pull request stops; each forge gate (mergeable state, merge state, a required check that is not passing or cannot be read, a missing approval or requested change, an unresolved thread, a base that is not the default branch) is its own hold, and a merged pull request skips them; this checkout is the landing's only when it is the pull request's head, and a fork's branch name never matches a local one; a review is required by level and by the higher of the changed files' tier and the body's, never lowered by the body; the reviewed commit comes from the body, then the ledger's clean line, then a person's approval only on someone else's pull request, where a bot or a standing change request does not count; a rebased but unchanged patch is current, later test or doc edits and later code edits are told apart, and a stale review holds only when one is required; an open or dismissed ledger finding holds and a dropped or fixed one does not; a run is ordered bottom-up, a stack is walked from its tip, and a dependent is marked as the forge user's or not. `harness pr status` reports the same holds and review as `land check --merge` for the same head, lists each unresolved thread with its author's permission, writes nothing to the forge, and exits with the code of the first blocker: conflict, then thread, failing check, pending check, any other hold. `--watch` repeats the pass only while pending checks are the first blocker, retries a failed read twice, and exits 8 with the last verdict on a timeout, on an injected clock. `harness pr threads` replies with a file's exact text and resolves threads, refusing an id that is not the pull request's, or an empty body, before any write, and leaves a resolved thread alone. `harness land sync` fast-forwards the default branch where a worktree holds it or by moving the ref, and leaves a diverged one; `harness land finish` refuses an unmerged pull request before any removal, keeps a remote branch that is someone else's or still a base, keeps a held review directory and goes on, touches only a fork's review directory, retires this checkout last, and changes nothing on a second run. `harness land repair` refuses a dependent that is someone else's, a fork's, holds a merge commit, or whose boundary is not proven on the default branch by content, before any retarget; puts the branch back on a conflict or a changed own patch; and pushes only with a lease on the head it read. |
| CLI behavior | `cmd/harness/cli_test.go` | Help and invalid invocations exit 0 and 2, spawn no subprocess, write nothing, and read no stdin. |
| Compaction context | `internal/resume`, `cmd/harness/install_checks_test.go` | `harness resume` prints the branch, its head, and the next action of each task document the branch changed, capped, and nothing outside a checkout; the install puts a compaction hook in every client, and Claude Code's merge replaces only the harness's own hook group, keeping the user's hooks through install and uninstall. |
| Status line | `cmd/harness/statusline_test.go`, `internal/statusline` | Recorded payloads render byte for byte against the goldens captured from the shell renderer, an empty payload reaches neither the keychain nor the usage cache, and a `CLAUDE_CONFIG_DIR` profile gets its own keychain item, credentials file, settings, and usage cache. |
| Checkout discovery | `internal/checkout` | The nearest enclosing checkout is resolved by the floor marker beside the Harness module, a different module is refused rather than substituted, managed ownership is the explicit asset namespaces alone, and the skill tree flattens groups to bare names while refusing a name two groups share or a skill outside a group. |
| Bench accounting | `internal/bench`, `cmd/harness/bench_test.go` | Collection deduplicates clients, windows by date, folds Codex subagent rollouts into their top-level session, drops sessions run under the scratch-projects directory, and survives damaged or oversized records; the reports match the baselines recorded on the fixture population. |
| Change classification | `internal/changemap`, `cmd/harness/change_map_test.go` | Floor tiers are deterministic and first match wins, configured critical globs beat every other rule, mechanical is assigned only by the lockfile and generated rules and to a locked skill body under `skills/` (flat or grouped) that still matches its lock hash, collapsed to one entry while `skills-lock.json` itself is critical and an edited body is raised and named, code under `skills/` takes the ordinary rules, `criticalPaths` validation names the bad entry, and the changed-file listing covers committed, staged, unstaged, untracked, and deleted files. |

The suite does not snapshot role, skill, or floor prose, beyond the em-dash check. It checks executable boundaries such as frontmatter guards, permission hooks, installation ownership, and configuration validation; instruction quality belongs in behavioral evaluation.

What deterministic tests cannot settle: whether a client actually loads a rendered role, whether a T3 Code delegate's result survives its release, whether each client runs its compaction hook from the session's working directory (Codex only once the hook is trusted in `/hooks`), and fresh-parent recovery through each client launch mechanism. Those are behavioral acceptance below, and the live ones need a separately authorized model budget. A fixture or read-back check never stands in for them, and an unavailable check is recorded as unverified rather than as a pass.

`harness bench` measures cost and friction in real sessions, nothing else. It does not measure delivery quality, correctness, or the effect of changing the suite itself. Deleting or consolidating tests is not evidence of a speedup, and neither is a single timed run.

## Behavioral acceptance

Some changes cannot be settled by the checks above, because they act on machine state or on a running client: installer copy reconciliation, the tracked client settings, interactive authentication, and the session metrics a harness change is meant to move. Accept those by running the affected tool and observing it, never by static inspection alone.

`harness install` is a provisioning surface. Run it when the intent is to change machine state, never merely to validate a change. `--no-backup` deletes collisions instead of archiving them and is unrecoverable.

`go run ./cmd/harness install` is the same provisioning surface over the working tree. The binary embeds the floor, skills, roles, client settings, and aliases, so `go run` installs the working tree: it copies those assets into each client, records every copy in `~/.harness/state.json`, and copies itself to `~/.local/bin/harness`. An edit under those paths reaches the clients on the next install, not live. A tool already on `PATH` is never installed over, so with every tool present it calls no package manager; name components (`go run ./cmd/harness install skills agents`) to reconcile only those. Selecting `mcps` or `settings` still rewrites the Engram and agent-browser MCP registrations and the tracked client settings. Run it when copies need reconciling, not as a validation step.

Session-population measurement for a harness change is `harness bench --split <commit-date> --since <window-start>`; `harness bench --help` lists its options. It measures cost and friction after a change ships; whether the change makes agents behave as intended, before it ships, is `/work`'s eval playbook.

## Where an instruction lives

Every instruction has one home. Before you add or change one, find its home and every existing copy, change it in the home, and delete the copies.

| The instruction | Its home |
| --- | --- |
| Must hold on every turn, in every repository, whether or not a skill runs | The floor (`FLOOR.md`, installed as each client's global instruction file) |
| Applies when a kind of situation comes up (debugging, a measured number, writing a test) | That `principle-*` skill, which `/work`'s trigger list names |
| Decides which procedure a task follows and the order skills run in | `/work` and its playbooks |
| Is a step in one procedure | The skill that owns that procedure |
| Is a step several skills share | One support skill that the others name |
| Is a fact about one repository (commands, paths, conventions) | That repository's `AGENTS.md` |
| Explains the harness to a person | Human docs, never read as instructions |

A rule that seems to need two homes belongs in the more general one, and the other refers to it by name. A rule that tooling can enforce (a client permission rule, a hook, a test) lives in the tooling, and the prose keeps at most its reason.

Human docs are the `README` files and the prose guides under `docs/`. Keep them accurate when a change alters what they describe, but take no instruction from them. `docs/adr/`, `docs/agents/`, and agent-read data such as a `verify-<app>/features/README.md` feature map are not human docs.

## Conventions

Markdown prose is never hard-wrapped. `.editorconfig` owns indentation: two spaces for Markdown, JSON, and YAML.

Locked skill bodies under `skills/` are not edited in place; use `harness skills` (`add` takes the group) and commit only the reviewed change. Moving a skill to another group is a `git mv` plus repointing its `.agents/skills` link. This repository's tracker is GitHub, and its lifecycle configuration lives in `docs/agents/harness.json`.
