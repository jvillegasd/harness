# Map a change

`harness change-map` lists every changed file with a floor review tier from built-in rules and `criticalPaths` in `docs/agents/harness.json`. `change-map`, `review`, and `land check` all build on it.

## Sub-features

- `tiers` classifies staged, unstaged, untracked, and deleted files against a base.
- `critical-paths` raises a file matching `criticalPaths` to `critical`.
- `json` prints one object for scripts.

## How to get to it (user POV)

From the repository root: `harness change-map`, or `harness change-map --base <ref> --json`.

## Driving it with verify-harness

Preconditions:

- Fresh sandbox. The worktree starts clean at `$REVISION`. Run each command in a subshell inside it: `C() { ( cd "$SANDBOX/checkout" && "$@" ); }`. Paths below are relative to `$SANDBOX/checkout`.

- **Empty diff.** Run `C "$SANDBOX/harness" change-map --base HEAD --json`. Exit code `0`, and no files are listed.
- **Mixed change.** Append a line to `README.md`, to `FLOOR.md`, and to `internal/ui/ui.go`, and add an untracked `docs/verify-note.md`. Run `C "$SANDBOX/harness" change-map --base HEAD --json`. `FLOOR.md` is `critical` (a configured critical path), `README.md` and `docs/verify-note.md` are `tests-docs`, and `internal/ui/ui.go` is at least `structural`.
- **Proof.** Save the JSON from both runs to `$EVIDENCE/change-map/`.

## Gotchas

- Without `--base`, the base is `origin/HEAD` or `main`, so a sandbox at a branch head reports the whole branch. Pass `--base HEAD` for a recipe-sized diff.
- `internal/ui/ui.go` stands in for any Go file with no critical rule. If that file moves, pick another file under `internal/` that `criticalPaths` does not list.
