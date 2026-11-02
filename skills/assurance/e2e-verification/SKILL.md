---
name: e2e-verification
description: "Run a scripted browser E2E suite with real screenshots on an isolated install, or create or maintain a project verify skill."
disable-model-invocation: true
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
  opencode/autoinvoke: "false"
---

Check, through the real UI, that what will be released works end to end, and show it with screenshots. Ordinary local checks belong to `verification`; a focused code inspection belongs to `review`.

Three modes share these principles. The release run (the default) is the Steps below. `create` turns what you learn about a repository into a project-local `verify-<app>` skill with a feature map, and `maintain` drives that map again to find drift. Pick the mode from the request; without a clear signal, run the release run.

## Why

A suite that runs against the wrong target, a shared system, or mocked evidence says nothing about the release. These principles keep the result trustworthy:

- Test what ships. Install the release artifact (zip, image, build output) when one exists, and otherwise test the committed revision and name it.
- Run in an isolated environment: a separate compose project, ports, database, and data directory, created fresh and seeded only with invented fixtures. Shared, QA, and production systems stay out of reach, and a shared database is read-only (schema reads and SELECT only).
- Every test saves at least one real screenshot of the observed outcome: the browser UI, a rendered document (PDFs rasterized with pdf.js), or captured CLI or HTTP output rendered as a page. Each image comes from its own test, unedited.
- Assert behavior, not markup. Fix a flaky locator or wait instead of weakening an assertion. A test that fails on a real defect stays failing and becomes a finding.
- QA is read-only for the product: application code, configuration, and data outside the isolated environment change only when the user asks for a fix. Then the failing test is the RED evidence and turns GREEN before delivery.
- Keep secrets and `.env` contents out of logs, screenshots, and the report.

## Steps

These steps are the release run.

1. **Scope.** From the request, the diff or release notes, and the install guide, list the user-visible behaviors that ship. Group them into areas, such as release and install, the main flows, error handling, permissions, and isolation. Give each test a stable ID (`R1`, `P3`, `E12`) and a one-line expected outcome.
2. **Environment.**
   - Build the isolated environment from the artifact, following the product's own install steps, and record each step's output as evidence.
   - Verify that it is up before testing: the health endpoint, seeded rows, and login.
   - Note every place where the environment differs from production, and confirm that none of them hides a defect.
3. **Runner.**
   - Put the suite in the session scratchpad, not the repository, unless the user asks to keep it.
   - Use `playwright-core` with an installed Chrome (`executablePath`), or with agent-browser's Chrome for Testing, headless, at a fixed viewport and locale. Add `pdfjs-dist` when documents are produced.
   - Start from `scripts/kit.mjs` (copy it next to the runner). It provides `test`, `ok`, `shot`, `evidence`, `viewPdf`, `fetchBody`, and `finish`, which writes `shots/results.json`.
4. **Write tests the way a user acts.** Use real clicks, typing, drag and drop, keyboard, and native dialogs. Check the server-side effect too: a GET response, a database SELECT inside the isolated environment, or the PDF's text. Cover each area's main path, its error paths (400, 403, 404, validation messages), concurrency or race paths where an async UI allows them, and tenant or company isolation. Keep tests independent, or order them explicitly, and reset the test data before each run.
5. **Run and stabilize.**
   - Run the full suite. Rerun single tests with an ID filter. A test may depend on a setup test, such as the one that opens the page.
   - Investigate every failure until it is classed as a test bug, which you fix and rerun, or a product defect, which becomes a finding.
   - Collect browser console errors, and treat unexpected ones as failures.
   - Finish only when two consecutive runs give the same result.
6. **Findings.** For each defect, give the exact reproduction, the expected and actual behavior, the screenshot, and supporting evidence such as database rows or response bodies. Point to the likely code location when it is known. Rank by severity: data loss or security first, then wrong output, then usability.
7. **Report.**
   - Build `report.html` with `scripts/report.mjs`. It shows summary tiles, findings, and one card per test with its checks and clickable screenshots.
   - When an artifact or page-publishing tool is available, publish the report privately, with the screenshots as supporting files. Otherwise give the absolute local path.
8. **Clean up.** Offer to stop and remove the isolated environment (for example `docker compose down -v`), and leave it running until asked, because the user may want to rerun tests. When a fix is authorized, rerun the failing test (RED, then GREEN) and the full suite before delivering it.

## Mode: create

Study the repository and write a project-local skill that any later session can use to launch, drive, and observe the app. The skill lives in the target repository, not here, because launch commands, selectors, and gotchas belong to one product.

1. **Study.** Find how the app runs (start commands, ports, data directory, required services), how to drive it (the UI, CLI, or API a user touches, with real selectors and commands from the code and docs), how to observe it (the UI, logs, files, database rows inside the isolated environment), and how to isolate it from shared state.
2. **Write `verify-<app>/SKILL.md`** in the target repository, with these sections:
   - Launch: start the app from an empty data directory, so no earlier state or real data is involved.
   - Doctor: one read-only health check that says whether the app is up and usable.
   - Drive: the real selectors and commands for the main actions.
   - Evidence: exercise the real user path, capture the action and the resulting state, and keep the evidence through cleanup.
   - Cleanup: stop and remove only what Launch started, by recorded identity such as a compose project, a PID file, or a port the skill opened, never by process name. Cleanup never deletes the evidence.
3. **Write the feature map.** Create `verify-<app>/features/README.md`, an index of the app's features with one line each, and one file per feature, starting with 3 to 5 of the most important. Each feature file has an H1, one paragraph saying what the feature is for, and exactly four sections:
   - Sub-features: the distinct behaviors inside it.
   - How to get to it: the route to it from a user's point of view.
   - Driving it: the preconditions, then the action, the exact command or clicks, and the observable result.
   - Gotchas: what trips a run, including every missing precondition found while driving.
4. **Prove it once.** Drive one feature end to end by following only the generated skill, and keep the screenshots or captured output. Fix the skill where it was wrong. A missing precondition fails the journey and goes under Gotchas; the journey is never weakened to pass. A generated skill that never ran is a draft, and the report says so.

## Mode: maintain

Keep an existing `verify-<app>` skill true to the app. It may run on a schedule, so it needs no human in the loop and edits only the verify skill's own files.

1. **Drive every mapped feature live**, even when the source looks unchanged, because docs that look right can still fail against the running app.
2. **Read each feature's source** for drift: renamed routes, changed selectors, flags, defaults, and preconditions.
3. **Sort each problem** into one of three kinds:
   - Doc drift: the map or skill is wrong. Fix the map.
   - Harness gap: the app is right but the driver cannot reach it. Fix the driver.
   - Product gap: the app does not do what its feature file says it should. Record it with the reproduction and leave product code alone.
4. **Report the outcome** as `clean` (everything drove as documented, nothing edited), `changed` (the verify skill's files were fixed, listed with the reason), or `blocked` (something could not be driven or decided; say what is needed). Product gaps are listed under any outcome.

The feature map also feeds the change workflow: when it exists, `change-map` names the affected feature for each behavioral or critical item, and `verification` drives those features (ladder step 5).

## Output

Report:

- The tested revision or artifact and the environment, including how it differs from production.
- The pass count per area, and each finding with its severity and evidence.
- The report link or path and the screenshot folder.
- What was not covered, and why.
- What is still running.
- For `create`: the files written and which feature was driven as proof, or that the skill is still a draft. For `maintain`: the outcome (`clean`, `changed`, or `blocked`), the problems found by kind, and the files edited.

Report a skipped or unrunnable test as exactly that.
