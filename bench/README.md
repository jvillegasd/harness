# bench

`harness bench` answers one question: **did a change to the harness move cost and friction in the sessions people actually ran?**

It compares real sessions from before a change with real sessions after it. Rereading the harness's prose and judging that it sounds better proves nothing, so this repository's `AGENTS.md` makes bench the measurement for a harness change. Bench does not judge quality. For that, a person still reads the transcripts.

### Why real sessions and not test tasks

Bench deliberately does not run a fixed set of synthetic tasks. Synthetic tasks can swing between identical runs, and they tend to leak into the fixture they are meant to test. Real sessions have their own problems: they arrive slowly and mix in variables no experiment controls. The `THIN` warning and the caveats below exist to handle those.

## Run it

```sh
harness bench --split <commit-date> --since <window-start>
```

A typical use: you fix a friction point in the harness, for example one that `retro` suggested. Write down which metric you expect to move and the date the fix landed. Later, run bench with `--split` set to that date.

Run it from inside the checkout. It reads work patterns from `bench/cohorts.json` there. That file is gitignored, so a fresh worktree has only the committed, empty `cohorts.example.json` until you copy your own in. Until a pattern matches, the report does not split sessions into work and personal.

| Flag | Meaning |
| --- | --- |
| `--split` | The commit date of the change, as `YYYY-MM-DD`. Sessions before it count as `before`, sessions on or after it as `after`. Required for the report, not for `--rows`. |
| `--since` | Ignore sessions older than this date. Use it to leave out a period when the harness looked nothing like today's. Sessions outside the window are dropped before anything is counted. |
| `--client` | Limit to `claude`, `codex`, or `opencode`. Repeatable. Repeating a client adds nothing, so no client is counted twice. |
| `--home` | Where to find session stores. Defaults to your home directory. Point it at any directory laid out like a home to read sessions captured from another machine. |
| `--rows` | Print one JSON object per session instead of the report, so you can see the spread behind a median. Honors `--since` and needs no `--split`. |

## What it measures

Bench counts eight things per session and reports the median across all sessions in a group.

| Metric | Kind | What it counts |
| --- | --- | --- |
| `turns` | Cost | Messages a person typed. Anything the client writes into its own transcript is not a turn (see below). |
| `tool_calls` | Cost | Tool calls the agent made. |
| `minutes` | Cost | Time from the first timestamped entry to the last. |
| `commands` | What the harness aims to move | Slash commands, including skills called as `/name`, so you can see whether people use explicit commands. |
| `delegation` | What the harness aims to move | Messages produced by the session's subagents. Agents should delegate only for a stated reason, so too much delegation shows up here. |
| `interrupts` | Friction | Times a person stopped a run. |
| `denials` | Friction | Times a person refused a tool call. |
| `errors` | Friction | Tool calls that failed. A refusal counts as a denial, never also as an error. |

**What does not count as a turn:** tool results, slash-command entries (in either form), `<task-notification>` messages, local-command output, `isMeta` notices, compaction summaries (`isCompactSummary`), injected `<system-reminder>` blocks, and interrupt markers. The client writes all of these, not the person.

**A note on `delegation`:** a role delegated through T3 Code's `delegate_task` runs as its own session, not as a subagent. It never shows up in `delegation`. Instead it joins the set of sessions that every median is taken over.

## Split work from personal sessions

A hard week at work moves every metric even when the harness has not changed. To keep that separate, copy `cohorts.example.json` to `cohorts.json` and list the projects that count as work:

```json
{ "work": ["acme", "some-client-repo"] }
```

- Each entry is a case-insensitive substring. Any project that matches no entry counts as personal.
- The report splits into work and personal only once a pattern matches something.
- `cohorts.json` is gitignored, because its whole job is to name clients or employers.

Clients record a project in different ways: as a slug with dashes, or as a `directory` column. Bench folds both the pattern and the project name by turning every run of `-` and `/` into one separator, so a single `acme` matches both forms. Name a single path segment, not a span that crosses a `/`.

## Where the sessions come from

**Claude Code** stores sessions in `~/.claude/projects/<slug>/<session>.jsonl`. It is the only client that records all eight metrics. Interrupts and denials come from matching `claudeInterrupt` and `claudeDenials` in `internal/bench/collect.go`.

**Codex** stores sessions in `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl` and `~/.codex/archived_sessions/`. Bench reads them from `~/.codex`, whatever `CODEX_HOME` says, just as the installer does.

- Turns are `user_message` events. Interrupts are turns aborted as `interrupted`. Denials are tool outputs that exactly match one of the sentences in `codexDenials`.
- Codex saves neither whether a tool call succeeded nor slash commands, so `errors` and `commands` are missing.
- Bench reads only plain JSONL rollouts, which is Codex's default. It cannot read a rollout compressed to `.jsonl.zst` or moved to paginated history. Both are optional Codex features that are off by default.
- A thread forked with its full history, by `/fork` or by a subagent started with its parent's context, copies that history into its own rollout. Those copied turns or items are counted again.

**OpenCode** stores sessions in `~/.local/share/opencode/opencode.db`. Bench opens it read-only and immutable, because a client may be running. OpenCode records no interrupts, denials, failed results, or slash commands, so those four are missing.

### Subagents count as delegation, not as sessions

Each client stores subagents next to their parent session:

- Claude Code writes `<session-id>/subagents/*.jsonl` beside the session file.
- OpenCode gives a subagent its own `session` row that carries the parent's id.
- Codex writes each subagent its own rollout, whose metadata names its parent thread.

A naive search would count all of these as sessions. Bench excludes them and counts their activity as the parent's `delegation` instead. For Codex, bench follows each subagent up to its top-level session. A Codex internal rollout with no parent, such as a guardian review, is dropped. Claude Code's inline `isSidechain` flag is always false, so bench cannot use it to spot a subagent.

A role delegated through T3 Code works the other way. It runs as a top-level session of its own, so it counts as a session and not as delegation. That has two effects: role work disappears from `delegation`, and every median shifts toward a worker's shorter transcript.

## Read the report

```text
split 2026-09-02  since 2026-08-01

claude  work
  metric          before     after     delta
  sessions            34        29
  turns              4.0       3.0      -1.0
  tool_calls        41.0      38.0      -3.0
  minutes            9.4       7.1      -2.3
  commands           1.0       2.0      +1.0
  delegation         0.0      12.0     +12.0
  interrupts         1.0       0.0      -1.0
  denials            0.0       0.0      +0.0
  errors             2.0       2.0      +0.0
```

Each heading is the client name, followed by `work` or `personal` once a work pattern matches a session in the window.

Two rules matter more than the numbers:

- **A metric a client does not record prints `-`, never `0`.** "Not measured" and "measured zero" are different claims. Mixing them up would invent evidence.
- **Below 20 sessions (`THIN = 20`), the report warns you.** With that few sessions, a difference in medians is not a finding.

Bench compares two groups of sessions, not matched pairs, and the groups differ in more than the change you are testing. Look for a real gap, not a small nudge.

## Share results safely

`harness bench` writes no files.

- **The report** names nothing and is safe to paste anywhere.
- **`--rows` output is not.** Each row includes the project the session ran in, which on a work machine names a client or employer.
- **`cohorts.json`** is gitignored for the same reason.
