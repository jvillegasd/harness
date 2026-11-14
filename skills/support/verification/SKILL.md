---
name: verification
description: "Trigger: verify changes or coordinate checks. Run proportionate checks and report revision-bound evidence."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Choose and coordinate the checks for a change, and report evidence tied to the exact revision it covers. Ordinary checks do not need this skill.

## Why

A pass is only worth what it covers. Binding each result to a revision, and naming what was run against what, lets the next reader trust it or know to rerun it.

## Steps

1. Compare the requested behavior with the actual diff, including owned untracked files. Identify the meaningful failure behavior and the affected seams.
2. Resolve commands from project instructions, task runners, or the toolchain. Run cheap targeted checks first, then the repository-required ones. Use existing tests, and add regression coverage where it can fail for the right reason. TDD applies only when the task or project asks for it.
3. Bind results to the commit. Commit owned work first where the floor lets you; only work that stays uncommitted needs a content identity covering tracked and untracked inputs. Record each command with its outcome: pass, fail, skipped, unavailable, or stale. A later edit makes the affected evidence stale, and HEAD equality alone does not identify a dirty tree.
4. Fix owned failures and rerun the affected checks. When failures repeat without progress, stop and report as the floor's Working rules describe. For authorized PR delivery, also run the forge's required checks; otherwise stop after the local ones.

A fixture, a static read-back, or a self-review is not live acceptance or independent inspection, so report it as what it is.

For persistent frontend, backend, API, or logical journey coverage, use `journey-verification` to create and run tests with the minimal required services and reusable fixtures or provider fakes.

## Certainty ladder

When a claim carries the safety of a change, say how far it is proven. Take each fact as far down this ladder as is cheap, and report where it stopped:

1. Said: someone asserted it. Worth nothing on its own.
2. Cited: a real `path:line`, or the library's own source, shows it.
3. Walked: the failure path was traced step by step and does not reach.
4. Ran: a script or test calls the real code and fails loudly if the fact is wrong.
5. Reproduced: it was observed in the running application.

Only steps 4 and 5 count as verified. A small script that imports the shipped library and calls the exact function in question usually reaches step 4 cheaply. The verdict below reaches step 5.

## Verdict

Before a pull request opens or its head changes, and at `build`'s integration, get an independent run-as-user verdict when `change-map` lists `critical` or `behavioral` items and the project has a `verify-<app>` skill from `/create-verification-skill`. Without one, report the verdict as unavailable and name `/create-verification-skill`. The author's own drive of the app is a check, not a verdict.

1. **Scope.** Pick the files under `verify-<app>/features/` that the `critical` and `behavioral` items touch. Each such item that no feature covers is a gap: name it, and suggest the feature map update `/maintain-verification-skill` would make.
2. **Revisions.** A feature change is driven at the head. A bugfix is driven at the base and then the head: its reproduction must fail on the base for the reported reason and pass on the head.
3. **Runner.** One fresh `executor` that never saw the implementation, launched through `delegation`, so `harness models resolve executor` picks its client, model, and effort. When the executor cannot launch here (another client outside T3 Code), launch a fresh native subagent instead and say so. Give the runner a checkout of its own, never the parent's: `isolation: "worktree"` through T3 Code or a native tool that takes it, otherwise one detached worktree from `harness review worktree add <revision>`, retired after the run with `harness review worktree retire`. A bugfix's base run happens in that worktree too. The brief's goal is the verdict, its scope is the selected features, and its report below replaces the executor role's output fields. Its brief carries the `verify-<app>` skill path, the selected feature files, the revisions, the requirements quoted from their source, and the report format below. It withholds the conversation, the rationale, the diff's explanation, and review findings. The runner follows the skill's Launch, Doctor, Drive, Evidence, and Cleanup sections, edits no tracked file, and fixes nothing.
4. **Report.** The runner returns:
   ```
   verdict: PASS | PASS+NOTES | FAIL
   revision: <head sha> (base <sha> for a bugfix)
   <feature>: pass | fail | could-not-drive: <what was driven, the end state observed, the evidence path>
   gaps: <unmapped items> | none
   ```
   FAIL means a driven feature missed its proving end state, or a bugfix reproduction did not fail on the base and pass on the head. PASS+NOTES means every driven feature passed and something stayed open: a gap, or a feature that could not be driven, each named. PASS means every selected feature passed with no gaps.
5. **Use.** The verdict is bound to its head. A later commit makes it stale unless it changes only tests or documentation. Fix a FAIL and run a fresh verdict; never overrule one. Report it as one line: `Verdict: <verdict> at <sha>`, with the gaps.

## Output

What passed, its scope and revision, the verdict line when one ran, and the remaining failures or unverified behavior. Keep the evidence in an existing task document only when `continuity` needs it.
