# Review levels: comparison and recommendation

Status: proposed level changes only, awaiting human approval. The safeguard implementation preserves existing level behavior and the machine's selected level. This report is planning input, not authority to change either.

## Sources and limits

Compared on 2026-11-03 against Harness `9c86eb4` and the pinned upstream sources below. This is a comparison of contracts and instructions. No live model evaluation, cost measurement, installation, or publication was performed. Instructions within upstream skills were treated as comparison data, not commands to this session.

| Source | Observed contract | Useful lesson |
| --- | --- | --- |
| [Matt Pocock code-review](https://github.com/mattpocock/skills/blob/4ae2317ddb22b046e26455e4f9c9b3aaaf62212d/skills/engineering/code-review/SKILL.md) | Two fresh parallel agents review standards and requirements, with separate short reports. Missing requirements skip the spec axis. This skill defines no off/medium/high system, independent refutation stage, or repair validator. | Preserve spec coverage where requirements exist and keep axes separate. |
| [pstack blast-radius](https://github.com/backnotprop/pstack/blob/ecec42adbe4f240e4ae19d296768445f0dfbd38e/skills/blast-radius/SKILL.md) | Trace downstream contracts and prove the load-bearing safety fact with real code. Wide changes can use multiple models. | A clean hunt should state how far its safety assumption was proven. |
| [pstack shipping](https://github.com/backnotprop/pstack/blob/ecec42adbe4f240e4ae19d296768445f0dfbd38e/skills/poteto-mode/playbooks/shipping.md) | An independent verifier runs each PR as a user would before landing. CI and bot approval do not replace this verdict. Patch drift requires reassessment. | Keep evidence independent and bound to the reviewed patch. These are shipping requirements, not a review-level hierarchy. |
| [pstack Bugbot triage](https://github.com/backnotprop/pstack/blob/ecec42adbe4f240e4ae19d296768445f0dfbd38e/skills/poteto-mode/references/bugbot-triage.md) | Fix plausible behavior defects, dismiss only supported low-risk noise, ask about consequential ambiguity. Historical dismissals do not authorize broad exemptions. | Require evidence for dismissal rather than treating every comment as mandatory work. |
| [Gentle-AI native review integration](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/docs/review-integration.md) | The user controls the review switch. Passive/medium/high describe candidate risk, not user assurance preferences. Automatic review is due for high risk; size is a context budget. Medium defaults to reliability, high to four lenses, with explicit selection possible. Go binds immutable evidence, one refuter batch, and one bounded correction with targeted validation. | Separate permission to start review from risk-selected depth. Freeze inputs and bound correction. |

Gentle-AI's [bundled reliability lens](https://github.com/Gentleman-Programming/gentle-ai/blob/3ea08cdfbdce671053c82377a4365915799abe57/internal/assets/claude/agents/review-reliability.md) describes one or three refuters, while its native integration specifies one transaction-wide batch. The matrix uses the native integration contract, not the bundled lens's older orchestration instructions. No claim is made that a particular runtime follows either correctly in practice.

## Current versus recommended levels

These are recommendations for a later approved change. Retain `off`, `medium`, and `high` and the default `medium`. Treat the level as the human's assurance preference and the change tier as the depth selector.

| Mode | Current behavior | Recommended behavior |
| --- | --- | --- |
| `off`, automatic | No review starts. | Unchanged. Never silently escalate a human-disabled review. |
| `medium`, ordinary PR above `tests-docs` | Quick bugs hunt, parent reproduction, independent refuter for unresolved findings. No repair validator. | Keep the trigger and lean verification. Add a spec hunter when requirements exist. Add one scoped validator after substantive confirmed repairs. |
| `medium`, critical PR or qualifying local handoff | Same Quick hunt despite concrete security, data-loss, compatibility, or concurrency risk. | Standard coverage: bugs, spec when available, standards, and risk for named categories. Use one batched executable verifier and independent refuter when findings need them, plus one repair validator. |
| `medium`, other local handoff | No automatic review. | Unchanged. |
| `high` | Review structural and higher changes locally and before PR delivery. Quick for structural/behavioral, Standard for critical, Deep for a critical one-way door. | Keep triggers. Use bugs plus available spec for structural/behavioral; retain Standard/Deep coverage for critical changes. Keep batched verification and scoped repair validation. |
| Explicit user request, any level | Standard unless a depth is named; batched verification and repair validation. Deep falls back to Standard when the configured reviewer uses the session client. | Keep behavior and explain coverage/fallback before launching. Resolve explicit Quick requests independently of automatic-level rules. |

Keep the current exclusions for mechanical/tests-docs-only automatic work. An explicit request still receives a review. Preserve the four-hunter cap, with standards dropped first when Deep needs a second bugs hunter. Preserve independent refutation and human disposition of unresolved findings.

### Rationale, assurance tradeoffs, and calls

Counts below are role tasks per slice, before input retries or delegated fixes. Verifier/refuter/validator calls occur only when their phase has work. Each retry is an additional invocation, not a new assurance guarantee.

| Recommendation | Basis | Expected benefit and tradeoff | Call implication |
| --- | --- | --- | --- |
| Preserve available spec coverage at ordinary levels | Matt's independent spec axis | Makes requirements omissions an explicit review concern. Adds context and a hunter; overlap with bugs is possible. | Medium ordinary: up to 2 hunters + 1 refuter + 1 repair validator = 4, versus 2 currently. High ordinary with batched verification: up to 5, versus 4 currently. |
| Escalate critical changes at medium | pstack's risk tracing and execution; Gentle's risk-selected coverage | Gives named harm paths dedicated attention and executable verification. More expensive even though the selected user level stays medium. Benefit is unmeasured. | Up to 4 hunters + 1 verifier + 1 refuter + 1 validator = 7, versus 2 currently. |
| Validate substantive repairs at medium | Gentle's targeted correction and pstack's patch-bound evidence | Avoids automatically ending a confirmed repair with no independent repair check. Adds one task only when applicable. | Included in the proposed ceilings above. |
| Keep independent refutation rather than majority voting | Existing Harness contract; Gentle's current batch contract | Retains a cited failure-path cut and uncertainty reporting. Does not treat agent agreement as execution proof. | At most one refuter task per slice, not per finding. |

Do not claim fixed token cost: slices, findings, context size, retries, probes, and fixes affect spending. Actual bug recall, false positives, cost, and friction require separately authorized measurements.

### Consistency work if the matrix is approved

Update `POLICY.md`'s Verification and review guidance and the review skill's Level/Depth tables together. Remove the old blanket statement that medium repairs have no validator. Keep `open-pr` and `land` aligned on reviews covering the current patch for PRs above tests-docs. Check build's extra-spec rule against the new ordinary spec coverage to avoid duplicate hunters. Preserve the review-level CLI enum, default, and human-only setting authority. No schema migration is needed.

## Evaluation candidates awaiting confirmation

These are candidates, not registered cases. No `confirmed_by` values, eval-store writes, or model runs have been fabricated. Bases are the first parents of the listed heads. Expected locations refer to those historical trees, not today's grouped skill paths.

| ID | Kind | Base / head | Expected finding or review target | Evidence and limits |
| --- | --- | --- | --- | --- |
| `state-null-map` | Defect | `214ab7c^` / `214ab7c5b12e7a2243799b8b92233210edd0607a` | `internal/statefile/statefile.go:26-29`, class `null-state-map`: literal JSON null returns a nil map; a caller assigning its owned key panics. | `git blame ff0f076^` maps these lines to this head; `ff0f076` adds the nil-map guard and `TestSaveLevelOverANullState`. This head is a retained pre-squash object; confirm availability in the evaluation checkout. No historical run performed here. |
| `quoted-orca-report` | Defect | `673b5a3^` / `673b5a3e9d8b252ad655d053a2f69d2a680fc89e` | `internal/models/lifecycle.go:38-41`, class `quoted-prose-denied`: a literal report message containing punctuation such as parentheses or a question mark is denied even inside single quotes. | `git blame e4f1b20^` identifies the introduction; `e4f1b20` replaces the blanket rejection with quote-aware parsing and adds regression tests. Historical runtime delivery remains unverified. |
| `ledger-finding-relabel` | Defect | `3cb1a89^` / `3cb1a89a739d10fbe8f71b4fc034f70ab9d6c2eb` | `internal/reviewledger/command.go:164-175`, class `finding-relabel`: updating an existing finding can replace severity and other identity fields, permitting a defect to be relabelled as a nit. | `git blame 6a2e3bf^` maps the override table to this head; `6a2e3bf` restricts updates to disposition/evidence/commit and adds regression coverage. Retained pre-squash object, no historical run here. |
| `standards-thread-clarity` | Provisional clean | `618ac08^` / `618ac08a1d78648cc5ff8fc0887fa4245085e82c` | Review the complete four-file change to standards and thread triage. Expected findings: none, pending human inspection. | Merged change; no later edits through `9c86eb4` to its standards and triage reference files. This is weak negative evidence, not proof of cleanliness. Today's safeguard edits are excluded. |
| `cli-help-clarity` | Provisional clean | `9c86eb4^` / `9c86eb4fa7083d48aa56f122b9b9bd05eef85905` | Review all six files, including Go help text changes, not only `docs/cli.md`. Expected findings: none, pending human inspection. | Merged current baseline; no observation window after the change. Particularly weak clean label until independently inspected. |

Before running, confirm all five labels with the human and verify every base/head resolves. Provide the exact same cases and depth to baseline and revised-skill runs while keeping expected findings hidden from hunters. Freeze the review-skill revision and assigned model/effort separately from the historical target tree. Score recall, confirmed false positives, and unresolved findings using `skills/workflow/review/references/evals.md`. Do not infer a speedup or correctness improvement from repository checks.
