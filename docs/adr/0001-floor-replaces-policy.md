# A short floor replaces the policy, and rigor is opt-in through /work

The harness used to install a 100-line `POLICY.md` into every client: hard rules, the default process, delegation mechanics, and the principles index, much of it restated in the skills and drifting from them. We replaced it with `FLOOR.md`, which holds only what must apply on every turn in every repository (authority, preservation, secrets, untrusted text, prose), and moved each procedure to the one skill that owns it. Rigor became opt-in: the principles index and the playbooks live in `/work`, which only the human starts, so a session without `/work` gets the floor and the skills it reaches on its own, and nothing points it at the principles.

## Considered options

- **No always-loaded file**, relying on client permission rules: rejected, because rules such as untrusted text and preserving unrelated changes rest on judgment that no pattern can enforce.
- **Keep POLICY and add `/work` beside it**: rejected, because two always-on homes for the same rules is the drift that prompted the change.

## Consequences

The floor drops POLICY's "Enforced by" column, because the command and secret rules are now enforced by each client's own permission config (Claude Code's settings, Codex's rules file, OpenCode's `opencode.json`), not stated in prose. The install component keeps its `policy` key so existing install records still match.
