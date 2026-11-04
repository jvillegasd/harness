---
name: journey-verification
description: "Create and run persistent frontend, backend, API, and logical journey tests with only the services each journey needs."
license: MIT
---

Turn a meaningful sequence of actions into repeatable tests of real application behavior. A journey crosses the use cases needed to reach an outcome and checks the resulting state and side effects. A journey can cover a frontend with mocked APIs, backend logic with provider fakes, or a complete application with real dependencies. Its boundary determines what the result proves. Tests and their supporting assets stay in the target repository; `e2e-verification` handles disposable end-to-end runs on a named feature, bugfix, PR, or path.

## Choose the boundary

Inspect the requested behavior, application entry points, existing tests, runner, and dependency setup. Select the smallest boundary that exercises the journey: browser interactions, application interfaces, API handlers, CLI commands, or a running backend. Keep the application behavior inside the selected boundary real. Start only the frontend, backend services, databases, or queues the journey requires.

Name each journey's preconditions, action sequence, expected outcomes, and dependencies before writing it. Reuse existing coverage rather than copying a boundary's rules into another test layer.

## Build a reusable suite

Use the project's native runner and test layout. Reuse its browser, mocking, fixture, factory, HTTP testing, and database tools. For frontend journeys, prefer the existing Cypress or Playwright setup when present. Prefer standard-library or installed tools; add a test dependency only when the delivered suite needs it. Choose tools from the actual stack rather than introducing a universal runner.

Persist the supporting assets each journey needs alongside the tests: invented fixtures or factories, external-provider mocks or fakes, setup and teardown helpers, and minimal service configuration. Follow existing conventions and expose a repeatable command through the project's normal test mechanism. Document required dependencies and setup where the project keeps test instructions.

Replace dependencies outside the selected boundary at their I/O boundaries when necessary, including API responses for frontend-only journeys or external providers for backend journeys. A fake should model the responses or failures the journey needs, not implement the application's business rules. Assert observable effects such as visible UI state, navigation, persisted state, returned output, emitted events, or captured provider requests, rather than internal call order. State which integrations remain unverified because of substitutions.

Use isolated temporary data and test-owned resources. Keep tests independent; preserve action order within each journey. Control time and randomness where relevant, and await observable completion with bounded waits rather than fixed sleeps. Clean up only resources created by the suite, including on failure, while retaining useful failure evidence. Use no shared or production data.

For frontend journeys, drive real user interactions and assert visible outcomes with stable semantic locators or the project's test identifiers. Reuse API interception, response fixtures, and authentication helpers where appropriate. An intercepted successful response proves frontend handling, not backend execution. Retain useful browser failure artifacts using the runner's conventions; screenshots are not mandatory for every test.

## Exercise and report

Cover the requested successful journey and its meaningful rejection or failure paths. Assert relevant intermediate state and final outcomes; successful status codes alone do not establish the journey. Include retries, permissions, or isolation when they matter to the requested behavior, rather than generating a generic checklist.

Run the selected journeys and relevant project checks. Separate test setup defects from product defects. Repair test defects and rerun; keep product defects as failing evidence unless fixing the product is authorized. A skipped or unavailable test is never a pass.

Report the tested revision or content identity, commands and results, persisted test and helper locations, required services, substitutions, and coverage gaps. Explain how to rerun from clean test state. Running these tests proves only their selected boundary; claim browser, backend, deployment, and provider coverage only where actually exercised.
