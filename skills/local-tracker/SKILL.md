---
name: local-tracker
description: What a tracker operation means when the tracker is committed markdown files rather than a service.
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Local Tracker

Read this when `docs/agents/harness.json` sets `"tracker": "local-markdown"`. It maps command tracker operations to committed files without replacing workflow steps. Writes need a branch, commit, and push as specified below. Local markdown is tracker-bound: `/proto`, `/ship`, `/land`, and `/abandon` run their ticket transitions.

## Layout

One directory per feature under `spec_home`, committed with the code:

```
<spec_home>/<feature-slug>/spec.md
<spec_home>/<feature-slug>/issues/<NN>-<slug>.md
```

`<feature-slug>` is the lowercase spec title with non-alphanumeric runs replaced by one hyphen and edge hyphens trimmed. `/spec` fixes it at publication; later commands reuse it. `<NN>` starts at `01` in dependency order.

## State

Two lines precede the `## Type` heading in every implementation-ready ticket, and a `Status:` line precedes the first heading of `spec.md`:

```
Status: ready-for-agent
Blocked by: 01, 03
```

`Status:` carries a mapped label from `harness.json` or terminal `done`, meaning the ticket's pull request merged. `Blocked by:` lists same-feature ticket numbers or `none`. Record cross-feature dependencies in prose, never as scanned edges.

A `spec.md` with `issues/` files is a parent: mapped `in_progress` from publication, then `done` only when every child is `done`. Report children without `Status:` as blocking closure. A collapsed one-slice `spec.md` has no child files: it is the ticket, carries `Status:` and `Blocked by:`, and becomes `done` on its own merge.

A planning-only ticket lacks `Acceptance criteria` or `Test seams and checks` and has no `Status:`. Exclude it from the frontier but count it as blocking parent closure. Once its contract fields are complete, `/create-ticket` promotes it in place and writes its first `Status:`.

## Identity

The ticket id and branch `<tracker-id>` are `<feature-slug>-<NN>`; numbers alone repeat across features. No reverse parsing of the branch name is needed.

An id resolves to exactly one file, `<spec_home>/<feature-slug>/issues/<NN>-*.md`. That is how `/triage` reads a ticket it was given by id and how `/proto` names a branch from one.

A collapsed spec has no `<NN>`: its id and branch `<tracker-id>` are `<feature-slug>`, resolving to `<spec_home>/<feature-slug>/spec.md` in every command. Treat it as a ticket throughout.

Use the ticket's repository-relative path in the pull request body's prose `Closes` line, including `spec.md` for a collapsed spec. Never use `Closes #<NN>`, which names an unrelated GitHub issue. `/land` finds the pull request by searching bodies for that path and finds the ticket by reading the same line.

## Reading

Comments append under a `## Comments` heading, oldest first. The policy's rule for reading a ticket applies unchanged: read the whole thread, and the newest word wins.

Duplicate detection has no issue listing to run. Grep `<spec_home>` for the spec's key terms instead, and surface every hit.

## Writing state

Tracker state must reach the default branch so later runs see it. Fetch and fast-forward that branch, then commit and push each operation below. Only these listed bookkeeping operations qualify for the policy's default-branch exception; no implementation or unlisted tracker write does.

Resolve the default branch's worktree with `git worktree list`, selecting `<default>`. Run tracker edits there and Git operations with `git -C <that path>`. Never switch the feature worktree with `git checkout <default>`.

Before any write, check `gh api repos/{owner}/{repo}/branches/<default> --jq .protected`. If protected, stop and escalate before creating an unpushable bookkeeping commit.

