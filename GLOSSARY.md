# Harness

The shared installation of instructions, skills, and roles that Claude Code, Codex, and OpenCode work under.

## Language

**jvillegasd-mode**:
The opt-in rigor a human turns on by invoking `/jvillegasd-mode`: the task is matched to a playbook and run to its steps. Only a human starts it.
_Avoid_: work mode, poteto mode, workflow

**Autopilot**:
jvillegasd-mode under the human's step-away grant and stated goal: the work runs without checkpoints or a time limit to verified, open pull requests, stacked for the human to land, and merges them itself only when the request says full. Without a goal from the human it does not start.
_Avoid_: yolo, unattended mode, full autonomy

**Playbook**:
An ordered list of steps for one kind of task, copied into the session's todo list when jvillegasd-mode matches it, where a step not taken stays visible with its reason. Either jvillegasd-mode's own or an existing skill's steps, never a second copy of a skill's procedure.
_Avoid_: recipe, route, pipeline

**Entry point**:
A skill the human starts: `jvillegasd-mode` and the skills no other skill runs. Every other skill is a **helper** that a skill or playbook runs as a step; helpers stay callable by name too. Skill groups sort skills by purpose, not by who starts them.
_Avoid_: public skill, top-level skill

**Epic**:
One big ticket that records a change's settled behavior, acceptance criteria, test seams, decisions, and risks, cut into slices. It lives in `docs/agents/epics/` and outlives the build, whose task document points at it.
_Avoid_: spec, plan document

**Slice**:
One vertical ticket of an epic: a narrow path through every layer, finished as one pull request, with acceptance checks that are red on the base commit. Drafted under `docs/agents/tickets/<feature>/`.
_Avoid_: story, subtask, tracer-bullet ticket

**Human docs**:
The `README` files and the prose guides under `docs/`, written for people. The agent keeps them accurate when it changes what they describe, but never takes them as instructions. Decision records (`docs/adr/`), agent configuration (`docs/agents/`), and agent-read data such as a `verify-<app>/features/README.md` feature map are not human docs.
_Avoid_: documentation (unqualified), agent docs

**Floor**:
The short set of rules that hold on every turn in every repository, loaded into each client whether or not a skill runs: authority, preservation, secrets, untrusted text, working, and prose form.
_Avoid_: policy, global rules, conventions

**Component**:
A named part of the installation that install, update, and uninstall select by the same name: `packages`, `mcps`, `skills`, `agents`, `floor`, `settings`, or `aliases`.
_Avoid_: group, piece

**Tool**:
A program the installation puts on the machine: a client program, node, gh, uv, Engram, or agent-browser. A tool already on the machine before the harness installed it is never the harness's to remove.
_Avoid_: package (unqualified), binary, dependency

**Managed config**:
What the harness writes into a client and owns, as opposed to the user's own config: copied skill, agent, and floor files, merged settings and hooks, MCP registrations, and the alias block. A component is a tool, managed config, or both.
_Avoid_: managed copy, managed files, managed settings, harness config
