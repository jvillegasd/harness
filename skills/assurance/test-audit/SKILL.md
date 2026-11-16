---
name: test-audit
description: "Audit a test suite for value: keep each test that owns a contract, and repair, merge, or cut the rest with evidence."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

# Test Audit

Decide which tests in a scope earn their place, then repair, merge, or cut the rest without losing a contract. A test earns its place when it is the **owner** of a contract: a credible regression of behavior someone observes turns it red, and no stronger test already catches that regression. Read `principle-test-behavior-not-implementation` in full first; it defines behavior and implementation, and this skill builds on it.

## The value bar

Apply it to every test in scope.

A test is an owner when all three hold:

- **Contract.** It protects something a caller, a user, a file format, or another system observes: a result, an effect, an invariant, or an external contract.
- **Regression.** A credible edit to the production code turns it red. Name that edit.
- **Owner.** It is the strongest test of that contract, at the lowest boundary that still fails for the right reason. A second test at a higher layer earns its place only for a risk the lower one cannot see: wiring, transport, process lifecycle.

The **retention list** names contracts that stay owned even when their test is slow or static: the public CLI and API surface, config and state-file formats, migration of state already on users' machines, on-disk layout, security and permission boundaries, platform differences, defaults, byte-exact or generated output, and packaging and release. A repository's `AGENTS.md` validation contract, when it has one, extends this list with its own boundaries.

A **hollow** test makes a candidate, never a verdict on its own. These are the hollow shapes:

- It asserts nothing, compares a value to itself, or computes its expected value with the code under test.
- It restates a copied list, manifest, or constant, or greps source text, where no user-facing contract reads that text.
- It asserts call shape or a private helper that the boundary test already covers.
- Its mock supplies the very behavior the test asserts.
- Its fixture fabricates what the production owner should produce (an ordering, a receipt, a write), or asserts persistence on a path the code never writes.
- Its negative case is caught by an unrelated guard, or targets a rejection the code never reaches.
- Its name promises behavior its inputs and assertions never exercise.
- It guards the absence of something already removed (a retired command, flag, or name) that no user still depends on.
- It drives a seam that exists only for tests: an export, flag, hook, wrapper, or message with no production caller.

A test that fails at the baseline is a bug report first: reproduce it and repair the production owner.

## Steps

1. **Baseline.** Name the scope: a package, a file, or one boundary. Run its tests at the current head and record each file's result. Done when every test file in scope has a recorded result.

2. **Ledger.** Read every test in scope in full, table rows included, together with its production owner, the owner's callers, and the history that added the test (`git log -S`, `git blame`). Give each test one mark, and give each table row its own mark when rows differ:
   - `keep`: name the contract and the regression it catches.
   - `fix`: the contract is real and the assertion is hollow; name the repair.
   - `merge`: name the owner that absorbs it, such as a sibling table row or a stronger suite.
   - `cut`: name the proof that remains, or why no contract exists.

   Judge each test by its assertions, never its name. A test whose `cut` lacks evidence is marked `keep`. Done when every test has a mark and one evidence line.

3. **Approval.** Show the human the ledger grouped by mark, with the `fix`, `merge`, and `cut` rows in full. Edits start once the human approves rows; apply only the approved ones. Done when the human has answered each non-`keep` row.

4. **Prove the owners.** For each `fix`, each owner absorbing a `merge`, and each `keep` whose regression is not plainly visible: make the named regression edit in a production file that has no uncommitted changes, run the test, and see it go red. Then restore the file and confirm `git diff --quiet -- <file>`. Done when each of those tests went red on its edit and the tree holds no regression edit.

5. **Apply.** Make the approved repairs, merges, and cuts. With each cut, delete what only it used: production seams, test helpers, and fixture files. A fixture can be loaded by a glob or a directory walk, so search for its directory as well as its name before deleting it. Update any document that states a contract you cut, such as the `AGENTS.md` validation contract. Done when every approved row is applied and the repository's checks pass.

6. **Report.** Give the counts per mark, the contracts kept, each cut with its remaining proof, the regressions proven in step 4, test lines and production lines removed (from `git diff --numstat`), baseline failures filed as bugs, and follow-ups.
