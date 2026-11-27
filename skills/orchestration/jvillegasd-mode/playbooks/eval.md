# Eval

**You own the experiment design: plan, blind, run, synthesize.** Blinded runs settle whether a skill, prompt, or instruction change alters agent behavior. Reading the diff or the prose is not evidence of that.

Blinding holds for every step:

- Nothing you write for a candidate (a directory or file name, the prompt, a planted file other than the skill under test) reveals the experiment: none of eval, judge, experiment, rubric, score, benchmark, candidate, arena, test, or compare used to describe the run itself. The same words in their ordinary task sense stay ("fix the score display"), and the planted skill stays verbatim, since editing it changes the variant. Directory and slug names are ones a user might pick for a real project.
- The candidate's prompt reads as an ordinary user request: it states the goal, not the experiment.
- The candidate is asked for the work and, at most, design notes in general. It is never asked which skills, principles, or files it used, and never told other candidates exist; chain-following is graded from what it did, not from what it says.
- The judge knows it is judging but sees outputs only by a neutral label, never by variant or model. When two variants compete, one judge scores both sets in a single pass on one scale, blind to which set each came from.

1. **Frame.** Name the variant under test, the baseline it competes with (usually the current version), and the behavior that counts as success. Write a rubric of three to six concrete criteria for the judge alone. State the runs and models you plan in the reply, then run them.
2. **Set up the environments.** One ordinarily named working directory per candidate, outside the repository and never over the human's own skills, with its arm's skill planted where the client loads skills from that directory. Plant the context an organic task would have: a project skeleton, and the skills a candidate would naturally read.
3. **Write one organic prompt,** the request a person would type, and give every candidate the same one.
4. **Run the candidates in parallel,** on at least two models, with every arm getting the same models and the same number of runs on each, so a model difference cannot pass for a variant difference. Use at least two runs per model per arm. Start each as a session inside its own directory, since only a session started there loads its planted skills and a subagent shares its parent's directory. Confirm from every session's record that it loaded its own arm's planted copy, not an installed skill of the same name; a session that resolved another copy is excluded and reported, then rerun on the same model until the arms are balanced again. When a rerun still resolves the wrong copy, stop and report the eval as inconclusive, because the arms would not be comparable. A session that only dispatches, polls, or collects results runs as a `worker` at the `fast` tier through `delegation`.
5. **Judge blind.** One judge, a `worker` at the `high` tier on a model family no candidate used, launched through `delegation`, scores every output against the rubric. When no other family can launch, say so and mark the verdict single-family.
6. **Verify the chain from the record, not self-report.** Read each candidate's session file, the one its client wrote for that candidate's directory, never another project's sessions, since those hold the human's unrelated chats. Grade which files it actually opened and the shape of what it produced.
7. **Read every output yourself,** end to end, and compare with the judge. A disagreement means a biased judge or an ambiguous rubric: name which, fix it, and rerun the judge. Vet the gap with `principle-explain-the-number` before you trust it.
8. Delete the candidate directories you created.

The eval changes no tracked file. When the verdict is to promote the variant, that change is a new task, matched again.

**Reply:** the variant and baseline, the rubric, each candidate's notes by label with the label key, the judge's verdict, your own read, and whether to promote the variant.
