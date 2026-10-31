---
name: optimization
description: "Trigger: optimize performance, reduce latency, improve throughput, optimize a webapp. Measure bottlenecks, optimize, and verify before/after results."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

Improve measured performance in any codebase. Measure and implement by default, and stay analysis-only when asked. Look only at the areas the target system actually has.

## Why

Code inspection produces hypotheses, not proven bottlenecks or gains, so every optimization is measured before and after. These principles keep the result honest:

- Preserve behavior, accessibility, and data correctness. Add caching, memoization, concurrency, or lazy loading only with evidence that it addresses the measured problem.
- Discover the tools that exist (a profiler, an MCP server, production telemetry) instead of assuming one. Installing tools or configuring external services needs its own request.
- When measurement is unavailable, report the gap and keep the affected claims unverified. Continue the independently measurable work, and do not keep a speculative optimization as a proven improvement.

## Where to look

| Applicable surface | Investigate when evidence points here |
| --- | --- |
| Any codebase | Repeated computation, algorithms, CPU, allocations, memory retention, startup, and I/O. |
| Networked code | Redundant requests, waterfalls, payload size, batching, concurrency, and caching with correct freshness, invalidation, and user isolation. |
| Database-backed code | Slow queries, execution plans, indexes, and N+1 query counts as input grows. |
| Webapps | Unnecessary renders, initial and deferred bundles, lazy loading, image sizing/loading, fonts, and blocking or unused scripts. Check deferred interactions as well as initial load. |

## Steps

1. Identify the target workload or real user flow and the metric that matters. Inspect project commands, benchmarks, profilers, instrumentation, and exposed MCP tool descriptions, and choose by capability: CPU/memory profiling, network tracing, query analysis, browser measurement, flow automation, or production telemetry. Chrome DevTools and Playwright are examples, not prerequisites; prefer available native tools when sufficient.
2. Capture a reproducible baseline before editing. Record the revision and local changes, command or flow, dataset, build mode, environment, and cold or warm cache state. For webapps, include device and network conditions and use production builds for bundle comparisons. Repeat measurements enough to characterize variability, since one noisy run proves no gain.
3. Rank the measured bottlenecks by impact on the target metric. Apply the smallest supported change to one bottleneck at a time, and watch that the cost does not shift onto another critical path.
4. Rerun the same workload under comparable conditions after each change. Report the sample count, the same statistic before and after, absolute and relative differences, and variability. Keep local measurements separate from production observations. Check the relevant tradeoffs, such as latency versus memory, request count versus payload size, and startup versus first interaction.
5. Run the relevant correctness checks and real flows. Revert only the optimization's own changes when it regresses behavior or repeated measurements show no benefit. Noisy results are inconclusive: investigate or revert instead of claiming success.

## Output

The tools discovered and used, the scenario and measurement method, each change's before and after results with evidence locations, the correctness checks, reverted experiments, and remaining gaps. Local measurements describe local conditions only; claim production impact only from production observations. Create report files only when asked or needed for recovery.
