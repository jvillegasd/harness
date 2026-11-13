# Forensics

**You own the diagnosis.** The deliverable is a cited cause for a runtime symptom (a leak, a spinning process, a stall, a glitch), never a fix. Read-only: no branch and no PR.

1. **Get the artifact.**
   - **Live process:** capture the signal on the surface where the symptom shows (the project's `verify-<app>` skill, `journey-verification`, or the CLI): a CPU profile for a spin, a heap snapshot for a leak, a trace for a glitch.
   - **Captured artifact** (a profile, trace, spindump, or heap snapshot handed to you): it is a fixed dataset, so read it and do not re-run it.
2. **Make it queryable** before you read it: load it into sqlite or a parser, one row per sample, frame, or node.
3. **Reduce it to the cause**: the frames holding the most time and the call path to them, the retainer chain from a leaked object to its root, the thread blocked and its wait reason, the loop firing with no input.
4. **Prove the mechanism.** For a live process, confirm it cheaply with injected instrumentation or a hot patch on the running process. For a captured artifact, diff it against a paired capture; with none, label the finding the strongest hypothesis the artifact supports, not a confirmed cause.
5. **Attribute it to source**: file, symbol, and the line that allocates or schedules. A frame with no source mapping is not yet a diagnosis: resolve the symbols, or say the artifact does not carry them.

When the cause is known and a fix is wanted, that is a new task, matched again as a bug fix or perf.

**Reply:** the artifact and its format, the reduced finding, how the mechanism was proved or that it is a hypothesis, the source location, and the artifact paths.
