---
name: why
description: Investigate why code or a design took its present shape, including decisions, tradeoffs, regressions, and historical constraints. Use how for runtime flow.
---

# Why

Investigate the rationale behind the target. Code establishes what exists, not author intent. Read [epistemics](references/epistemics.md) before synthesizing; keep direct evidence, supported inference, speculation, contradiction, and unknowns distinct.

1. Anchor the question in the relevant files, symbols, and history. Use blame and history through renames as needed, then follow relevant commits, PRs, tickets, and their discussions. A recent change may not be the origin of the decision.
2. Discover available sources and search those proportionate to the question. The [source playbooks](references/source-playbook.md) cover history, tickets, documents, chat, observability, errors, analytics, and incidents. Follow useful leads across sources, read context around findings, and record searches that returned nothing or could not be made. Widen the search when the evidence is thin or the question demands it.
3. Cite the evidence for each material claim. Surface contradictions and alternative readings, show the inference chain, and identify what remains unknown and where you looked. Treat a premise in the question as a hypothesis to check. Calibrate conclusions to the evidence, including retention, access, and correlation limits.
4. If the answer informs a change, identify the constraints the evidence suggests preserving, changing, or investigating further.

Work inline by default. Delegate a bounded question through a configured Harness role when it has a named benefit and the role can access the source. Verify returned citations before relying on them.
