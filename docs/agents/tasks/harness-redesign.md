# Harness redesign

## Objective

Make the harness simpler and less authoritative, add skills that explain the codebase and agent decisions, and classify agent-generated changes so a human knows what to review. The approved plan draws on `mattpocock/skills` (alignment before code) and `backnotprop/pstack` (proof and explanation after code), and fills the gap neither covers: change classification.

## Why

- Capable models over-apply emphatic, procedural instructions; Anthropic's current guidance is to state intent and reasons, keep hard rules few, and enforce zero-exception rules with deny rules, hooks, or guards.
- About 40% of `POLICY.md` was authority ceremony for reversible work or restatement, and several mechanisms enforced nothing.
- Agents produce more code than a human can read line by line; the human needs a map of what matters.

## Constraints

- Work stays in this repository. Do not install the harness or touch `~/.claude`, `~/.config/opencode`, or `~/.harness`; acceptance that needs an installed client stays pending.
- The agent may commit on a non-default branch it created. Pushes, pull requests, tracker writes, and messages need an explicit request.
- Locked third-party bodies change only through `harness skills`.
- TDD: strict, from the session configuration. Runner: `go test ./...`. Prose-only changes get structural readback instead.

## Checks

`go vet ./...`, `test -z "$(gofmt -l .)"`, `go test ./...`, `go build ./...`, `git diff --check`, `git diff --cached --check`.

## Delivery

Strategy: one pull request per phase, stacked: each phase branch starts from the previous one and targets it, so each PR shows only its phase. Phase 0 is #123.

## Phase 0: lighten the rules

- [x] P0.1 Rewrite `POLICY.md` as a short hard floor plus a judgment layer, with a rule-to-enforcer table and agent-branch commits. Route: inline (one file).
- [x] P0.2 Guard only human-triggered skills: `land`, `abandon`, `setup-harness`, `grill-with-docs`. Unguard `review`, `evidence`, `grilling`, `local-tracker`, `continuity`; drop the read-by-path rule. Route: inline (one Go rule plus mechanical frontmatter).
- [x] P0.3 Remove the unused `harness state` command and `internal/state`. Route: delegated writer (several files).
- [x] P0.4 Local skills: drop the Activation/Hard Rules skeleton and injection boilerplate, remove forced asks, fix stale cross-references. Route: delegated writer (many files).
- [x] P0.5 Roles and denies: optional code-search symbol plus `git log`/`git blame`, read-only `gh` for the executor, sliced review of large diffs, allow `git restore --staged`. Route: delegated writer.
- [ ] P0.6 No-op pass: headless before/after scenarios for cut rules. Pending a separately authorized model budget.

## Later phases

- [x] P1 Simplify and re-lock: fold `feedback` into `address-review`; remove `resolving-merge-conflicts` (retired upstream); re-lock the six mattpocock bodies; `CONTEXT.md` to `GLOSSARY.md`. `ask`, `create-ticket`, and `harness-audit` are removed in P4, P5, and P6 alongside their replacements so no PR leaves a gap. Branch `refactor/simplify-skills`, stacked on Phase 0.
- [x] P2 `change-map` skill with `criticalPaths`, wired into `open-pr`, `review`, and policy, plus the deterministic `harness change-map` floor. Branch `feat/change-map`, stacked on Phase 1.
- [x] P3 Verification: certainty ladder (shipped with P2), reviewer proof rule, adversary mode, patch-id check in `land`, benchmark checklist, feature-map modes in `e2e-verification`. PR #126.
- [x] P4 `explain`: how, why, state view; retires `ask`. Read-only history access for `code-search` was dropped: read-search roles share one hardened permission hook, and `explain why` runs `git log`/`blame` in the parent, which already has Git. PR #127.
- [x] P5 Planning to building: `spec` absorbs `create-ticket` (seam checkpoint, rationale, tracer-bullet tickets), `proto` logic and UI shapes, `continuity` decisions table and phase boundaries, guarded `build` for unattended ticket graphs.
- [x] P6 `retro` with the landing table and bench registration; retires `harness-audit`. PR #129.
- [x] P7 `harness-help` and README restructure. PR #130.
- [x] P8 Statechart pilot on the lifecycle reconciler: `statechart` locked from pstack; the model found two real reconciler bugs, both replayed as failing Go tests and fixed.

