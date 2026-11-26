# Author a local skill

`harness skills new` scaffolds a guarded local skill with its two links, `harness skills move` regroups it, and `harness skills verify` checks every locked body against its hash and every local skill for its invocation guards.

## Sub-features

- `skills-new` writes `skills/<group>/<name>/SKILL.md` with all three guards, `agents/openai.yaml`, and the `.agents/skills` and `.claude/skills` links.
- `skills-move` moves the body with `git mv` and repoints the link.
- `skills-verify` passes on a clean checkout and fails, naming the skill and key, when a local skill loses a guard.

## How to get to it (user POV)

From the repository root: `harness skills new <name> <group>`, `harness skills move <name> <group>`, `harness skills verify`.

## Driving it with verify-harness

Preconditions:

- Fresh sandbox. These commands act on the checkout, not on `HOME`, so run each in a subshell inside the sandbox checkout: `C() { ( cd "$SANDBOX/checkout" && "$@" ); }`, then `C "$SANDBOX/harness" skills verify`. Paths below are relative to `$SANDBOX/checkout`.

- **Clean verify.** Run `C "$SANDBOX/harness" skills verify`. Exit code `0`.
- **New skill.** Run `C "$SANDBOX/harness" skills new demo-verify planning`. Exit code `0`. `skills/planning/demo-verify/SKILL.md` holds `disable-model-invocation: true` and `opencode/autoinvoke: "false"`, `skills/planning/demo-verify/agents/openai.yaml` holds `allow_implicit_invocation: false`, and `readlink .claude/skills/demo-verify` prints `../../.agents/skills/demo-verify`.
- **Verify accepts it.** Run `C "$SANDBOX/harness" skills verify`. Exit code `0`.
- **Move.** Run `C "$SANDBOX/harness" skills move demo-verify writing`. Exit code `0`, and `readlink .agents/skills/demo-verify` prints `../../skills/writing/demo-verify`.
- **Verify catches a lost guard.** Delete the `disable-model-invocation: true` line from `skills/writing/demo-verify/SKILL.md`, then run `C "$SANDBOX/harness" skills verify`. Exit code is not `0`, and the output names `demo-verify: missing disable-model-invocation`.
- **Old flag is gone.** Run `C "$SANDBOX/harness" skills new other planning --guarded`. Exit code is not `0`, and the output names `unknown flag --guarded`.
- **Proof.** Save each command's output and exit code, and the scaffolded `SKILL.md`, to `$EVIDENCE/skill-authoring/`.

## Gotchas

- `skills move` stages its rename with `git mv` in the sandbox worktree. Cleanup discards it with the worktree; never run this recipe in the real checkout.
- A local skill outside the allowlist (`open-pr`, `address-review`) must carry all three guards, so a hand-written skill without them fails `skills verify` by design.
