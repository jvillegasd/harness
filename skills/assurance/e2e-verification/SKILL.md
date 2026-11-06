---
name: e2e-verification
description: "Run a real end-to-end test of a feature, bugfix, PR, or path on an isolated environment, with a screenshot per test and a report."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "2.0"
  opencode/autoinvoke: "false"
---

Prove, through the real app, that one named target works end to end, and show it with screenshots. The target is a feature, a bugfix, a PR, or a path. Ordinary local checks belong to `verification`, a focused code inspection to `review`, and persistent in-repo journey tests to `journey-verification`; this suite is disposable and runs against the running app.

## Why

A suite that runs against the wrong revision, a shared system, or mocked evidence says nothing about the target. These principles keep the result trustworthy:

- Test the revision under test, and name it: the PR head commit, the fix commit, the feature's commit, or a release artifact (zip, image, build output) when the target ships as one.
- Run in an isolated environment: a separate compose project, ports, database, and data directory, created fresh and seeded only with invented fixtures. Shared, QA, and production systems stay out of reach, and a shared database is read-only (schema reads and SELECT only).
- Every test saves at least one real screenshot of the observed outcome: the browser UI, a rendered document (PDFs rasterized with pdf.js), or captured CLI or HTTP output rendered as a page. Each image comes from its own test, unedited.
- Assert behavior, not markup. Fix a flaky locator or wait instead of weakening an assertion. A test that fails on a real defect stays failing and becomes a finding.
- QA is read-only for the product: application code, configuration, and data outside the isolated environment change only when the user asks for a fix. Then the failing test is the RED evidence and turns GREEN before delivery.
- Keep secrets and `.env` contents out of logs, screenshots, and the report.

## Target

The target's kind decides where the test list comes from:

| Target | Test list | Extra proof |
| --- | --- | --- |
| Feature | Its user-visible behaviors: the main path, error paths, and permissions | None |
| Bugfix | The bug's reproduction, plus the neighboring behavior the fix touches | Run the reproduction on the pre-fix revision and show it goes red, then green on the fix |
| PR | The PR diff, with `change-map`'s `critical` and `behavioral` items as the areas | Name the PR head commit tested |
| Path | One user path end to end: a route, a flow, an endpoint sequence, or a CLI command chain | None |

When the request names no target, ask for one. When the repository has a project `verify-<app>` skill, reuse its Launch, Doctor, Drive, and feature map rather than rediscovering them, and drive every entry point the map lists for the target.

## Steps

1. **Scope.** From the target, list the user-visible behaviors to prove. Group them into areas, such as the main flow, error handling, permissions, isolation, and install when an artifact ships. Give each test a stable ID (`F1`, `B3`, `E12`) and a one-line expected outcome. Every behavior in scope has a test, or the report names it as not covered.
2. **Environment.**
   - Build the isolated environment from the revision under test, following the product's own setup or install steps, and record each step's output as evidence. For a bugfix, also build the pre-fix revision, or switch the same environment to it.
   - Verify that it is up before testing: the health endpoint, seeded rows, and login.
   - Note every place where the environment differs from production, and confirm that none of them hides a defect.
3. **Runner.**
   - Put the suite in the session scratchpad, not the repository, unless the user asks to keep it.
   - Use `playwright-core` with an installed Chrome (`executablePath`), or with agent-browser's Chrome for Testing, headless, at a fixed viewport and locale. Add `pdfjs-dist` when documents are produced.
   - Start from `scripts/kit.mjs` (copy it next to the runner). It provides `test`, `ok`, `shot`, `evidence`, `viewPdf`, `fetchBody`, and `finish`, which writes `shots/results.json`.
4. **Write tests the way a user acts.** Use real clicks, typing, drag and drop, keyboard, and native dialogs. Check the server-side effect too: a GET response, a database SELECT inside the isolated environment, or the PDF's text. Cover each area's main path, its error paths (400, 403, 404, validation messages), concurrency or race paths where an async UI allows them, and tenant or company isolation where the target touches them. Keep tests independent, or order them explicitly, and reset the test data before each run.
5. **Run and stabilize.**
   - Run the full suite. Rerun single tests with an ID filter. A test may depend on a setup test, such as the one that opens the page.
   - For a bugfix, run the reproduction against the pre-fix revision first; it must go red there for the reason the bug describes. A reproduction that passes before the fix proves nothing, so fix the test until it fails for the right reason.
   - Investigate every failure until it is classed as a test bug, which you fix and rerun, or a product defect, which becomes a finding.
   - Collect browser console errors, and treat unexpected ones as failures.
   - Finish only when two consecutive runs give the same result.
6. **Findings.** For each defect, give the exact reproduction, the expected and actual behavior, the screenshot, and supporting evidence such as database rows or response bodies. Point to the likely code location when it is known. Rank by severity: data loss or security first, then wrong output, then usability.
7. **Report.**
   - Build `report.html` with `scripts/report.mjs`. It shows summary tiles, findings, and one card per test with its checks and clickable screenshots.
   - When an artifact or page-publishing tool is available, publish the report privately, with the screenshots as supporting files. Otherwise give the absolute local path.
8. **Clean up.** Offer to stop and remove the isolated environment (for example `docker compose down -v`), and leave it running until asked, because the user may want to rerun tests. When a fix is authorized, rerun the failing test (RED, then GREEN) and the full suite before delivering it.

## Output

Report:

- The target, the tested revision or artifact (and the pre-fix revision for a bugfix), and the environment, including how it differs from production.
- The pass count per area, and each finding with its severity and evidence.
- For a bugfix, the red run on the pre-fix revision and the green run on the fix.
- The report link or path and the screenshot folder.
- What was not covered, and why.
- What is still running.

Report a skipped or unrunnable test as exactly that.