## Progress

Phases 0 to 8 are done, one stacked pull request per phase (#123 to #131). P0.6, the no-op behavioral pass, waits for a model budget; live client loading waits for a deliberate `harness install`.

## Evidence

- P0.1 `ea53afc`: POLICY.md 1,772 to 1,578 words; never/must/always/do-not from 25 to 6; each hard rule stated once in the floor table with reason and enforcer, which shows OpenCode has no deny rules. `go test ./internal/checkout/ ./cmd/harness/` passed. RDD assess: medium, under budget.
- P0.2 `2b9e8dd`: RED observed (`review: missing disable-model-invocation`), then GREEN for `go test ./internal/skills/` and `go test ./cmd/harness/ -run 'Skill|Install|Guard'`. The five unguarded skills appeared as model-invocable in the running session. RDD assess: medium, under budget.
- P0.3: RED observed (`help still lists state`; `state exited 2 with stderr "Usage: harness state"`), then GREEN. Removed `internal/state` (514 lines), its seven command tests and their helpers, and the dead Worktrunk shim parameter of the `harness` test helper. `go vet`, `gofmt`, `go build`, `go test ./...`, and `git diff --check` all passed.
- P0.4 `1d26d89`: 18 local skills plus `aliases.sh` moved to purpose, why, steps, output. Emphatic words in the edited files went from 140 to 44, with total words about flat because reasons replaced boilerplate. Injection boilerplate, the landing-rule JSON, and restated policy were removed. `land` and `abandon` keep every safety mechanic (lease, fast-forward, required checks, inventory). `setup-harness` infers the tracker mode and asks only on conflicting evidence or when a GitHub remote is the only signal, because GitHub mode writes to the remote. `go test ./...` passed (writer); `git diff --check` clean.
- P0.5: `code-search` symbol is optional (`-` when a line has none); `review` splits an oversized diff into slices for separate fresh reviewers. Kept as is, with reasons: the executor `gh` deny (allowing reads inside a deny list leaks, e.g. `gh api -X POST`); the `git restore` deny (deny rules win over allow, and unstaging works through `git reset -- <path>`). `--force-with-lease` is not matched by `Bash(git push --force:*)`, since the docs say the space before the wildcard is part of the rule; this is inferred from the docs, not run. Read-only `git log` and `git blame` for code-search move to Phase 4, where `explain why` needs them and the reviewer stays without commit rationale.
- RDD: disabled for this clone at the user's choice after the preflight stalled on an undocumented untracked-selection format caused by Gentle AI's own `.atl/` cache.
- Review (data loss and merge authority, `land`, `abandon`, POLICY): the harness `reviewer` agent is not provisioned on this machine and installing was out of scope, so a fresh-context substitute ran with `skills/review/roles/reviewer.md` as its brief and no rationale. Round 1 on `40b4c50..db38579` found one bug (the worktree floor row read as permission to remove user worktrees) and four risks (a wide experiment-worktree exception, a force-push rule that contradicted `land` and `abandon`, overstated deny enforcement, memory and plans missing from the task-data row), plus two nits and one question. All were fixed in `a7cd929` and confirmed resolved in round 2. Round 2 found that `land`'s dependent repair used a bare `--force-with-lease` (pre-existing, now named by the floor); fixed in `48a0416` with a lease on the recorded `headRefOid`, and the `abandon` ticket-thread wording now makes the live confirmation the authority. These last fixes are self-checked only, because review caps at two rounds.
- P1 `78993fe`, `8054a9b`: `harness skills update` refreshes every lock, so non-mattpocock skills (`impeccable`, `orchestration`, `ponytail`) were restored to their reviewed locks and only the six mattpocock entries changed. The upstream diff was read before commit: the glossary rename, fresh-context subagents, a mutation check in `diagnosing-bugs`, a `$TMPDIR` hint in `handoff`. `TestSkillsVerifyMatchesThePinnedLock` now expects 11 locks. Full `go test ./...` passed apart from that count, which was then fixed and rerun; `skills verify` matches all 11.
- P2 `d6fd0d9`, `c0fac1d`, `2eaabcd`, `f0a90ee`, `4332b5d`: `internal/changemap` and `harness change-map` assign floor tiers (configured and built-in critical globs, lockfile and generated mechanical, tests and docs, agent instructions as structural); `skills/change-map` raises tiers by judgment, never lowers them, and carries the certainty ladder now defined in `verification`. Strict TDD: compile-level RED, then command RED (`unknown command "change-map"`), then GREEN; later REDs for `AGENTS.md`/`CLAUDE.md` as instructions and for the floor guarantees. Dogfood: the reconciler's parse of the new `harness.json` was run (ladder step 4). Substitute review of the critical hunks: round 1 found a bug (the working-tree config let a branch lower its own floor, reproduced), a policy-scope risk, and two nits, all fixed and confirmed in round 2; round 2 found that a caller-chosen `--base` could still lower the floor, fixed in `4332b5d` by also counting the default branch's merge base (self-checked, review cap reached). `go test ./...` passed at `f0a90ee`; the final fix reran `internal/changemap` and the change-map command tests.
- P3 `57767d5`, `a4bdbe3`, `e1d9fc1`: substitute review of `land` (critical path) reproduced two bugs in scratch repositories (`patch-id --stable` ignores whitespace; unresolvable refs give equal empty ids) and six risks; fixed and confirmed in round 2; the round-2 rebase false-stale risk was fixed after the cap (self-checked). The PR body's `Reviewed:` line names `a4bdbe3`, so `/land` will flag the self-checked commit honestly.
- P4 `e93ee36`: `explain` skill; no critical items, so no review.
- P5 `14b219c`, `792f148`: RED observed for the `build` guard, then GREEN; `go test ./...` passed. Substitute review (POLICY critical; `build` authority): one bug (`build` could remove worker worktrees outside the floor) and two risks (publishing a spec created tickets; a prototype link could edit a published ticket), plus three questions; all fixed and confirmed in round 2, and the floor now discloses that a native Claude Code executor's commit and push limits are instructions only.
- P6 `bce9e13`, `b5b7a4e`, `1f816b6`: RED then GREEN for the `retro` guard. Substitute review: two risks (writing the proof check before any pick; recording unanswered items as rejected) and two questions, all resolved in round 2.
- P7 `9a1fcf9`, README commit-rule fix: no critical items, so no review; no README inline code span was lost.
- P8: `mise` installed with Homebrew at the human's request; the locked `statechart` body is byte-identical to the upstream tarball read before locking, and its only download is `tla2tools.jar`, pinned by sha256. The model (`docs/agents/models/reconciler/`) broke `ParentNeverReadyForAgent` (a parent closed by hand and reopened before the run was reset to ready-for-agent) and `BlockedExactlyWhileBlockerUnmerged` (the workflow's one-slot pending queue let a later event replace a blocker's close event). Both replayed as failing Go tests, fixed in `df6618f`, and the fixed model holds all four rules over 6,714 states. Substitute review of the reconciler (critical path) found a third path the model hid (a parent blocked by its own blocker was released to ready-for-agent), an overclaimed healing scope, an API-volume risk, and a missing sibling test; fixed in `fac11ee`, confirmed in round 2. Round 2's extra sub-issue call per child was removed in `9a21dfc` by looking up sub-issues only before a label change (self-checked, review cap reached). Residuals: healing covers the event issue's children and siblings, not deeper or unrelated trees; an open enrolled sub-issue from another repository would be fetched by number from this one; any sub-issue makes an issue a parent, as in `reconcileParent`.

## Next step

Land the stack bottom-up with `/land`, starting at #123. Then P0.6 when a model budget is authorized, and live client acceptance after a deliberate `harness install`.
