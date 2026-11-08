// Managed by the harness: adds the branch, its head commit, and the task document's next action to OpenCode's compaction prompt, so the summary keeps them.
export const HarnessResume = async ({ $, directory }) => ({
  "experimental.session.compacting": async (_input, output) => {
    try {
      const text = await $`${process.env.HOME}/.local/bin/harness resume`.cwd(directory).quiet().nothrow().text()
      if (text.trim()) output.context.push(text)
    } catch {
      // A missing or failing harness must never block compaction.
    }
  },
})
