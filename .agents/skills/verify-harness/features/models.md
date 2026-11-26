# Resolve role models

`harness models` assigns each tier a client and model and each role a tier, and `harness models role resolve` prints the client, model, and effort a role launches on. Every subagent launch depends on this answer.

## Sub-features

- `role-resolve` prints a role's launch target as JSON.
- `role-set` overrides one role, and `role-reset` drops the override.
- `tier-preset` switches which client serves each tier.

## How to get to it (user POV)

- Run `harness models role resolve review-bugs --json`.
- Run `harness models role set review-bugs --effort high`, then `harness models role reset review-bugs`.
- Run `harness models tier preset claude`.

## Driving it with verify-harness

Preconditions:

- Fresh sandbox. The scratch `HOME` has no models file, so every answer starts from the defaults.

- **Default.** Run `H models role resolve review-bugs --json`. Exit code `0`, and the JSON has `role`, `tier`, `client`, `model`, `effort`, and `source` fields with a non-empty `client` and `model`.
- **Override.** Run `H models role set review-bugs --effort high`, then resolve again. `effort` is `high`, and `source` names the override.
- **Reset.** Run `H models role reset review-bugs`, then resolve again. The JSON matches the default step.
- **Preset.** Run `H models tier preset claude`, then `H models role resolve executor --json`. `client` is `claude`.
- **Isolation.** `ls "$SANDBOX/home/.harness"` shows the models file the commands wrote. Every command ran through `H`, whose `HOME` is the sandbox's, so nothing else needs checking; never read or list the real `~/.harness`.
- **Proof.** Save every resolve JSON, in order, to `$EVIDENCE/models/`.

## Gotchas

- Which model a tier picks depends on the client CLIs on `PATH` (`codex debug models`, for example), so `model` may differ between machines. Assert on fields and on changes between steps, not on a fixed model name.
- A tier this machine cannot serve prints a warning and keeps the client default. That is a real result, not a failure of the recipe.
