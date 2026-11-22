# Project

**You own the roadmap.** Several features toward one product goal ("build an app that does X, Y, and Z") run as milestones on one integration branch. Each milestone is an epic built by `build`; the agent lands its own reviewed pull requests on the integration branch, and the human merges that branch once.

1. **Frame.** State the product goal and the definition of done as pass/fail checks on the running product, one per capability the human named. In an existing repository, ground the scope with `code-search` through `delegation`. In a new one, choose the stack and layout as `agent` decisions, settling a fork a run can answer with `proto`.
2. **Write the roadmap** into the task document (`checkpoint`): milestones in order, each a demoable increment that leaves the product working, with its own done check. The first milestone is the scaffold and the riskiest unknown: the product builds, its tests run, CI runs them, and the app can be driven the way `verification` will drive it (`principle-foundational-thinking`). Write each milestone's epic when you reach it, not up front, since what earlier milestones teach changes later ones.
3. **Hold the one gate.** Show the human the roadmap and wait for their go, because a project spends hours of model budget on a direction only they can confirm. Under autopilot, state it and start.
4. **Open the integration branch**: `project/<slug>` from the default branch, pushed to origin. Every milestone's pull requests target it, as their root or through a stack.
5. **Run each milestone.**
   - Write its epic with `epic`'s steps, then build it with `build`'s steps, using the integration branch as the base where `build` names the default branch.
   - Land each pull request on the integration branch once `harness land check --into project/<slug> <pr>` puts it on the `open` lane with no holds: its review verdict covers its head and its required checks are green. Merge it with `harness land merge <pr> --into project/<slug> --head <sha> --body-file <file>`, then clean up with `harness land finish <pr> --into project/<slug>`, from the root of a stack up. A hold is fixed with `address-review`, never accepted on the human's behalf.
   - Verify the milestone's done check on the integration branch's head in the running product (`verification`), and add a `## Decisions` row. When the milestone changed what the roadmap assumed, rewrite the rest of the roadmap before the next one.
6. **Deliver the project.** Check the whole definition of done on the integration branch. Open its pull request into the default branch with `open-pr`, the roadmap and every `agent` decision in the body, so the human reads the project in one place. The human merges it, or asks `/land` to.

Milestone tickets follow the tracker `docs/agents/harness.json` names, published only on the human's request, as `epic` describes.

**Reply:** the goal, the `agent` decisions first, the roadmap with each milestone's state and its pull requests, the integration pull request with its readiness report, and what is still open.
