# Command-line reference

This page covers `harness` command behavior that does not fit in `--help`. For the flags themselves, run `harness <command> --help`.

## Components

`harness install`, `harness update`, and `harness uninstall` all accept the same component names. Name one or more to act on only those. Name none to act on the command's default.

| Component | What it covers | Kind |
| --- | --- | --- |
| `clients` | Claude Code, Codex, and OpenCode | Tool |
| `packages` | node, gh, and uv | Tool |
| `mcps` | Engram and agent-browser, and their registration in each client | Tool and managed config |
| `skills` | The skills, copied into each client | Managed config |
| `agents` | The role agents, rendered for each client | Managed config |
| `floor` | The floor, as each client's global instructions | Managed config |
| `settings` | Tracked client settings, compaction hooks, and command rules | Managed config |
| `aliases` | The yolo aliases and the line that loads them from your login shell's startup file | Managed config |

The two kinds:

- **A tool** is a program on your machine.
- **Managed config** is what the harness writes into a client. The harness records it in `~/.harness/state.json` so it can tell its own files from your edits.

To limit a run to some clients, add `--client claude`, `--client codex`, or `--client opencode`. You can repeat the flag.

## Install and uninstall

```sh
harness install                          # every component, every client
harness install skills agents            # only these, in every installed client
harness install --client claude          # every component, Claude Code only
harness uninstall                        # all managed config; tools stay
harness uninstall mcps --client codex    # Codex's MCP registrations
harness uninstall --all                  # everything, then the harness binary
```

A choice of components or clients applies to that run only. A later `harness install` with no names installs everything again, including a client you removed with `harness uninstall --client`.

### How install treats tools

- **A tool on your `PATH` counts as installed**, however it got there. Install never reinstalls or upgrades it. Use `harness update` for upgrades.
- **A missing tool is installed** with Homebrew, npm, or Anthropic's installer. Only then does install use the network. It records each tool it adds.
- **Without `--client`**, install configures every client that is installed or that this run installs.
- **Naming a client that is not installed** stops the run before it writes anything.
- **Installing `mcps` also reconciles `settings`**, because Engram's setup rewrites `settings.json` and `opencode.json`. The output says when this happens.

### How uninstall treats tools

- **With no names**, uninstall removes the managed config of every component and leaves every tool installed.
- **Naming `clients`, `packages`, or `mcps`, or passing `--all`**, also removes those components' tools. It removes only the tools the harness recorded installing, and uses the same method that installed them.
- **A tool you had before the harness stays**, and the summary names it.
- **A shared tool** (node, gh, uv, Engram, agent-browser) stays while any client still has harness config.
- **No uninstall deletes a tool's data** (`~/.engram`, `~/.agent-browser`, `~/.claude`, `~/.codex`) or `~/.dotfiles-backups`.

### When the harness removes itself

`~/.harness`, which holds your `harness models` choices and your review level, is deleted only when nothing recorded remains: no client keeps a component, and no tool is recorded. With `--all`, uninstall then also deletes `~/.local/bin/harness`.

Two notes for older installs:

- Machines installed before tool records existed have none. Uninstall treats all their tools as present before the harness, so it keeps them.
- Records written under the old component names `policy` and `mcp` are read as `floor` and `mcps`.

### Delete backups

`harness uninstall --clear-backups` deletes the archives that installs left under `~/.dotfiles-backups/harness/`. It runs on its own, without other arguments, and the files cannot be recovered.

## Update installed tools

`harness update` upgrades tools in this order: packages, clients, MCPs, then locked skills. Name components to update only those.

| Component | Updates |
| --- | --- |
| `packages` | Node, `gh`, and `uv` |
| `clients` | Claude Code, Codex, and OpenCode |
| `mcps` | Engram, agent-browser, its browser runtime, and existing client registrations |
| `skills` | Locked upstream skill bodies and `skills-lock.json` |

