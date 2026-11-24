---
name: no-comments
description: "Sweep a diff's code comments with Comment Sicko, fix the code its kills expose, and encode the constraints comments claim."
argument-hint: "[files or diff; none sweeps the diff against the base branch, working tree included]"
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  opencode/autoinvoke: "false"
---

# No comments

Comment Sicko, a fresh `worker`, kills comments in a sandbox. You judge its report, bring back the accepted kills, and fix the code they expose.

Scope: $ARGUMENTS. With none, the diff against the base branch (default `main`), working tree included.

## Steps

1. **Sandbox.** The key is `no-comments-<branch>`. Retire any sandbox an earlier run left (`harness review worktree retire --branch <key>`), snapshot the scope (`harness review snapshot --branch <key> --base <base> -- <scoped paths>`), add the sandbox (`harness review worktree add --branch <key> --patch <patch> <base>`, without `--patch` when the snapshot writes nothing), and stage everything in it (`git -C <sandbox> add -A`). Done when `git -C <sandbox> diff` is empty.
2. **Launch.** Through **delegation**, launch the `worker` at the `high` tier for independent inspection. Its brief is [`references/comment-sicko.md`](references/comment-sicko.md) verbatim, then the scope, the sandbox path as the one place it edits, and that it leaves the index as it is. When no subagent can launch, run that prompt yourself in the sandbox as a fresh pass and say so. Done when you hold its report and `git -C <sandbox> diff`.
3. **Judge.** Rule on every kill, keep, `MUST KILL`, and `REVIEW` by [Judging](#judging). To reject the report, retire the sandbox and rerun steps 1 and 2 once with a fresh worker whose brief names the failure; when you reject the rerun too, only its rejected findings go open. Done when every finding is accepted, rejected, or open.
4. **Bring back.** Apply each file's accepted hunks to the working tree with plain `git apply`, leaving the index as it is. A file that no longer applies, because it changed during the run, goes open. Retire the sandbox. Done when the sandbox is gone and every accepted hunk is applied or open.
5. **Fix.** Fix each accepted `MUST KILL` at its root, inside the scope: delete the dead path, drop the parameter, call the real API. When a fix needs a new shape, sketch one shape for the whole accepted set in **codebase-design** terms, then build it. Remove every workaround the kills named. A root cause outside the scope gets the smallest in-scope fix and goes open. **principle-fix-root-causes** and **principle-redesign-from-first-principles** set the intent; the scope stays the fence. Done when every `MUST KILL` is fixed or open.
6. **Encode.** A constraint comment says `do not remove`, `do not change wording`, or `talk to X before changing`. For each one about our own code, offer the cheapest in-scope type, runtime check, test, or CI lint, and get approval interactively or from the caller in advance. Approved: encode it, then delete the comment. Otherwise: delete it, report the constraint open, and sketch the out-of-scope work. A keep about something we cannot change stays. Done when every constraint is encoded or open.
7. **Report.** Run the checks the touched files need, then report the deletion count, restored comments, `REVIEW` keeps, reruns, the sketch, fixes, encoding offers and encodings, unenforced constraints, and other open work.

## Judging

Comment Sicko's fresh eyes win unless a rule here overrules them.

- **Reject the report** for application-code edits, edits outside the scope, `MUST KILL` reasons that misstate the code, or flags that call kept intentional code guilty.
- **Kills** stand, ambiguous ones included. Restore a deleted comment only with the exact exception that protects it and scoped proof. A reshape flag on our own surprising code stays actionable, and its comment stays deleted.
- **Keeps** survive by matching one exception in Comment Sicko's list by name. A keep claiming an outside rule (a dependency, platform, vendor, or protocol) also needs proof that the rule holds and binds us. A refuted or still-ambiguous keep is deleted, except a `REVIEW` keep, which stays open until someone checks its rule.
- **Thin claims:** before ruling on an `IMPORTANT` or `do not remove` kill or keep, trace its symbol or run the **why** skill on it.
- **Suppressions:** audit the scoped lint and TypeScript suppressions it missed. One that guards correctness or safety is a `MUST KILL`.
