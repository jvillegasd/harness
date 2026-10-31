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

Strategy: `ask-on-risk`. Each phase is one or two pull requests; Phase 0 is branch `refactor/lighten-policy`.

## Phase 0: lighten the rules

- [ ] P0.1 Rewrite `POLICY.md` as a short hard floor plus a judgment layer, with a rule-to-enforcer table and agent-branch commits. Route: inline (one file).
- [ ] P0.2 Guard only human-triggered skills: `land`, `abandon`, `setup-harness`, `grill-with-docs`. Unguard `review`, `evidence`, `grilling`, `local-tracker`, `continuity`; drop the read-by-path rule. Route: inline (one Go rule plus mechanical frontmatter).
- [ ] P0.3 Remove the unused `harness state` command and `internal/state`. Route: delegated writer (several files).
- [ ] P0.4 Local skills: drop the Activation/Hard Rules skeleton and injection boilerplate, remove forced asks, fix stale cross-references. Route: delegated writer (many files).
- [ ] P0.5 Roles and denies: optional code-search symbol plus `git log`/`git blame`, read-only `gh` for the executor, sliced review of large diffs, allow `git restore --staged`. Route: delegated writer.
- [ ] P0.6 No-op pass: headless before/after scenarios for cut rules. Pending a separately authorized model budget.

## Later phases

- [ ] P1 Simplify and re-lock: remove `ask`, `create-ticket`, `feedback`, `harness-audit`, `resolving-merge-conflicts`; re-lock mattpocock bodies; `CONTEXT.md` to `GLOSSARY.md`.
- [ ] P2 `change-map` skill with `criticalPaths`, wired into `open-pr`, `review`, and policy.
- [ ] P3 Verification: certainty ladder, reviewer proof rule, adversary mode, patch-id check in `land`, benchmark checklist, feature-map modes in `e2e-verification`.
- [ ] P4 `explain`: how, why, state view.
- [ ] P5 Planning to building: `spec` merge, `proto` shapes, `continuity` phase boundaries, `build`.
- [ ] P6 `retro` with the landing table and bench registration.
- [ ] P7 `harness-help` and README restructure.
- [ ] P8 Statechart pilot on the lifecycle reconciler.

## Progress

Phase 0 started on `refactor/lighten-policy`.

## Evidence

None yet.

## Next step

P0.1.
