# 16 Probe and write a repository's tracker setup

## What to build

`harness setup probe` prints, as JSON, the evidence the tracker mode is chosen from: whether the repository has a GitHub remote, its default branch and whether that branch is protected, the mode an existing tracker configuration already selects, and any existing local spec tree. It changes nothing.

`harness setup tracker --mode <github|local-markdown> [--spec-home <dir>]` writes the choice. It creates or updates the tracker configuration in its stable shape and keeps everything already there: mapped label values, `criticalPaths`, and keys it does not know. In GitHub mode it verifies forge credentials first, creates or updates the four mapped labels, and installs the lifecycle workflow and its reconciler, removing the superseded script when present. In local-markdown mode it creates no labels and installs no workflow. It prints every path it created or updated.

`setup-harness` keeps choosing the mode, asking the human when the evidence conflicts, and inferring `criticalPaths`; it calls these commands for the evidence and the writes.

## Acceptance criteria

- `probe` reports each fact above for a repository with and without a GitHub remote, and writes nothing.
- `tracker --mode github` on a repository with no configuration writes the stable shape, calls the forge once per mapped label, and installs the workflow and reconciler byte-identical to the harness's copies.
- An existing configuration with custom label values, `criticalPaths`, and an unknown key keeps all three.
- `--mode local-markdown` sets the two mode values, defaults the spec home to `docs/specs`, and writes no workflow and no label.
- Missing forge credentials in GitHub mode are refused before any file is written.
- A superseded reconciler script is removed; unrelated workflows are untouched.
- A second run with the same arguments changes no file and reports nothing created or updated.
- `setup-harness` names the commands and no longer spells out the file shape, the label calls, or the copies.

## Seam

Proposed, awaiting the human's confirmation: a small package's tests over throwaway repositories and a fake forge settle the probe and the writes; one command test in `cmd/harness` proves the wiring. The existing byte-identical check between the harness's reconciler and the deployed one stays as it is.

## Blocked by

none

## Out of scope

Choosing the mode. Inferring `criticalPaths`. Trackers other than GitHub and local markdown.