The other components carry no tool, so `harness update` refuses them and points you to `harness install`. `--client` limits `clients` and the `mcps` registrations to the named clients. `packages` and `skills` serve every client, so `--client` does not affect them.

### How the updater upgrades a tool

The updater first finds out which tools are installed and how. Then:

- It updates each tool through the source it came from, such as its Homebrew formula or cask or its vendor updater. It keeps the release channel you are on.
- It skips tools that are not installed.
- If it cannot tell how a tool was installed, it asks you to update that tool by hand.
- Copies of a client managed by Orca stay under Orca's control.
- Cask updates need Homebrew's `--no-quit` support.
- Codex installed through npm updates through the npm prefix that owns it, but only on a stable release. A prerelease or an unclear install needs a manual update.

### When an update fails

The updater keeps going after a failure. It skips any step that depends on the failed one, and at the end it exits with status 1 if anything failed. Skipped tools and manual updates do not count as failures. Completed updates are not rolled back.

The summary lists version changes, unchanged versions, commands that succeeded without a visible version change, skips, manual updates, and failures.

### Where you can run it

- `clients`, `packages`, and `mcps` work from any directory.
- `skills` needs a Harness checkout. A bare `harness update` outside a checkout skips skills.

### Updating skills safely

`harness update skills` and `harness skills update` both refuse to run if a locked skill body or `skills-lock.json` has any local changes: staged, unstaged, untracked, or ignored. Your other work stays untouched.

After an update, read the changed skill bodies and the lockfile, then run `harness install` to deploy them. The updater does not commit or publish anything. It does not install missing tools or update the harness itself.

## Review commands

`harness review` sets how much review happens on its own, and posts a review's verdict on its PR. [How review works](review.md) explains the review itself.

### Set the review level

```sh
harness review level            # print the current level
harness review level high       # set it: off, medium (default), or high
harness review level --json     # machine-readable, used by agents
```

The level is stored in `~/.harness/state.json` and applies to every repository on this machine. Only you set it. Agents read it but never change it. `/land` uses the level in force when it runs, not the one in force when the PR was opened.

### Where a review's result lives

A review's result is one verdict comment on the PR, never a file on your machine. It opens with a hidden marker that `harness land check` and `harness pr status` read, then shows the verdict, the reviewed range, the level, and the notes: each confirmed or fixed finding with the run that proved it, plus coverage and how many findings each reviewer had dropped.

| Verdict | Meaning | Holds `/land merge`? |
| --- | --- | --- |
| `PASS` | Nothing left to report | No |
| `PASS+NOTES` | It may land; the notes say what you should still read | No |
| `FAIL` | A confirmed finding is unfixed, or part of the change went unreviewed | Yes, until a newer verdict replaces it or you accept it by name |

Only the newest verdict from someone with write access to the repository counts. A verdict from anyone else is listed as ignored. A review of work with no PR yet posts nothing; `open-pr` opens the PR first and then runs the review that posts it.

A run-as-user verdict is a second kind of comment with its own marker, posted with `--run` once an agent that did not write the change has driven it as a user would. It names the one commit that was driven and reads `PASS`, `PASS+NOTES`, or `FAIL` the same way. Neither kind counts as the other. `/land merge` holds a `critical` or `behavioral` PR until a run verdict covers its code, or you accept landing without one by name. Later edits to tests or docs keep it, and any other edit makes it stale.

### Commands the review skills run

The review, `open-pr`, and `/land` skills call these instead of working the answers out by hand. You can run them to see what an agent saw:

```sh
harness review target 42                 # what a review of PR 42 reads: its base, commit, and key
harness review verdict 42 --result PASS --base <base> --sha <head> --level medium --dry-run   # the comment a review would post
harness review verdict 42 --run --result PASS --sha <head> --dry-run   # the comment a run-as-user check would post
harness review prune --dry-run           # review scratch of branches that exist nowhere (run git fetch --prune first)
```

