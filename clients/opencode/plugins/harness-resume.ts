// Managed by the harness: adds the branch, its head commit, and the task document's next action to OpenCode's compaction prompt, so the summary keeps them.
import { execFile } from "node:child_process"
import { homedir } from "node:os"
import { join } from "node:path"
import { promisify } from "node:util"

const run = promisify(execFile)

async function resume(cwd) {
  try {
    const { stdout } = await run(join(homedir(), ".local", "bin", "harness"), ["resume"], { cwd })
    return stdout.trim() ? stdout : ""
  } catch {
    // A missing or failing harness must never block compaction.
    return ""
  }
}

// OpenCode v1 calls server() and v2 calls setup().
export default {
  id: "harness-resume",
  server: async ({ directory }) => ({
    "experimental.session.compacting": async (_input, output) => {
      const text = await resume(directory)
      if (text) output.context.push(text)
    },
  }),
  setup: async (ctx) => {
    const hook = await ctx.session.hook("compaction", async (request) => {
      const text = await resume(ctx.location.directory)
      if (text) request.system.push({ type: "text", text })
    })
    return () => hook.dispose()
  },
}
