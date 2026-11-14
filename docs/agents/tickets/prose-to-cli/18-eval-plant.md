# 18 Plant and clean blinded eval environments

## What to build

`harness eval plant --variant <skill dir> --baseline <skill dir> --client <claude|codex|opencode> --runs <n> [--skeleton <dir>]` builds the directories a blinded skill comparison runs in. For each arm and run it creates one directory under the scratch-projects root with an ordinary project name, copies the skeleton into it, and plants that arm's skill where the client loads skills from its working directory. Nothing a candidate can see says it is being compared: no directory, file, or planted text added by the command contains the words the eval playbook bans.

It prints a run id and writes two files outside every candidate directory: the label key, mapping each neutral label to its arm, and a manifest of the directories. The judge is given labels and never the key.

`harness eval clean <run id>` deletes that run's directories and nothing else.

The `eval` playbook's planting step and its final cleanup call these commands.

## Acceptance criteria

- `plant --runs 2` creates four directories under the scratch-projects root, two per arm, each holding the skeleton and its arm's skill at the client's working-directory skill path.
- No directory or file name the command creates inside a candidate directory contains eval, test, judge, rubric, score, benchmark, candidate, or compare.
- The label key maps every label to exactly one arm and is not inside any candidate directory.
- The human's installed skills are untouched.
- Planting the same skill name for `claude` and for `codex` puts it at each client's own path.
- `clean` removes every directory in the run's manifest, the key, and the manifest, and leaves other directories under the scratch-projects root alone.
- `clean` with an unknown run id exits non-zero and removes nothing.
- A scratch-projects root set through the environment is honored, so tests never touch the real one.
- The `eval` playbook names the commands for planting and cleanup.

## Seam

A small package's tests over a temporary scratch root settle planting and cleaning; one command test in `cmd/harness` proves the wiring.

## Blocked by

none

## Out of scope

Launching a client (ticket 19). Writing the prompt or the rubric. Judging.
