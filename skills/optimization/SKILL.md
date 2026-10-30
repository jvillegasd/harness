---
name: optimization
description: "Trigger: optimize performance, reduce latency, improve throughput, optimize a webapp. Measure bottlenecks, optimize, and verify before/after results."
license: MIT
metadata:
  author: jvillegasd
  version: "1.0"
---

## Activation Contract

Use for measured performance improvements in any codebase. Default to measuring and implementing; honor analysis-only requests. Select only areas present in the target system.

## Hard Rules

- Measure before and after every optimization. Code inspection identifies hypotheses, not proven bottlenecks or gains.
- Preserve behavior, accessibility, and data correctness. Do not add caching, memoization, concurrency, or lazy loading without evidence that it addresses the measured problem.
- Discover available tools; never assume a particular MCP, profiler, or production telemetry service exists. Do not install tools or configure external services automatically.
- When measurement is unavailable, report the gap and keep affected claims unverified. Continue independently measurable work; do not retain speculative optimizations as proven improvements.

## Decision Gates

| Applicable surface | Investigate when evidence points here |
| --- | --- |
| Any codebase | Repeated computation, algorithms, CPU, allocations, memory retention, startup, and I/O. |
| Networked code | Redundant requests, waterfalls, payload size, batching, concurrency, and caching with correct freshness, invalidation, and user isolation. |
| Database-backed code | Slow queries, execution plans, indexes, and N+1 query counts as input grows. |
| Webapps | Unnecessary renders, initial and deferred bundles, lazy loading, image sizing/loading, fonts, and blocking or unused scripts. Check deferred interactions as well as initial load. |

## Execution Steps

1. Identify the target workload or real user flow and the metric that matters. Inspect project commands, benchmarks, profilers, instrumentation, and exposed MCP tool descriptions. Choose by capability: CPU/memory profiling, network tracing, query analysis, browser measurement, flow automation, or production telemetry. Chrome DevTools and Playwright are examples, not prerequisites; prefer available native tools when sufficient.
2. Capture a reproducible baseline before editing. Record revision and local changes, command or flow, dataset, build mode, environment, and cold/warm cache state. For webapps, include device and network conditions and use production builds for bundle comparisons. Repeat measurements enough to characterize variability; never infer a gain from one noisy run.
3. Rank measured bottlenecks by impact on the target metric. Apply the smallest supported change to one bottleneck at a time; avoid shifting cost into another critical path.
4. Rerun the same workload under comparable conditions after each change. Report sample count, the same statistic before/after, absolute and relative differences, and variability. Keep local measurements separate from production observations. Check relevant tradeoffs such as latency versus memory, request count versus payload size, and startup versus first interaction.
5. Run relevant correctness checks and real flows. Revert only the optimization's own changes when it regresses behavior or repeated measurements show no benefit. Treat noisy results as inconclusive; investigate or revert rather than claiming success.

## Output Contract

Report tools discovered and used, scenario and measurement method, each change's before/after results with evidence locations, correctness checks, reverted experiments, and remaining gaps. Name MCP tools by server and bare tool name. Do not claim production impact from local measurements or create report files unless requested or needed for recovery.
