# Composable capabilities

## Request and authority

Implement the approved “Replace mandatory workflows with composable capabilities” plan. The initial request authorized local implementation only. The subsequent user request, “create pr, squash merge”, explicitly authorizes scoped commits, publication, and squash merging this work. Installation into live clients and model-funded acceptance remain outside the request.

## Requirements

- Ordinary work: understand, implement, check, report. Implementation and plan approval authorize local work; commits and publication need an explicit request. “open a PR” or “ship this” covers necessary delivery without repeated confirmation. Only explicit `/land` authorizes merge.
- Ask unresolved consequential decisions while continuing independent settled work. Choose documentation and delegation by uncertainty, risk, and recovery needs, never file count.
- Remove `ship`; replace `adversarial-review` with `review` and `workflow-checkpoint` with `continuity`; add `verification` and `open-pr`. Retire old names without wrappers. Make `spec` (including tickets) and `proto` optional; keep feedback/review maintenance on its existing branch and authority.
- Preserve investigation, design/domain guidance, tracker support, land/abandon, named roles, and client/model resolution. Update local references, guards, assignments, lifecycle docs and installer expectations. Locked third-party bodies stay unchanged.
- Require focused independent inspection only when requested or a concrete security, data-loss, compatibility, or concurrency risk exceeds ordinary checks. Keep rationale blind, findings revision-bound, substantive repairs rechecked, correction bounded, and unavailable review unverified.
- Bounded recoverable work creates no artifact. Longer work reuses one task/spec document or `docs/agents/tasks/<feature>.md`, holding requirements, acceptance checks, tasks, evidence, pending decisions, next action, and exact user strings/examples. Mirror the full document and locator in one project-scoped Engram observation. Reconcile both copies against Git on resume; preserve conflicts and unfinished work, invalidate stale checks, leave Engram failure explicitly pending, preserve legacy checkpoints. Neither copy grants authority.
- Add small disposable Go behavioral fixtures with a local forge substitute and evaluator expectations outside the agent workspace. No production CLI or model runner. Cover local completion, plan versus delivery authority, unrelated staged work, risk versus routine review, interrupted/conflicting/stale/unavailable continuity, and explicit merge authority.
- Separate fixture checks, live agent acceptance, and transcript assessment. Live scenarios need an explicitly authorized model budget; otherwise compliance remains unverified. Retain `harness bench` for later session-population cost/friction comparisons.
- No installer architecture, model-system, tracker-reconciler, or Engram-service replacement; no RDD engine or mandatory TDD. Live installation is separate provisioning.

## Acceptance checks

Run `go vet ./...`, empty `gofmt -l .`, `go test ./...`, `go build ./...`, `git diff --check`, and `git diff --cached --check`. Rerecord and inspect installer goldens. Verify locked bodies are byte-unchanged and retired names have no active integration references. Fixture tests do not prove live agent compliance.

## Tasks

- Complete: policy and independent capability replacement.
- Complete: local references, named-role paths, guards, lifecycle docs, and installer/role goldens.
- Complete: seven disposable scenarios, test-only forge/memory substitutes, and external evaluator instructions.
- Complete: targeted fixture tests, exporter smoke check, golden inspection, locked-body verification, vet, formatting, and build.
- Complete: final full-suite rerun after excluding bundled skill tests from fixture discovery.

## Evidence and recovery

Branch: `refactor/composable-capabilities`; worktree: repository checkout. Initial Git status was clean. Engram mirror pending: host has no registered runtime identity; attributed writes are prohibited. Existing memory/checkpoint records remain untouched. No live acceptance or installation attempted.

## Verified implementation snapshot before housekeeping follow-up

- Base HEAD: `96d811950896825e8abc59a982d8c6eb7b1be883`.
- SHA-256: `895315e16722586b5520d32acbf84dd9e7c63ab823c7dc15e228ecf9503cc49a` over HEAD, then sorted changed/untracked path names and current file bytes, symlink targets, or deletion markers, separated by NUL; excludes this evidence document to avoid self-reference. The Git index remains unchanged and empty.
- `go vet ./...`: pass.
- `test -z "$(gofmt -l .)"`: pass.
- `go test ./...`: pass, including the seven scenario fixtures and installer goldens without update mode.
- `go build ./...`: pass.
- `git diff --check` and `git diff --cached --check`: pass.
- Installer goldens: rerecorded through `TestInstallerGolden -update`; all six changed inventories inspected. Role goldens match rendered definitions.
- `harness skills verify`: all 12 locked skills match; locked directories and `skills-lock.json` are unchanged against HEAD.
- Exporter smoke: disposable scenario prepared, retained local forge worked after the preparing process exited, and the fixture was removed. No model launched.
- Fixture correction: copied skill Go tests initially contaminated the fixture's `go test ./...`; hidden `.harness-skills/` keeps those assets outside Go package discovery. The fixture now exercises its own checks in the deterministic suite.
- Local instruction references and repository skill links checked; retired capability names have no active references or wrappers.
- These checks validate structure and fixture mechanics, not live policy compliance. No independent live reviewer, client acceptance, real Engram integration, or provisioning run is claimed. The tracker documentation's merge-evidence wording was reconciled to the unchanged reconciler after the Go run; documentation has no exact-prose tests.

## Housekeeping follow-up

The user requested autonomous cleanup of agent-created pollution, including archives, logs, dumps, and installed packages. Added a shared policy rule to prefer disposable environments, clean up task-owned temporary artifacts and unnecessary packages before handoff, preserve deliverables and existing work, and report uncertain leftovers. No new skill or cleanup automation is needed. The snapshot hash above predates this policy addition. After the addition, vet, formatting, the full Go suite, build, and both whitespace checks passed. No packages or temporary artifacts were added for this follow-up; existing ignored files were preserved. Live behavioral acceptance and Engram mirroring remain unverified/pending as above.

## Pending decisions

None blocking repository implementation. Live model budget and provisioning remain outside this request.

## Next action

Local implementation and verification are complete. Publish the scoped changes in a pull request, verify forge merge gates, and squash merge under the user's explicit request. Verify the updated default branch afterward. Live-client installation remains separate; live scenarios require an explicitly authorized model budget. The Engram mirror remains pending until the host registers this session.
