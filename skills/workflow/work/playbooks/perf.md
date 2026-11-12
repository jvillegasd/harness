# Perf

**You own the measurement.** Every fix is tied to a number; reading the source is not measuring.

1. Capture a baseline on the surface where the slowness shows up (the project's `verify-<app>` skill, `journey-verification`, or a benchmark), and vet it and every later number with `principle-explain-the-number`.
2. Ground hypotheses with `how`, and claim no ceiling you have not run. Try the mantras cheapest first, and stop at the first one that meets the target:
   1. Don't do it: stop work whose result nothing uses.
   2. Do it, but don't do it again.
   3. Do it less.
   4. Do it later.
   5. Do it when they're not looking.
   6. Do it concurrently.
   7. Do it cheaper.
3. Plan the fix from the trace. When it crosses a function boundary, run [design](design.md)'s step 2 first. Implement inline, since the fix depends on reading the trace, and verify each attempt before the next (`principle-sequence-verifiable-units`).
4. Capture the post-fix measurement on the same surface and compare the artifacts mechanically.
5. Finish, as work mode's Finish lists, with the measurement in the PR's evidence.

Sustained improvement against a metric, rather than a one-off fix, is autopilot with the metric as its definition of done.

**Reply:** the baseline number, the post-fix number, the change, and the artifact paths.