| Command | What it answers or does |
| --- | --- |
| `target` | Resolves the current branch, a PR number or URL, another branch, or a `<base>..<sha>` range to the base, commit, and key a review reads. It fetches what it needs and creates no branch or tag. A PR from a fork is keyed `pr-<n>` |
| `verdict` | Posts a review's verdict on an open PR as one comment and prints it, or with `--run` a run-as-user verdict, which takes no `--base` or `--level`. It refuses a commit that is not the PR's current head, and a `PASS+NOTES` or `FAIL` without `--notes`. `--dry-run` prints it without posting |
| `worktree add`, `worktree retire` | Creates the one temporary worktree a review may use, outside your checkout with Git hooks off, and removes it later. Anything it holds is saved as a patch in the review directory before removal |
| `snapshot` | Saves uncommitted work under the named paths as one patch and prints its digest, so a review of work not yet committed reads exactly that content. It leaves your index and working tree as they were |
| `remove`, `prune` | Delete a review directory, the scratch a review run leaves in the Git directory (saved patches and the worktree record). `/land` removes a merged branch's directory; `prune` removes those of branches that exist nowhere. Both leave a directory alone while the review worktree it records still exists |
| `evals seed`, `evals score` | The review skill's own evals. `seed` lists candidate bug cases from past `fix:` and revert commits, with the lines each introducing commit got wrong, for you to confirm. `score` grades a run against the confirmed cases (recall, false positives, dropped findings) and says whether it regressed against the last run on the same cases |

## Worktree commands

`harness worktree` lists a repository's linked worktrees and removes one without losing work. `/land`, review, and `build` use it to retire the worktrees they create, and `/cleanup` uses it to sweep the rest.

```sh
harness worktree audit                             # every linked worktree and whether it is safe to retire
harness worktree audit --forge                     # also each branch's PR, and the head to retire a merged one into
harness worktree retire ../my-task                 # remove the worktree and its landed branch
harness worktree retire ../my-task --keep-branch   # remove only the worktree
harness worktree retire --branch feat/x            # delete a landed branch that no worktree holds
```

`audit` removes nothing. It prints one line per worktree with a state:

| State | Meaning |
| --- | --- |
| `safe` | Clean, and its branch has landed |
| `dirty` | It holds uncommitted changes |
| `unlanded` | Its branch is not proven to be on the default branch |
| `loose` | It is detached at a commit no branch holds |
| `ignored` | Only ignored files stand in the way, and removal would delete them |
| `locked` | You locked it with `git worktree lock` |
| `missing` | Its directory is gone. `git worktree prune` clears it |

`retire` refuses, and changes nothing, in every state but `safe`. Each refusal names the flag that answers it: `--allow-ignored` once you have checked the ignored files are build output, `--save-patch <file>` to keep uncommitted work as a patch, or `--keep-branch` to leave the branch. It never removes the main worktree.

A branch has landed when it is an ancestor of the default branch, or when merging it would change nothing, which is how a squash merge reads. If the default branch later rewrote the same lines, the branch reads as `unlanded` even though its PR merged. Pass `--into <ref>` to compare against another ref. `audit --forge` reads each branch's PR through `gh`: an open PR marks its row held, and for an `unlanded` branch whose PR merged it fetches that PR's head and prints it under `INTO`, so `retire --into <that head>` can prove the branch holds nothing the merged PR lacked. Otherwise neither command fetches, so run `git fetch --prune` first.

Run `harness worktree audit --help` and `harness worktree retire --help` for every flag.

## Land commands

`harness land check` answers whether a PR can land, and how. `/land` runs it and acts on the answer, so the rules live in the command and you can read the same verdict yourself. It reads GitHub through `gh`, fetches the PR's head and base, and changes nothing else.

```sh
harness land check                 # the PR of the branch checked out here
harness land check --merge 19 20   # can these merge? Printed in the order they would land
harness land check --merge --stack 23   # PR 23 and every open PR under it
```

It prints one JSON verdict per PR. The fields you will read most:

