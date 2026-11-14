# Splitting into a stack

Prefer several narrow pull requests to one large one: a reviewer reads a narrow one in full, and each lands or reverts on its own. The delivery request covers every pull request in the stack, so the split needs no second approval.

## Decide

Order the commits so they tell the story (`principle-sequence-verifiable-units`), then read them as candidate pull requests. Split where a run of commits:

- passes the repository's required checks on its own,
- makes sense to a reviewer without the commits above it, and
- would be worth landing even if the commits above it were dropped.

Keep one pull request when no such cut exists, or when the cuts would leave pieces too small to review on their own. Independent work, which builds on nothing else in the change, branches from the default branch as its own pull request rather than joining the stack.

## Build

The root pull request targets the default branch. Each child branch starts from its parent's exact tip, and its pull request targets the parent branch: `gh pr create --base <parent-branch>`.

Run `open-pr`'s steps for each pull request from the root up, each with its own `change-map` and its own `Reviewed:` line, since each is reviewed and merged on its own.

## After opening

Take one status pass over the stack with `harness pr status --stack <tip>`, which reports every pull request under the tip bottom-up, and write a readiness report for each. Driving the stack green is `address-review`'s `drive` mode, run when asked and after the whole stack exists, because a fix low in the stack restarts the checks above it.

Never rebase, retarget, or force-push a pushed branch in the stack. A fix goes on the branch that owns the code. When a parent merges, `/land` repairs its children.
