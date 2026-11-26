# Harness verification map

This directory is the maintained source for verifying the user-facing behavior of the `harness` CLI. Read this index before driving, then use the matching feature file as the recipe.

## Baseline preconditions

- A sandbox from `scripts/sandbox up`, with `SANDBOX`, `EVIDENCE`, and `REVISION` exported and `sandbox doctor "$SANDBOX"` reporting `ok`.
- The `H` prefix from the skill's Launch section, so every command uses the scratch `HOME`, `TMPDIR`, and `HARNESS_ASSETS`.
- `git` and `go` on `PATH`. `gh` only for the recipes that say so.

## Driving conventions

- Start each recipe from a fresh sandbox unless its preconditions say otherwise.
- Pass `--client claude` and name components explicitly, so no run reaches Homebrew or the network.
- Treat each command as literal.
- Assert on files on disk and on `--json` output, not on the tree-style human output, which may change wording.

## Features

| Feature | File | Touch it when the change affects |
| --- | --- | --- |
| Install and uninstall | [install-uninstall.md](install-uninstall.md) | `internal/install`, `clients/`, `FLOOR.md`, skill frontmatter, `skills-lock.json` |
| Author a local skill | [skill-authoring.md](skill-authoring.md) | `internal/skills`, `harness skills` commands, skill guards |
| Resolve role models | [models.md](models.md) | `internal/models`, `internal/agents`, `roles/` |
| Map a change | [change-map.md](change-map.md) | `internal/changemap`, `docs/agents/harness.json` |
