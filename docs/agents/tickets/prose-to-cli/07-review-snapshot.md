# 07 Snapshot uncommitted work for review

## What to build

`harness review snapshot --base <base> [--branch <key>] -- <owned paths>...` captures work that may not be committed as one binary patch against the base, saves it in the key's review directory, and prints the patch's path and its sha256 digest. The patch covers tracked changes and untracked files under the owned paths. The index and the working tree are left byte for byte as they were.

The digest is the content identity `review` records with `--digest` and `verification` binds evidence to, so both stop describing how to compute one.

## Acceptance criteria

- Two snapshots of the same content print the same digest; changing one byte of an owned file changes it.
- An untracked file under an owned path is in the patch and changes the digest.
- A change outside the owned paths is not in the patch and does not change the digest.
- Applying the saved patch to a clean checkout of the base reproduces the owned paths' content, binary files included.
- After the command, `git status --porcelain` and the staged diff are identical to before it ran, including a file that was partly staged.
- With no changes under the owned paths, the command exits non-zero and writes no patch.
- `review`'s Snapshot step 1 and `verification` step 3 name the command and no longer spell out the diff and hash.

## Seam

The review ledger package's tests over throwaway repositories settle the snapshot; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Deciding which paths are owned. Recording the review itself, which stays `harness review record`.
