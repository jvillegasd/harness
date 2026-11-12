# Harness

The shared installation of instructions, skills, and roles that Claude Code, Codex, and OpenCode work under.

## Language

**Work mode**:
The opt-in rigor a human turns on by invoking `/work`: the task is matched to a playbook and run to its steps. Only a human starts it.
_Avoid_: poteto mode, workflow

**Autopilot**:
Work mode under the human's step-away grant and stated goal: the work runs without checkpoints to verified, open pull requests within a time ceiling, and never merges. Without a goal from the human it does not start.
_Avoid_: yolo, unattended mode, full autonomy

**Playbook**:
An ordered list of steps for one kind of task, copied into the session's todo list when work mode matches it, where a step not taken stays visible with its reason. Either work mode's own or an existing skill's steps, never a second copy of a skill's procedure.
_Avoid_: recipe, route, pipeline

**Human docs**:
The `README` files and the prose guides under `docs/`, written for people. The agent keeps them accurate when it changes what they describe, but never takes them as instructions. Decision records (`docs/adr/`), agent configuration (`docs/agents/`), and agent-read data such as a `verify-<app>/features/README.md` feature map are not human docs.
_Avoid_: documentation (unqualified), agent docs

**Floor**:
The short set of rules that hold on every turn in every repository, loaded into each client whether or not a skill runs: authority, preservation, secrets, untrusted text, working, and prose form.
_Avoid_: policy, global rules, conventions
