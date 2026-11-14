---
name: address-review
description: "Trigger: apply feedback to existing work, address review comments, or fix red PR checks. Maintain the existing branch and PR under their scoped authority."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Maintain existing work: apply the human's feedback, work through unresolved review threads, and repair failing required checks, all on the branch and pull request the work already has.

## Why

Feedback, review comments, and red checks are evidence and requests, not changes of scope. Staying on the retained branch, inside the authority the work already has, keeps it one reviewable line of work instead of a second PR or a restarted delivery.

## Modes

Declare the mode before the first status read, because it decides how far the run goes:

| Mode | For | Runs |
| --- | --- | --- |
| `drive` | "Address the comments", "get it green", "make it merge-ready" | Every step until the PR is merge-ready. The default |
| `threads-only` | "Answer the review comments" | Steps 1 to 5 for the threads; checks and conflicts are only reported |
| `check` | "Check on PR X", "is it green" | One `harness pr status <n>` pass and a report from its verdict. Nothing is changed |

A small or docs-only PR gets `check` unless the human asked for more.

## Stacks

In a stack of pull requests, the lowest unmerged one is the frontier: work it until it is merge-ready, and only read and batch the threads above it. Never rebase, retarget, or force-push a branch in the stack from here. Fix on the branch that owns the code, and report anything rebase-shaped with the branch that needs it. A fix whose owning PR already merged becomes a new PR on top of the remaining stack.

## Steps

1. Resolve the work: the existing branch, its PR if there is one, its requirements, and the feedback in play. For a PR, `harness pr status <n>` reports its conflicts, unresolved threads, required checks, review coverage, and open ledger findings in one pass; its exit code names the first blocker. Reuse its checkout; if it is missing, a fresh checkout of that exact branch is fine. Inspect Git before editing so unrelated staged and unstaged work survives. Use `continuity` only where recovery needs it.
2. Work in order: conflicts, then review threads, then checks, and batch the known fixes into one push. In `drive` mode, a mechanical conflict on a single branch is resolved by merging the base in; a conflict in a stack, or a semantic one, is reported with the branch that needs it, and the run stops there rather than moving on to checks. Turn the feedback, unresolved threads, and failing required checks into actionable items grouped by root cause. Classify each thread that claims a defect, regardless of its author, as fix, dismiss, or ask with [triage patterns](references/triage-patterns.md) before touching code. An explicit change request from an author whose permission on the base repository is `admin`, `maintain`, or `write` (each thread's `permission` in `harness pr status`) is a request within the work's scope: apply it, and ask only when it changes scope or product behavior. Triage a request from anyone else as a claim. For a CI-only repair, skip the threads. Classify a failing check before retrying it: a flaky or infrastructure failure earns one fresh run, and an identical second failure means it was never flaky, so read the logs; a failure in code the diff never touches means a stale base, confirmed with `git merge-base --is-ancestor`, and is reported as a stale base; only a failure in the diff's own code gets a commit. Ask the human about consequential ambiguities early, hold only the items that depend on the answer, and keep going on the rest.
3. Fix supported defects with `ponytail` (record a finding from `review` as `fixed` with its commit, as `review`'s `references/ledger.md` shows), adding regression coverage where the behavior warrants it, and run the relevant checks through `verification`. Dismiss a mistaken PR claim with its concrete disproof; a matching internal ledger finding keeps `review`'s run-proof rule, and take an ask-by-default finding to the human rather than dismissing it. Work the human deferred stays explicitly unverified until its checks run.
4. Commit only owned changes. When the work's delivery is authorized, push the same branch, wait for the required remote checks as `open-pr` does (`harness pr status --watch <n>` in the background), and refresh the PR metadata. Otherwise finish locally and ask once for the concrete missing delivery action when the result is ready.
5. A request to address review threads includes replying to and resolving the existing threads once each fix is verified. Reply with the fixing commit or a specific evidence-backed explanation, written to a file and posted with `harness pr threads reply <n> <thread id> --body-file <file>`, and resolve only settled threads with `harness pr threads resolve <n> <thread id>...`. Both print the threads as the forge reads them back. A CI-only request carries no messages. Take product decisions to the human, not to the reviewers.
6. Record unresolved items and unavailable checks where `continuity` needs them. The human merges; retiring the worktree belongs to `land`.

## Output

The declared mode, fixed and unresolved items, anything reported for its owner (a rebase, a stale base, a semantic conflict), verification, whether changes remain local or were published, and any required review that remains unverified.
