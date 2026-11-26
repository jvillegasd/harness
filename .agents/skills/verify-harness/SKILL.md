---
name: verify-harness
description: "Drive the harness CLI the way a user does, in a disposable HOME and checkout: install and uninstall into clients, author local skills, resolve role models, and map a change. Use to prove a harness change works before a run-as-user verdict."
disable-model-invocation: true
metadata:
  opencode/autoinvoke: "false"
---

# Verify the harness

The harness is a Go CLI, `harness`. A user runs it from a checkout (`go run ./cmd/harness`) or as the installed binary (`~/.local/bin/harness`), and it writes into `$HOME` (client folders such as `~/.claude`, `~/.codex`, `~/.config/opencode`, plus `~/.harness` and `~/.dotfiles-backups`). Every drive here runs a binary built from the revision under test, against a scratch `HOME` and a detached worktree of that revision, so the user's real home and checkout are never touched.

## Launch

From anywhere inside this repository:

```sh
.agents/skills/verify-harness/scripts/sandbox up
```

It prints three lines. Export them for the rest of the run:

- `SANDBOX`: the run's directory. It holds `harness` (the built binary), `checkout/` (a detached worktree at the revision), `home/` (the scratch `HOME`), and `tmp/`.
- `EVIDENCE`: where proof goes. It sits outside `SANDBOX` and survives cleanup.
- `REVISION`: the full commit the binary was built from. Every verdict names it.

Ready means `sandbox doctor "$SANDBOX"` prints `ok sandbox`. Run every command through this prefix, so it cannot reach the real home:

```sh
H() { env HOME="$SANDBOX/home" TMPDIR="$SANDBOX/tmp" HARNESS_ASSETS="$SANDBOX/checkout" "$SANDBOX/harness" "$@"; }
```

To verify a revision other than `HEAD`, check it out first, or run `git -C "$SANDBOX/checkout" checkout --detach <rev>` and rebuild with `(cd "$SANDBOX/checkout" && go build -o "$SANDBOX/harness" ./cmd/harness)`, then refresh both records: `export REVISION=$(git -C "$SANDBOX/checkout" rev-parse HEAD) && echo "$REVISION" > "$SANDBOX/revision"`.

## Doctor

```sh
.agents/skills/verify-harness/scripts/sandbox doctor "$SANDBOX"
```

It confirms that this script created the directory, that the binary exists, that the checkout still sits at the built revision, and that the binary runs. Run it first whenever anything looks off. Never drive a `HOME` the script did not create.

## Drive

Pick the features the change touches from [features/README.md](features/README.md) and follow each file's recipe with `H`. Name only components and clients the recipe names: a bare `H install` also runs `clients`, `packages`, and `mcps`, which use Homebrew and the network for any missing tool.

## Evidence

For each recipe, save to `$EVIDENCE/<feature>/`:

- the exact command line and its stdout, stderr, and exit code (`H … > out.txt 2> err.txt; echo $? > exit.txt`);
- the resulting files the recipe names, copied out of `$SANDBOX/home` or `$SANDBOX/checkout` before cleanup;
- a `revision.txt` holding `$REVISION`.

Proof standards: drive the real command a user types, never an internal Go function or a test helper. Capture both the action and the resulting state on disk; a success message alone proves nothing. Check side effects in the files the command owns (`settings.json`, `CLAUDE.md`, installed `SKILL.md` frontmatter, `~/.harness/state.json`), and check that keys the recipe seeded for the user survive.

## Cleanup

```sh
.agents/skills/verify-harness/scripts/sandbox down "$SANDBOX"
```

It removes the worktree it created and the sandbox directory, and refuses any directory it did not create. Evidence under `$EVIDENCE` stays. Run cleanup after every run, failed ones included.

## Helpers

`scripts/sandbox up | doctor <dir> | down <dir>` is the only helper. It needs `git` and `go` on `PATH`. `up` rolls back its worktree and directory when any step fails, and gives each run its own evidence folder. Commands a recipe runs inside `$SANDBOX/checkout` go in a subshell, `( cd "$SANDBOX/checkout" && … )`, so your shell never sits in the worktree that cleanup removes.
