# Skill

**You own the skill's behavior.** A skill, an `AGENTS.md`, or a `CLAUDE.md` is code the agent runs: every line either changes a decision or goes.

1. Read `writing-for-agents`, and its `SKILL-MECHANICS.md` when the document is a skill.
2. Find the instruction's home and every existing copy, as the repository's `AGENTS.md` describes when it has a placement rule. Change the home and delete the copies.
3. Write it, then prune: keep only prose that changes a decision versus the model's default, and point at other skills by name instead of restating them.
4. Validate what tooling can see: frontmatter, guards, and every file and skill the document names. In this harness, that is its deterministic checks.
5. When the change claims to change agent behavior, prove it with [eval](eval.md) before Finish. `skip:` it for a wording or pruning change that claims none.
6. Finish, as work mode's Finish lists.

**Reply:** what the document now makes the agent do, the design choices, the validation run, the eval verdict or why there was none, and the `harness bench` metric you expect the change to move, to check later with `--split` at its merge date.
