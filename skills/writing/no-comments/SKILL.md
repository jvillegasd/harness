---
name: no-comments
description: "Launch Comment Sicko on a diff or files, fix its accepted findings, and offer encodings for claimed constraints. Invoke only by name: \"no comments\", \"kill the comments\"."
argument-hint: "[files or diff to scan; none scans the current diff against the base branch]"
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  opencode/autoinvoke: "false"
---

# No comments

Launch Comment Sicko. Act on accepted findings.

Scope: $ARGUMENTS

Defer to Comment Sicko's fresh perspective.

## Scope

Use the files or diff the invocation names. Otherwise use the current diff against the base branch, default `main`, including the working tree.

## Steps

1. Give Comment Sicko a worktree of its own, so this checkout changes only by the hunks you accept. Retire any worktree an earlier run left under that key with `harness review worktree retire --branch no-comments-<branch>`, save the scope with `harness review snapshot --branch no-comments-<branch> --base <base> -- <scoped paths>`, create the worktree with `harness review worktree add --branch no-comments-<branch> --patch <patch> <base>` (without `--patch` when the snapshot writes nothing), and stage everything there with `git -C <worktree> add -A`, so `git -C <worktree> diff` shows only the worker's edits. Launch Comment Sicko through the **delegation** skill as the `worker` at the `high` tier, for independent inspection. Its brief is [`references/comment-sicko.md`](references/comment-sicko.md) verbatim, then the scope, the worktree path as the only place it may edit, and that it stages nothing. Do not restate its rules. When no subagent can launch, run that prompt yourself in the worktree in a fresh pass over the scope and say so.
2. Inspect its report and diff. Reject application-code edits, scope escapes, exception-protected deletions, misstated `MUST KILL` reasons, and flags that treat kept intentional code as guilty. Reshape flags on our-code surprises stay actionable. Do not restore those comments. A keep survives only when it matches one exception in Comment Sicko's list by name, and a keep claiming a rule of something outside our code also needs proof that the rule holds and binds us. Audit missed scoped lint and TypeScript suppressions. Correctness or safety suppressions stay actionable `MUST KILL`s. Restore deletions only with exact exceptions and scoped proof. Before accepting thin `IMPORTANT` or `do not remove` kills or keeps, trace their symbol or run the **why** skill on it. If a kill is ambiguous, do not restore. If a keep is refuted or still ambiguous, delete it, except a `REVIEW` keep about an outside rule, which stays until someone checks that rule. Report it open. Discard a rejected report by retiring the worktree with `harness review worktree retire --branch no-comments-<branch>`, and relaunch once from step 1 with a fresh worker whose brief names the failure. If you reject the rerun too, leave out only its rejected findings, report them open, and go on with the accepted ones. Apply each file's accepted hunks to this checkout's working tree with plain `git apply`, leaving the index alone. A file whose hunks no longer apply, because it changed during the run, stays out and is reported open. Then retire the worktree.
3. Fix trivial accepted flags directly by deleting a dead path, dropping a parameter, or using the real API. If any fix needs a shape, sketch it once for the accepted set and surrounding code in the **codebase-design** vocabulary. Stop at the sketch. The sketch shapes. Step 4 implements.
4. Implement the smallest root-cause fix in scope. Remove every named workaround. If the root cause is out of scope, land the smallest in-scope fix and report the rest open. The **principle-fix-root-causes** and **principle-redesign-from-first-principles** skills guide intent only. Neither authorizes widening the fence nor fixing instances outside it. Never bolt on symptom guards.
5. Constraint comments say `do not remove`, `do not change wording`, or `talk to X before changing`. Leave keeps about things we cannot change. Offer the cheapest in-scope type, runtime, test, or CI lint. Wait for interactive approval. Unattended runs need the caller's approval in advance. If approved, encode then delete. Otherwise delete, report the constraint open, and sketch out-of-scope work.
6. Run the checks the touched files need, then report the deletion count, restored comments, `REVIEW` keeps, reruns, the design sketch, fixes, encoding offers, encodings, unenforced constraints, and other open work.
