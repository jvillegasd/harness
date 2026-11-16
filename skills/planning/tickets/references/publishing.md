# Publishing tickets

Read when the human asked to publish the drafted tickets. An explicit request to create tickets in a named tracker covers the scoped writes without another approval.

1. Settle the granularity and the blocking edges with the human before any write, unless they agreed this breakdown earlier in the conversation, because rewriting a published graph is costly.
2. Resolve the tracker from `docs/agents/harness.json`. For a tracker other than GitHub, create the issues with its own tools, one per draft with its blocking edges, and remove each draft once its issue reads back, so one copy remains.
3. For GitHub, run `harness tickets publish <feature-slug> --dry-run` and show its write set, then `harness tickets publish <feature-slug>`. It owns the publish order, the native dependency edges, the read-back, the removal of matched drafts, and the safe rerun after a partial failure. When it stops, report the created and remaining lists it prints, and rerun it once the cause is fixed rather than creating issues by hand.
4. Keep the identifiers and unrelated metadata of tickets you update. The command creates new issues only; an update to an existing ticket is done by hand.
5. Done when every published ticket matches its draft and every edge resolves to a published ticket; a draft the command kept is reported with its mismatch.
