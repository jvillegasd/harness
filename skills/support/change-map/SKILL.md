---
name: change-map
description: "Trigger: before opening or refreshing a PR, when the human asks what changed or what to review, or to catch them up since a commit or session. Classify every change into review tiers and tell the human exactly what to read."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Map a change for its human reviewer: sort every changed file and hunk into a review tier, say what must be read line by line and what can be skimmed or skipped, back each risky item with the one fact it is safe because of, and surface the decisions the agent made on its own.

## Why

Agents produce more code than a human can read carefully, and an undifferentiated diff spreads attention evenly across a lockfile and an auth check. A map puts the human's attention where a mistake would cost the most, and it makes the agent show its evidence instead of asserting that the change is fine. The deterministic floor keeps the map honest: the agent may raise a tier when the code warrants it, but it cannot talk a risky file down.

## Tiers

| Tier | Meaning | The human |
| --- | --- | --- |
| `critical` | A one-way door or a concrete security, data-loss, compatibility, or concurrency risk: auth and permissions, secrets, migrations and deletion, public interface removal, money, CI and infrastructure, concurrency. | Reads it line by line. |
| `behavioral` | Observable behavior changes: outputs, UI, CLI flags, defaults, error handling, new interface surface, agent instructions. | Reads the intent and the tests, and spot-checks the code. |
| `structural` | Claims to preserve behavior: refactors, moves, renames with logic, performance work. | Skims it; trusts it only when existing tests are unchanged and pass. |
| `tests-docs` | Supporting material: new or changed tests, documentation. | Skims tests for assertions that cannot fail. |
| `mechanical` | No semantic content: lockfiles, generated code, formatting. | Skips it once the deterministic check that classified it is named. |

## Steps

1. Pick the mode. Diff mode (the default) maps the branch against its base. Since mode (`since <ref>`, or since the start of this session) maps what changed after that point, for catching the human up.
2. Get the floor. Run `harness change-map --json` (with `--base <ref>` when the base is not the default branch). It lists changed, staged, unstaged, untracked, and deleted files with a floor tier from the repository's `criticalPaths` in `docs/agents/harness.json` and built-in rules. If the command is unavailable, apply the same rules by hand and say so. A file only becomes `mechanical` through those deterministic rules; a locked third-party skill body arrives as one `mechanical` entry with its file count and source, and the new or changed entry in `skills-lock.json` is what to read instead.
3. Read every hunk and raise tiers where the code warrants it: a `structural` file whose change alters observable behavior becomes `behavioral`; anything matching the `critical` meaning becomes `critical`. Never lower a floor tier. When unsure between two tiers, choose the higher one and say why.
   When a critical change reveals a recurring sensitive area not covered by `criticalPaths` or a built-in critical rule, ask the human whether to add a focused path or glob to `docs/agents/harness.json` for future maps. Name the proposed pattern and why it merits a permanent critical floor. Classify the current change as critical regardless of the answer; change the configuration only if the human approves.
4. For each `critical` and `behavioral` item, name the one fact the change is safe because of, and how far it is proven on the certainty ladder in `verification` (said, cited, walked, ran, reproduced). A fact at "said" or "cited" is unproven; say so instead of implying safety. When the project has a feature map (`verify-<app>/features/README.md`, see `e2e-verification`), name the affected feature for each `behavioral` and `critical` item so `verification` can drive it live (ladder step 5).
5. Call the merge danger: a one-way door (hard to reverse: data migration, deletion, published interface, external message) or a two-way door, plus the blast radius in a few words.
6. List the decisions the agent made that the human did not ask for (a default, a naming choice, a dependency, a skipped case), each with its reason. Take them from the task document's `## Decisions` table (see `continuity`) when there is one, from the commits, and from the session. In since mode, also list the open decisions only the human can make, each with a recommendation.
7. When a critical change involves concurrency, retries, leases, or a lifecycle state machine, suggest `/statechart`, which model-checks the rules with TLA+ and draws the states in the reader's words (its toolchain installs through `mise` on first use).

## Output

Lead with the smallest view that makes the change clear: a few lines of pseudocode, a call tree, or a small Mermaid diagram. Then:

```
Tiers: critical N, behavioral N, structural N, tests-docs N, mechanical N

Must read
- path:Lstart-Lend (critical): why it matters. Behavior: one line. Safe because: <fact> (<ladder step>).
- path:Lstart-Lend (behavioral): ...

Skim: path, path (structural, tests unchanged and passing)
Skip: path (mechanical: lockfile)

Merge danger: one-way | two-way door; blast radius: <few words>
Decisions you did not ask for: <decision>: <reason>
Open decisions (since mode): <decision>: <recommendation>
```

In a pull request body, this map replaces a free-form summary and takes the review-guide form in `open-pr`'s [PR body template](../../workflow/open-pr/references/pr-body.md), which lists every file with its tier. In chat, keep the same order and keep it short.
