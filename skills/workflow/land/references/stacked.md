# Stacked pull requests

A pull request is stacked when another open pull request uses its head branch as its base. Squash merging replaces the head's commits with one new commit, so each direct dependent must be retargeted and rebased off the absorbed commits before the head branch goes.

## Enumerate

`harness land check` lists the direct dependents as `dependents`. Repair only a dependent whose `mine` is true: another author's branch is theirs to rebase, so report it instead. After a merge, the forge may already have retargeted a dependent to the default branch; retargeting again is harmless.

## Repair

For each direct dependent whose `mine` is true, run `harness land repair <dependent> --landed <n>`. It refuses, changing nothing, when the dependent is someone else's, a fork's, or holds a merge commit, or when it cannot find and prove by content the commit the dependent was built on. Otherwise it retargets the dependent to the default branch, rebases its own commits onto it in the worktree that holds its branch or in a temporary one, confirms its own change came through identical, and prints the boundary, the recorded head, the rebased head, and the worktree where the result sits. A conflict or a changed own change puts the branch back and exits non-zero.

Run that branch's own local rungs in the printed worktree, then publish with `harness land repair <dependent> --push`, whose lease fails if the dependent's owner pushed since the repair read it. Report a failed push instead of retrying. A local branch of the dependent's name that no worktree holds is stale after the push: report it and leave it.

A refusal is reported with its reason, and that dependent is left for its author: retarget to `<default>` and rebase off the landed head. Repair only direct dependents. A deeper chain keeps sitting on the old commits until its own base lands, and that run repairs it then. A conflict stops here: leave the head branch undeleted, report which dependents were repaired and which were not, and treat the rest as `/address-review` work.