- **`/spec` publishes.** Write `spec.md` with `Status:` set to mapped `in_progress`. Commit as `docs(tracker): publish <feature-slug>`.
- **`/create-ticket` publishes.** Write every ticket file the many-slice and research-batch routes produce with its `Status:` and `Blocked by:` lines already correct: mapped `blocked` while blockers remain, otherwise mapped `ready_for_agent`. Commit as `docs(tracker): slice <feature-slug>`. This has to land before any `/ship` run, which is the whole reason publication is not deferred to a branch.
- **`/create-ticket` collapses a one-slice spec.** When the spec yields exactly one coherent slice, promote `spec.md` in place instead of writing a ticket file: append the implementation projection (`What to build`, `Acceptance criteria`, `Test seams and checks`) and replace its publication `Status:` line with mapped `blocked` or `ready_for_agent` by its edges, adding a `Blocked by:` line the way any ticket carries one. Those lines are what put the spec itself on the frontier, so the feature directory holds one implementation-ready artifact and `spec.md` is both the spec and the ticket. Commit as `chore(tracker): promote <feature-slug>`. Only a many-slice spec and a research batch add files under `issues/`. Every later transition that names a ticket by `<feature-slug>-<NN>` names a collapsed spec by `<feature-slug>` instead: `/proto` claim, `/ship` claim and handoff, `/abandon` release, and `/land` close.
- **`/create-ticket` promotes.** When a planning-only ticket returns with its contract fields filled, write its first `Status:` line in place, mapped `blocked` or `ready_for_agent` by its edges. Commit as `chore(tracker): promote <feature-slug>-<NN>`. The file keeps its number, so every edge already pointing at it stays valid.
- **`/proto` claims, at its setup step.** Same read and same write as the `/ship` bullet below: re-read on freshly fetched `origin/<default>`, abort if the `Status:` is no longer mapped `ready_for_agent`, otherwise set mapped `in_progress`, commit as `chore(tracker): claim <feature-slug>-<NN>`, and write the claim record into the prototype's ledger. The prototype loop stops for the human every round, so this claim is what keeps the ticket off the frontier for a span measured in days rather than in one run.
- **`/ship` claims, at its worktree step.** Re-read the ticket on freshly fetched `origin/<default>` first. Mapped `ready_for_agent` is claimed: set it to mapped `in_progress` and commit as `chore(tracker): claim <feature-slug>-<NN>`. On the default branch this is what makes the claim visible to another run at all. Mapped `in_progress` has two readings, and what separates them is the run's contract source rather than its branch name, since every tracker-bound branch carries the `<tracker-id>` and so cannot say who claimed the ticket. Where the contract is a `/proto` ledger whose existing branch the run reused, the claim is the one `/proto` already took: verify it against the ledger's claim record, which must name this ticket and this branch, then continue and write nothing. A missing or mismatched record means another run claimed it: abort without retiring the prototype worktree. For every other contract the branch is freshly created, so a ticket already at mapped `in_progress` means another run claimed it between the frontier scan and here: abort as before.
- **`/ship` finishes, when it marks the pull request ready, and `/address-review` when it keeps it there.** Set the ticket to mapped `ready_for_human` and commit as `chore(tracker): hand off <feature-slug>-<NN>`. Where it already carries that value there is nothing to write and nothing to commit. This lands on the default branch, so it adds no commit to the pull request and re-triggers none of its checks; rung 5 stays green.
- **`/abandon` releases a claim.** A ticket left at mapped `in_progress` by a run that died, or by one whose pull request was closed unmerged, is off the frontier with no other way back. Where its branch has no open pull request, set it to mapped `blocked` or mapped `ready_for_agent` by its `Blocked by:` edges and commit as `chore(tracker): release <feature-slug>-<NN>`. Read the edges rather than assuming `ready_for_agent`: a blocker reopened while the run was live has to put the ticket back to `blocked` instead of onto the frontier. Never release one whose pull request is open; that run is still live.
- **`/land` retires.** After the merge, set the ticket to `done`, set every dependent whose blockers are now all `done` to mapped `ready_for_agent`, and set `spec.md` to `done` only if every file in `issues/` now carries `Status: done`, reporting any ticket still without a `Status:` line as what blocks that closure. A collapsed spec has no files in `issues/` and no `<NN>`: the ticket is its own `spec.md`, so its merge sets that file to `done` and the every-issues-file rule does not apply, because the spec's own `Status:` carries the merge evidence. Commit as `chore(tracker): close <feature-slug>-<NN>`, or `chore(tracker): close <feature-slug>` for a collapsed spec. Do this only after the local default branch has been fast-forwarded past the merge, and before the worktree is retired; committing on a default branch still behind the merge gets the push rejected as non-fast-forward.

These separate default-branch commits stay outside pull request diffs, so review needs no path exclusions. Review contracted spec or ticket edits normally.

## Frontier

Scan every `<spec_home>/*/issues/*.md` on the default branch, not one feature directory, since `/land` reports the frontier for the repository. A promoted one-slice spec is a ticket too, so also scan any `<spec_home>/*/spec.md` whose `Status:` is mapped `blocked` or mapped `ready_for_agent`; a `spec.md` still at mapped `in_progress` with no such status is a parent, not a ticket, and stays out. A ticket is ready when it carries mapped `ready_for_agent` and every number in its `Blocked by:` line is `done`. A promoted `spec.md` is written with a `Blocked by:` line like any ticket, so the scan parses it the same way. Order by feature directory, then by `<NN>`.

`Status:` and `Blocked by:` are the entire lifecycle state. Local mode installs no `.github/` workflow or reconcile script because file changes emit no issue events.
