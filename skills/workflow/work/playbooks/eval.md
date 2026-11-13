# Eval

**You own the experiment.** Before a skill, prompt, or instruction change ships, blinded runs settle whether it changes agent behavior; after it ships, `harness bench` measures its cost and friction in real sessions. Reading the diff or the prose is evidence of neither.

Blinding holds for every step:

- Everything a candidate sees (directory and file names, the prompt, planted files) looks like an ordinary project and an ordinary request. Words such as eval, test, judge, rubric, score, benchmark, candidate, or compare appear nowhere in it.
- The candidate is asked for the work and, at most, its design notes. It is never asked which skills, principles, or files it used, and never told other candidates exist.
- The judge knows it is judging, and sees outputs only by a neutral label, never by variant or model.

1. **Frame.** Name the variant under test, the baseline it competes with (usually the current version), and the behavior that counts as success. Write a rubric of three to six concrete criteria for the judge alone. State the runs and models you plan, and get the human's go before step 4, because live runs spend a model budget the human approves.
2. **Plant the environments.** One directory per candidate under `~/.scratch-projects/`, which `harness bench` leaves out of its population, each with an ordinary project name and holding a realistic project skeleton and the context an organic task would have. Plant the variant or the baseline where the candidate's client loads skills from its working directory (`.claude/skills/` or `.agents/skills/`), never by installing over the human's own skills.
3. **Write one organic prompt,** the request a person would type, and give every candidate the same one.
4. **Run the candidates** as headless client sessions started inside their own directories (`claude -p`, `codex exec`, or `opencode run`), in a mode that lets it edit only that directory (Claude Code's `--permission-mode acceptEdits`, Codex's `--sandbox workspace-write`, or OpenCode's permission rules in that directory's `opencode.json`), because only a session started there loads its planted skills; a subagent shares its parent's directory. Use at least two runs per arm and at least two models across them. Confirm from the first run's record that it loaded the planted copy, not an installed skill of the same name; when the installed copy wins, stop and report it, since the arms would not differ. Done when every run has finished or failed, with the failures named.
5. **Judge blind.** One judge, on a model family no candidate used and launched as `delegation` describes for a subagent with no role, scores every output in a single pass against the rubric. When no other family can launch, say so and mark the verdict single-family.
6. **Check behavior from the record.** Read each candidate's session file, the one its client wrote for that candidate's directory, never another project's sessions. Grade which files it actually opened and the shape of what it produced, not its own account.
7. **Read every output yourself** and compare with the judge. A disagreement means a biased judge or an ambiguous rubric: name which, fix it, and rerun the judge. Vet the gap with `principle-explain-the-number` before you trust it.
8. Delete the scratch directories.

The eval changes no tracked file. When the verdict is to promote the variant, that change is a new task, matched again.

**Reply:** the variant and baseline, the rubric, each run's notes by label with the label key, the judge's verdict, your own read, and whether to promote the variant.
