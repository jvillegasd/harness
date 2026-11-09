---
name: e2e-verification
description: "Run a real end-to-end test of a feature, bugfix, PR, or path on an isolated environment, with a screenshot per test and a report."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "3.0"
  opencode/autoinvoke: "false"
---

Prove, through the real app, that one named target works end to end, and show it with a screenshot per test. The target is a feature, a bugfix, a PR, or a path. This skill is a thin layer over the project's `verify-<app>` skill: that skill owns how to launch, check, drive, and clean up the app, and this one adds the target's test list, the bugfix red-then-green proof, and the screenshot report. `verification`'s verdict is the automatic gate before a PR; this is the deeper suite a human asks for, covering error paths and neighbors as well as the main path.

Without a `verify-<app>` skill, stop and name `/create-verification-skill`; it is made once and reused by every later run.

## Why

- Test the revision under test, and name it: the PR head commit, the fix commit, the feature's commit, or a release artifact when the target ships as one.
- Run only in the isolated instance `verify-<app>`'s Launch creates, seeded with invented fixtures. Shared, QA, and production systems stay out of reach.
- Every test saves at least one real, unedited screenshot of the observed outcome: the browser UI, a rendered document, or captured CLI or HTTP output rendered as a page.
- Assert behavior, not markup. Fix a flaky handle or wait instead of weakening an assertion. A test that fails on a real defect stays failing and becomes a finding.
- QA is read-only for the product: application code, configuration, and data outside the instance change only when the user asks for a fix. Then the failing test is the RED evidence and turns GREEN before delivery.
- Keep secrets and `.env` contents out of logs, screenshots, and the report.

## Target

| Target | Test list | Extra proof |
| --- | --- | --- |
| Feature | Its user-visible behaviors: the main path, error paths, and permissions | None |
| Bugfix | The bug's reproduction, plus the neighboring behavior the fix touches | The reproduction goes red on the pre-fix revision, then green on the fix |
| PR | The PR diff, with `change-map`'s `critical` and `behavioral` items as the areas | Name the PR head commit tested |
| Path | One user path end to end: a route, a flow, an endpoint sequence, or a CLI command chain | None |

When the request names no target, ask for one. Start the test list from the feature files under `verify-<app>/features/` that the target touches, and drive every entry point they list.

## Steps

1. **Scope.** List the user-visible behaviors to prove and group them into areas, such as the main flow, error handling, permissions, and isolation. Give each test a stable ID (`F1`, `B3`, `E12`) and a one-line expected outcome. Every behavior in scope has a test, or the report names it as not covered.
2. **Instance.** Run `verify-<app>`'s Launch at the revision under test, then its Doctor. For a bugfix, also launch the pre-fix revision. Note every way the instance differs from production, and confirm none hides a defect.
3. **Runner.** Put the suite in the session scratchpad unless the user asks to keep it. Drive with `verify-<app>`'s Drive recipe, and record each test with `scripts/kit.mjs` (copy it next to the runner): `test`, `ok`, `shot`, `evidence`, `viewPdf`, `fetchBody`, and `finish`, which writes `shots/results.json`.
4. **Tests.** Act the way a user does, and check the side effect too: a response, a row inside the instance, or a document's text. Cover each area's main path, its error paths, races an async surface allows, and isolation where the target touches it. Keep tests independent or ordered explicitly, and reset test data before each run.
5. **Run and stabilize.** For a bugfix, run the reproduction on the pre-fix revision first; it must go red for the reason the bug describes. Class every failure as a test bug, which you fix and rerun, or a product defect, which becomes a finding. Treat unexpected console errors as failures. Finish only when two consecutive runs agree.
6. **Findings.** For each defect: the exact reproduction, expected and actual behavior, the screenshot, and supporting evidence. Point to the likely code location when known. Rank data loss and security first, then wrong output, then usability. A finding that shows the feature map is wrong or incomplete goes in the report for `/maintain-verification-skill`.
7. **Report.** Build `report.html` with `scripts/report.mjs`: summary tiles, findings, and one card per test with its checks and screenshots. Publish it privately when a page-publishing tool is available; otherwise give the absolute path.
8. **Clean up.** Offer `verify-<app>`'s Cleanup, and leave the instance running until asked. When a fix is authorized, rerun the failing test (RED, then GREEN) and the full suite before delivering it.

## Output

- The target, the tested revision (and the pre-fix revision for a bugfix), and how the instance differs from production.
- The pass count per area, and each finding with its severity and evidence.
- For a bugfix, the red run on the pre-fix revision and the green run on the fix.
- The report link or path and the screenshot folder.
- What was not covered, and why, and what is still running.

Report a skipped or unrunnable test as exactly that.
