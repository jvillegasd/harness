# Eval

**You own the experiment.** A skill, prompt, or instruction change is a claim about agent behavior, and only blinded runs settle it. Reading the diff or the prose is not evidence.

Blinding holds for every step:

- Everything a candidate sees (directory and file names, the prompt, planted files) looks like an ordinary project and an ordinary request. Words such as eval, test, judge, rubric, score, benchmark, candidate, or compare appear nowhere in it.
- The candidate is asked for the work and, at most, its design notes. It is never asked which skills, principles, or files it used, and never told other candidates exist.
- The judge knows it is judging, and sees outputs only by a neutral label, never by variant or model.

1. **Frame.** Name the variant under test, the baseline it competes with (usually the current version), and the behavior that counts as success. Write a rubric of three to six concrete criteria for the judge alone.
2. **Plant the environments.** One scratch directory per candidate, outside the repository, holding a realistic project skeleton and the context an organic task would have. Plant the variant or the baseline where the candidate's client loads skills from its working directory (`.claude/skills/` or `.agents/skills/`), never by installing over the human's own skills.
3. **Write one organic prompt,** the request a person would type, and give every candidate the same one.
4. **Run the candidates** as fresh subagents, each in its own directory, with at least two runs per arm and at least two models across them, launched as `delegation` describes for a subagent with no role. Done when every run has finished or failed, with the failures named.
5. **Judge blind.** One judge, on a model family no candidate used, scores every output in a single pass against the rubric. When no other family can launch, say so and mark the verdict single-family.
6. **Check behavior from the record.** Read each candidate's transcript (its subagent session record, or `t3_thread_read` on a T3 Code delegate's thread), never another project's sessions. Grade which files it actually opened and the shape of what it produced, not its own account.
7. **Read every output yourself** and compare with the judge. A disagreement means a biased judge or an ambiguous rubric: name which, fix it, and rerun the judge. Vet the gap with `principle-explain-the-number` before you trust it.
8. Delete the scratch directories.

The eval changes no tracked file. When the verdict is to promote the variant, that change is a new task, matched again.

**Reply:** the variant and baseline, the rubric, each run's notes by label with the label key, the judge's verdict, your own read, and whether to promote the variant.
