# Publishing tickets

Read when the human asked to publish the drafted tickets. An explicit request to create tickets in a named tracker covers the scoped writes without another approval.

1. Settle the granularity and the blocking edges with the human before any write, unless they agreed this breakdown earlier in the conversation, because rewriting a published graph is costly.
2. Resolve the tracker from `docs/agents/harness.json` and list the exact write set. For a file-backed tracker, read `local-tracker` from its path: it owns the published layout and the state mapping.
3. Publish blockers before dependents, so every edge points at a ticket that exists, and rewrite each draft's Blocked by from draft numbers to the published blockers' identifiers. Use native sub-issue and dependency relationships where the tracker has them, and textual edges otherwise. A GitHub `blocked_by` edge takes the blocker's database id (`gh api repos/<owner>/<repo>/issues/<n> --jq .id`), not its number or node id.
4. Keep the identifiers and unrelated metadata of tickets you update. Apply a ready label only to a ticket whose scope is settled and whose blockers are satisfied, so planning items stay outside the ready frontier.
5. Read back every write. Done when each published ticket matches its draft and every edge resolves to a published ticket.
6. After a partial failure, report what was created and what remains, and read the tracker's actual state before retrying, so the retry creates no duplicate.
