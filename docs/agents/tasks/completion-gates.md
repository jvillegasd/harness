# Completion gates and comment constraints

Branch: `feat/pstack-gaps`. Status: shipped for review.

Request: "come back the gate then and skip build gap", followed by "and add in floor auto atribution to agent and  apply the comments contrains and remove all comments from golang code, all in the same pr, deploy subagents if necessary" (live thread, November 15, 2026).

## Requirements

Keep the committed autopilot trail audit, correct skill lock, and constraint-comment procedure. Restore the run-as-user landing gate. Prohibit agent attribution in the floor, per the subsequent human correction. Audit Go comment constraints, encode missing enforcement, and remove explanatory Go comments while preserving compiler directives. Publish together in one PR. Skip the build gap.

## Decisions

| Decision | Instead of | By | Why | Evidence |
| --- | --- | --- | --- | --- |
| Keep the run gate | Remove it | human | Latest request restores it | November 15 thread |
| Preserve go:embed | Delete executable directive | agent | Required to embed shipped assets | assets.go |
| Prohibit agent attribution | Automatic Assisted-by trailers | human | Corrected intended attribution policy | Follow-up in live thread |

## State and next action

Existing commits f97d6c9, 863af5e, and 0bfb135 hold the first three changes. The restored gate, attribution floor, and comment removal are complete. All 1,396 inventory entries were audited; three installer fixtures now enforce regular-file output, setup-before-reconciliation ordering, and flat real skill directories. Removed 1,395 comments and retained go:embed. Scanner token hashes matched before and after removal across all tracked Go files.

Checks passed on the implementation committed with this document: `go vet ./...`, `test -z "$(gofmt -l .)"`, `go test ./...`, `go build ./...`, `git diff --check`, and `git diff --cached --check`. Targeted gate and installer tests also passed. No live behavioral eval or run-as-user verdict is available. Installation was not run. Next: publish one PR and obtain the configured medium review; the run gate remains a landing hold without a verify-app skill or explicit human acceptance.

Attribution correction: the floor now prohibits agent attribution in commits and PR descriptions. The earlier automatic-attribution interpretation is superseded. Existing published history is preserved. Independent medium review remains unavailable after the configured Claude provider hit its API rate limit.
