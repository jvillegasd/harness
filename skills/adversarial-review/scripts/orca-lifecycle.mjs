import { execFileSync } from 'node:child_process';

// Loaded only into a read-only role's session by launch-args.
export const OrcaLifecycle = async () => ({
  config: async (config) => {
    const role = process.env.HARNESS_LIFECYCLE_ROLE;
    if (!['reviewer-a', 'reviewer-b', 'review-refuter', 'code-search'].includes(role)) {
      throw new Error('Missing read-only role identity');
    }
    const agent = config.agent?.[role];
    if (!agent) throw new Error('Missing named role');
    // Grant Bash only when this plugin and its blocking hook have loaded.
    agent.permission = { ...agent.permission, bash: 'allow' };
  },
  'tool.execute.before': async (input, output) => {
    if (input.tool !== 'bash') return;
    const guard = process.env.HARNESS_LIFECYCLE_GUARD;
    if (!guard) throw new Error('Missing lifecycle permission guard');
    const result = JSON.parse(execFileSync(guard, ['models', 'lifecycle-guard'], {
      input: JSON.stringify({ tool_name: 'Bash', tool_input: output.args }),
      encoding: 'utf8',
      timeout: 5000,
    }));
    if (result.hookSpecificOutput?.permissionDecision !== 'allow') {
      throw new Error('Only literal Orca lifecycle commands from the live worker preamble are permitted');
    }
  },
});
