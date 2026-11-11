# Plan

**You own the plan, not the code.** The plan is the deliverable: a checklist an owner runs box by box and the human audits from the evidence. Nothing is implemented here.

1. When the change is one or two files with an obvious approach, skip the plan: say so and stop.
2. Settle open questions by experiment before writing, with `proto` for each. Keep each prototype's branch, commit, and evidence for the plan. Ask the human only about a product or preference call no run can settle, with the options and your recommendation (`principle-never-block-on-the-human`).
3. Explore the code with `code-search` launched through `delegation` (`principle-guard-the-context-window`): file pointers, conventions, test commands, and entry points, not pasted contents.
4. Write the plan into the task document (`continuity`), one section per pull request in landing order (`principle-sequence-verifiable-units`). Each section names what it depends on, the files it touches, the change, what you will see when it works, and how it is verified: the tests, a run on the real surface, and a before and after number when performance is at stake. After the sections, record what the prototypes proved and what stays unproven, the alternatives rejected (the `## Decisions` table), and the risks with the pull request each lands in.
5. Name how the plan runs: interactive `/work` on the large playbook, or `/work` autopilot when the human will step away. When the human wants tickets and an unattended worker build, say that `/spec` turns the plan into a durable spec for `/build`.
6. Read `unslop` from its path and apply it to the plan.
7. Commit the task document on the task branch: reuse the existing one, or create one as `implement` names it, since a plan commit on the default branch needs an explicit request. Then hand back. Under autopilot, the written plan is the definition of done, so the run ends here without building.
8. Execution starts only on the human's explicit go. On go, run the large playbook on this plan, under an autopilot grant when the human steps away, without matching the plan row again, and update the task document's `Playbook:` line.

**Reply:** the plan path, the pull requests in order with their dependencies, what the prototypes proved and what stays unproven, and how the plan runs.