| Field | Meaning |
| --- | --- |
| `lane` | `open` (it can be merged), `merged` (clean up after it), or `stop` |
| `holds` | Every reason not to merge. Empty means the gates passed. For a merged PR, what the merge skipped |
| `mode` | `local` when this checkout is the PR's head, `fork` for a PR from another repository, otherwise `remote` |
| `mine` | Whether you wrote the PR. `/land` deletes the remote branch, and rebases a PR stacked on it, only when it is yours |
| `review` | Whether a review is required, the PR's latest verdict (its result, link, and reviewed commit), and whether it still covers the head |
| `run` | The same for the run-as-user verdict: whether one is required, the latest one, and whether it still covers the head |

Without `--merge`, an open PR reads as `stop`. Run `harness land check --help` for every field.

### Merge

`/land merge` merges through `harness land merge`, which writes to GitHub, so run it only when you mean to merge:

```sh
harness land merge 19 --head <sha> --body-file body.md   # squash-merge PR 19 at the head check printed
```

It runs the check again and refuses, changing nothing, when the PR is not on the open lane, its head moved, or a GitHub gate holds it (a conflict, a red or pending required check, a missing approval, an unresolved thread). It also refuses any other hold, such as a `FAIL` review verdict or a stale review, unless you pass `--accepted`. The squash commit takes the PR title as its subject and the file as its body, plus the description's `## Tradeoffs` section when the file has none. It never deletes the branch, and it succeeds only once GitHub reports the PR merged.

### After the merge

`/land` runs these once a PR has merged. Each one is safe to run again.

```sh
harness land sync                     # fetch and fast-forward the local default branch
harness land finish 19 20             # clean up after merged PRs 19 and 20
harness land repair 24 --landed 23    # move PR 24 onto the default branch after PR 23 was squash-merged
harness land repair 24 --push         # publish that move
```

- **`sync`** fast-forwards the default branch in the worktree that holds it, or moves the branch when none does. A default branch with commits `origin` lacks is reported and left alone.
- **`finish`** refuses an unmerged PR before it removes anything. For each merged PR it removes the review directory, deletes the remote branch only when the PR is yours, not from a fork, and not the base of another open PR, and retires the local branch and worktree once they are proven landed. This checkout goes last. It prints each action as done or kept, with the reason.
- **`repair`** fixes a PR that was stacked on one you squash-merged. With `--landed`, it rebases the dependent's own commits onto the default branch locally and refuses, changing nothing, when the PR is not yours, comes from a fork, holds a merge commit, or its own change would not survive intact. With `--push`, it publishes the result with a lease, so a push someone made in the meantime fails instead of being overwritten.

## PR commands

`harness pr` reports whether a PR is ready to merge and lets agents answer review threads. `open-pr` builds its readiness report from `pr status`, and `address-review` works through what it lists.

```sh
harness pr status                 # the PR of the branch checked out here
harness pr status 19 20           # these PRs, in landing order
harness pr status --stack 23      # PR 23 and every open PR under it
harness pr status --watch 19      # wait while required checks are pending
```

It prints a JSON verdict per PR with the same holds, review coverage, and run-as-user coverage as `harness land check --merge`, plus every unresolved review thread and its author's permission. It writes nothing to GitHub. The exit code names the first PR's first blocker, in the order to work on them:

| Exit code | First blocker |
| --- | --- |
| 0 | None, ready to merge |
| 3 | A merge conflict |
| 4 | An unresolved review thread |
| 5 | A failing required check |
| 6 | A pending required check, or mergeability GitHub is still computing |
| 7 | Any other hold, such as a draft, a missing approval, a review or run-as-user verdict that is not current, or a `FAIL` verdict |
| 8 | `--watch` timed out (default 30 minutes) |

`harness pr threads reply <pr> <thread id> --body-file <file>` posts a file's exact text as a reply, and `harness pr threads resolve <pr> <thread id>...` resolves threads. Both write to GitHub, so agents run them only under the authority they already have for that PR.

## Skill commands

