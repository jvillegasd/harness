---
name: evidence
description: Challenge a load-bearing claim with data instead of asserting it, starting with the query that could cancel the work.
---

# Evidence

Find out whether the premise is wrong before writing code.

## What deserves a query

Query only claims whose failure changes whether the work happens, its scope, or the fix location. Leave other claims out; evidence gathered for appearances costs time and changes nothing.

## Challenge protocol

Before querying, state a prediction the data could refute.

- Not "this error affects users" but "this error fires more than 10 times a day and has fired since the deploy of `<sha>`".
- Not "this endpoint is slow" but "p95 of `<endpoint>` exceeds 2s over the last 7 days".

Query and record `confirmed` or `refuted` with the number, or `no data` for an uninstrumented signal.

For a refuted claim, state the changed recommendation on its own line: dropped scope, a closed ticket, another root cause, or a smaller fix. If nothing changes, delete the claim as non-load-bearing.

## Kill query first

Before investigating a fix, run the query that could cancel the work:

- Is the error still occurring after the most recent deploy?
- Does any non-test user actually reach the affected step?
- Is the slow path ever called?

Lead with a kill result, such as "Zero occurrences in 30 days", rather than code analysis.

## Source map, by capability

Report and skip unconfigured capabilities; missing data neither blocks work nor permits invented numbers.

- **Error tracking.** Search issues for frequency and first/last seen; aggregate events by release, environment, and time; read issue details for stacks. Verify automated root-cause hypotheses in code before treating them as findings.
- **Product analytics.** Query affected people and funnel conversion. For what a user saw or did, prefer a replay confirming reproduction to event-count inference. Apply project test-account filters, and read current person properties from the source rather than inferring them from values captured at ingest.
- **Metrics, logs, and traces.** Query rates and latency percentiles, logs for confirmation and uninstrumented failures, traces for time spent, and incident/alert history for prior pages. Generate deep links with the tool's link generator instead of writing them by hand.
- **Database.** Verify tables, columns, and constraints before asserting they exist. Use row counts for blast radius, advisors for security/performance warnings on changed surfaces, and migration history before proposing migrations.
- **Tracker.** Search prior art and duplicates before calling something new.
- **Version history.** Use `git log` and `git blame` for behavior changes and accompanying work, release/deploy history to map telemetry changes to commits, and pull request discussion for rationale. Check regression claims against history, which every repository has.

## Keep the numbers honest

A number is only evidence if the reader can trace it. Give every number a source deep link and its timeframe, rendered as complete absolute URLs.

- Quote numbers exactly as read from a tool result, over the timeframe you queried. Changing the window changes the claim.
- For an empty result, write `no data in <source> for <window>` and keep the claim.
- If a query fails or times out, say so and leave the number unstated; an estimate would pass for data.
- Error counts do not give affected users, and a sample does not give an error rate, so state only what the query measured.

## Budget

Budget about three queries per claim. If unsettled after three, mark it `no data`, name the instrumentation needed, and move on.
