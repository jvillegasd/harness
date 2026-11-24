---
name: principle-launch-on-the-assigned-model
description: "Apply before launching a harness role (executor, code-search, worker, reviewer, a review lens), which every subagent a skill does not ship itself runs as. Resolve its assigned client, model, and effort and launch there; a launch with no model silently inherits the session's."
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Launch on the Assigned Model

**Why:** A subagent launched with no model runs on the session's model, and nothing fails to say so. A review meant for another model family loses its independence, and bulk work meant for a cheap tier runs on the expensive one.

**Pattern:**
- **Resolve before every launch.** Run `harness models role resolve <role> --json`, adding `--tier <tier>` for the `worker` and `--client <client>` when the brief needs a model family, and take the client, model, and effort it returns. A skill that says to launch a role through `delegation` means this step first. A skill that ships its own named agents, such as `impeccable`'s reviewers, launches them as it describes; they are not roles and the resolver rejects their names.
- **Launch where it resolves.** Launch the installed agent natively only when the resolved client is this session's; otherwise use T3 Code's `delegate_task` with that provider, model, and effort, as `delegation`'s Launch steps describe.
- **Report where it ran.** Name each launch's role, client, model, effort, and route (native or `delegate_task`), so a launch that missed its assignment shows.
- **Report a role that cannot launch where it resolves.** Work inline where independence is not required; the session's own model is never a silent stand-in.
