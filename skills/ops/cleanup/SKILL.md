---
name: cleanup
description: "Retire the worktrees, local branches, and review records that finished work left behind, after an audit and only the ones the human names. Invoke only by name: \"clean up worktrees\", \"prune what merged\"."
argument-hint: "[worktree paths or branches to retire; none audits every linked worktree]"
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

Retire the worktrees, local branches, and review records that finished work left behind, losing nothing.

Worktrees or branches: $ARGUMENTS

## Why

Removing a worktree deletes its uncommitted and ignored files, and deleting a branch drops every commit no other ref holds. `harness worktree retire` owns the checks that make a removal lossless and refuses when one fails, so each skill that retires a worktree it created (`land`, `review`, `build`, `research`) calls it instead of restating them. This skill is the sweep for what those never reach: a pull request merged on the forge, an abandoned branch, a run that died.

The audit cannot see whether another session is working in a worktree, so its `safe` is advice. The human names what goes: the paths in the invocation, or their answer to the audit.

## Steps

1. Audit. Run `git fetch --prune`, then `harness worktree audit`, with `--into origin/<branch>` when pull requests target a branch other than the default. `harness worktree audit --help` defines each state.
2. Read the forge for every `unlanded` row: `gh pr list --head <branch> --state all --json number,state,headRefOid`. A `MERGED` pull request whose content the default branch has since rewritten is still landed: run `git fetch origin refs/pull/<n>/head` and note `--into <headRefOid>` for step 4, which proves the branch holds nothing the merged pull request lacked. An `OPEN` pull request holds its row.
3. Show the human every row with what retiring it would delete and your recommendation, and wait for the set to retire unless the invocation already named it. Recommend holding a `dirty`, `loose`, or `locked` row and any row with an open pull request.
4. Retire each named row with `harness worktree retire <path>` and the `--into` from step 2, or `harness worktree retire --branch <name>` for a branch no worktree holds. A refusal changes nothing and names its reason; answer it only as below, and otherwise hold the row.
   - Ignored paths: pass `--allow-ignored` when every listed path is regenerable build output. A local configuration file or a secret (such as `.env*` or local settings) holds the row, so the human can move it.
   - Uncommitted work: show `git -C <path> status --short`. The human chooses to commit it, to keep it with `--save-patch <file>`, or to hold the row.
   - A branch not proven landed: pass `--keep-branch` when the human wants the checkout gone and the branch kept.
   - A `loose` head: put a branch on it first (`git branch <name> <head>`) when the human wants its commits kept.
   - A `missing` row: `git worktree prune`.
5. Run `harness review prune --dry-run`, which lists every review directory whose key is neither a local nor a remote-tracking branch: a fork's `pr-<n>` and a range's `range-<base>-<sha>` are always listed. Show the list beside step 3's rows, marking each `pr-<n>` whose pull request is still open (`gh pr view <n> --json state`), since its ledger is what `land check` reads. Run `harness review prune` only once the human approves the whole list; it cannot spare one directory, so if the human holds any, do not run it and report that. A directory it keeps names a review worktree in its `worktree.txt`; when step 4 retired that worktree, delete the `worktree.txt` and repeat this step.
6. Report each worktree removed, branch deleted, and patch saved, then each row held with its reason. Remote branches stay as they are: deleting one is a forge write that belongs to `land` or to the human.
