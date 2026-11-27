# Install and uninstall

Install copies the skills, the floor, and the tracked settings into each client's folder under `HOME`, and uninstall removes only what install owns, keeping the user's own keys.

## Sub-features

- `install-skills` copies every skill, with guarded skills keeping `disable-model-invocation: true`.
- `install-floor` writes `FLOOR.md` as `~/.claude/CLAUDE.md`.
- `install-settings` merges `clients/claude.json` into `~/.claude/settings.json`, including `skillOverrides`, without overwriting a value the user set.
- `uninstall` removes the tracked settings and copies and keeps user keys.

## How to get to it (user POV)

- Run `harness install skills floor settings --client claude`.
- Run `harness uninstall skills floor settings --client claude --yes`.

## Driving it with verify-harness

Preconditions:

- Fresh sandbox. Seed a user preference first: `mkdir -p "$SANDBOX/home/.claude" && printf '{"theme":"light","skillOverrides":{"mine":"off","tdd":"on"}}\n' > "$SANDBOX/home/.claude/settings.json"`.

- **Install.** Run `H install skills floor settings --client claude`. Exit code `0`, and the output contains `Harness AI installed`.
- **Skills copied and guarded.** `head -8 "$SANDBOX/home/.claude/skills/review/SKILL.md"` shows `disable-model-invocation: true`, and `head -8 "$SANDBOX/home/.claude/skills/open-pr/SKILL.md"` does not.
- **Floor written.** `cmp "$SANDBOX/home/.claude/CLAUDE.md" "$SANDBOX/checkout/FLOOR.md"` exits `0`.
- **Settings merged.** `jq .skillOverrides "$SANDBOX/home/.claude/settings.json"` keeps `"mine": "off"` and `"tdd": "on"` and adds `"grilling": "name-only"` and `"modern-web-guidance": "user-invocable-only"`.
- **Reinstall is quiet.** Run the install again. Exit code `0`, and `settings.json` is byte-identical to the first run's copy.
- **Uninstall.** Run `H uninstall skills floor settings --client claude --yes`. Exit code `0`. `jq . "$SANDBOX/home/.claude/settings.json"` holds the seeded `skillOverrides` entries (`mine`, `tdd`) and no harness entry. `test ! -e "$SANDBOX/home/.claude/CLAUDE.md"` passes.
- **Proof.** Copy `settings.json` after install and after uninstall, and the head of both `SKILL.md` files, into `$EVIDENCE/install-uninstall/`.

## Gotchas

- Uninstall leaves an empty `~/.claude/skills/` directory. That is expected, not leftover state.
- Settings reconciliation also rewrites `theme`, `model`, and `effortLevel` from `clients/claude.json`, so a seeded `theme` changes. Assert only on keys the harness does not track, and on `skillOverrides` entries.
- The `skills` component validates the skills with the real `claude` CLI under a throwaway `CLAUDE_CONFIG_DIR`. Claude Code runs one-time migrations on startup, and one clears `skipAutoPermissionPrompt` when `permissions.defaultMode` is not `auto`. If "Reinstall is quiet" fails on that key, the installer let Claude Code touch the sandbox's `~/.claude` again: report it as a failure.
- Install archives replaced files under `$SANDBOX/home/.dotfiles-backups`. Check there before reporting a lost file.
