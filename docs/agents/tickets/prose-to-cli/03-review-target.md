# 03 Resolve a review target

## What to build

`harness review target [<pr number or URL> | <branch> | <base>..<sha>]` resolves what a review reads into one JSON object: the kind of target, `base` and `sha` as full commit ids, the ledger `key`, whether the target is `foreign` (anything but the current task branch), and whether it is a `fork`.

- No argument: the current task branch. The key is the branch name, and a detached head is refused.
- A pull request: its base and head are fetched from origin, and the command refuses when the fetched head is not the head the forge reports. `sha` is that head, `base` is its merge-base with the pull request's base branch, and the key is the head branch name, or `pr-<n>` for a fork.
- A branch, local or `origin/<name>` (fetched first when remote): `sha` is its tip, `base` its merge-base with its open pull request's base, or with the default branch when it has none. The key is the name without the remote.
- A range: both ends resolved to full ids; the key is `range-<base>-<sha>` with each cut to 12 characters.

`review`'s Target section keeps the meaning of foreign and fork and calls this command for the resolution.

## Acceptance criteria

- Each of the four target kinds prints the `base`, `sha`, `key`, `foreign`, and `fork` described above against a throwaway repository with an origin.
- A fork's pull request is keyed `pr-<n>` and reports `fork: true`.
- A pull request whose fetched head differs from the forge's reported head is refused with a message naming both, and exits non-zero.
- An unknown branch, an unresolvable range end, and a detached head with no argument are each refused with nothing printed on stdout.
- The command creates no local branch and no tag.
- The key printed for a pull request equals the `key` `harness land check` prints for it.
- `review`'s Target section names the command and no longer spells out the `gh` and `git` calls.

## Seam

Proposed, awaiting the human's confirmation: the review ledger package's tests over throwaway repositories and a fake forge settle the resolution rules; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Creating the read worktree for a foreign target (ticket 05). Deciding whether the target is already reviewed (ticket 04).