Skills live in `skills/<group>/<name>/`, and clients call each one by its bare name. Every skill also has two links, `.agents/skills/<name>` and `.claude/skills/<name>`, which these commands keep correct.

```sh
harness skills new my-skill writing             # a local skill with a starting body and both links
harness skills new my-skill planning --guarded  # the same, but only callable by name
harness skills move my-skill design             # move a skill to another group, keeping its history
harness skills add <owner/repo> <name> <group>  # lock a third-party skill
harness skills update                           # update every locked skill from upstream
harness skills remove <name>                    # remove a locked skill and its lock entry
harness skills verify                           # recheck every locked body's hash, offline
```

`--guarded` adds the invocation guard for each client, so no agent starts the skill on its own. `move` uses `git mv` and keeps a locked skill's hash. Both `new` and `move` refuse a name that exists, an unknown group, or the group a skill is already in, and write nothing. A group is a directory under `skills/`, so to add a group, create its directory before you move a skill into it.

Locked skills come from other projects and change only through `add`, `update`, and `remove`. [Updating skills safely](#updating-skills-safely) covers what `update` refuses.

## Ticket commands

`epic` writes slice drafts to `docs/agents/tickets/<feature-slug>/`, one `<NN>-<slug>.md` file per ticket. These commands read them:

```sh
harness tickets check                      # validate every feature's drafts
harness tickets check my-feature           # one feature
harness tickets frontier my-feature        # which tickets are ready, done, and blocked
harness tickets frontier my-feature --done 1,2
harness tickets publish my-feature --dry-run
harness tickets publish my-feature
```

- **`check`** prints each problem as `path:line: message`: a missing or empty section, a reused number, a blocker that is not in the feature, or a cycle. A draft needs a title and five sections: What to build, Acceptance criteria, Seam, Blocked by, and Out of scope.
- **`frontier`** is how `build` picks the next tickets. A ticket is done when `--done` names it or its draft says `Status: done`, and ready once every ticket blocking it is done.
- **`publish`** creates GitHub issues, blockers first, with native dependency links. It records each issue in its draft as it goes, so a rerun after a failure creates no duplicates, and removes a draft once its issue reads back correctly. It refuses a repository whose tracker is not GitHub. It writes to GitHub, so agents run it only when you ask to publish. `--dry-run` shows every issue and link and writes nothing.

## Eval commands

The eval playbook in `/jvillegasd-mode` uses these to test whether a skill change changes agent behavior, with blinded sessions:

```sh
harness eval plant --variant <skill dir> --baseline <skill dir> --client claude --runs 3
harness eval run <run id> --prompt-file prompt.md --model <a> --model <b> --dry-run
harness eval run <run id> --prompt-file prompt.md --model <a> --model <b>
harness eval clean <run id>
```

- **`plant`** makes `--runs` directories per version under `~/.scratch-projects` (or `$HARNESS_SCRATCH_ROOT`). Each looks like an ordinary project and holds one version of the skill where the client loads it. Your installed skills are never touched. The key that says which directory holds which version stays in `~/.harness/eval/<run id>/`, outside every candidate.
- **`run`** starts one headless session per directory and model, each allowed to edit only its own directory, and saves the outputs by neutral label. Every model runs against both versions. `--dry-run` prints every command so you can approve the model budget first. A run runs once.
- **`clean`** deletes the run's directories, key, and records, and nothing else.

`harness bench` is the other half: it measures cost and friction in real sessions after a change ships. See [Bench](../bench/README.md).

## Attempt log

The hillclimb playbook in `/jvillegasd-mode` records every attempt with `harness attempts`:

```sh
harness attempts add --log ~/attempts.tsv --hypothesis "cache the index" --change "add LRU" \
  --before 812 --after 640 --gate pass --outcome kept
harness attempts list --log ~/attempts.tsv
harness attempts list --log ~/attempts.tsv --json
```

The log is a tab-separated file outside the repository. `add` fills in the id, the time, and the change as a number and a percent. A value that is not a number, or an unknown `--gate` or `--outcome`, is refused and nothing is written.
