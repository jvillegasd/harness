# 19 Run the planted candidates

## What to build

`harness eval run <run id> --prompt-file <file> --model <model>... [--dry-run]` starts one headless client session inside each planted directory, every one with the same prompt, in the mode that lets the client edit only that directory. It waits for all of them, then records for each label its exit status, where its output was saved, and where the client wrote that session's record.

`--dry-run` prints the exact command for every directory and starts nothing, so the human can approve the model budget before any run spends it.

A candidate that fails is recorded as failed and named; it does not stop the others. The outputs are saved by neutral label outside the candidate directories, so they can be handed to a judge without the key.

The `eval` playbook's run step calls this command and keeps the human's go, the check that the planted copy loaded, the judge, and reading the outputs.

## Acceptance criteria

- `--dry-run` prints one command per planted directory with that client's headless form and its edit-only-this-directory mode, and starts no process.
- Each client process is started with its working directory set to its own planted directory and receives the prompt file's exact text.
- With two models and two runs per arm, eight sessions are started, and each model appears in both arms.
- A client that exits non-zero is recorded as failed with its label, and the remaining sessions still run.
- Each label's output is saved outside every candidate directory, and the saved file names carry the label and not the arm.
- The run record names each session's record file for the client that wrote it.
- An unknown run id, or a missing prompt file, exits non-zero and starts nothing.
- The `eval` playbook names the command for its run step.

## Seam

Proposed, awaiting the human's confirmation: the eval package's tests start stand-in client programs placed on `PATH`, as the skills tests already do for `npx`, and assert each one's arguments, working directory, and input; one command test in `cmd/harness` proves the wiring. No test starts a real client.

## Blocked by

18

## Out of scope

Confirming from a session record that the planted skill loaded, which differs per client and stays with the agent. Launching the judge. A live run, which needs the human's budget approval and is behavioral acceptance, not a test.

Open question for the human, with the default the worker takes when unanswered: the headless flags are the ones the `eval` playbook names today. Default: take them from the playbook and verify each against the client version installed on the build machine before relying on it.
