export const title = "Agent lifecycle reconciler";
const word = { ready: "ready", progress: "in progress", human: "PR ready", blocked: "blocked" };
const issue = (v, c) => v.state[c] === "closed" ? (v.merged[c] ? "merged" : "closed by hand") : word[v.label[c]];
export const state = (v) => ({
  "Ticket A": issue(v, "A"),
  "Ticket B": issue(v, "B"),
  Parent: v.state.P === "closed" ? "closed" : v.plabel === "ready" ? "open, ready for agent" : "open",
  Workflow: v.running[1] === "-" ? "idle" : v.pending[1] === "-" ? "one run" : "run plus one pending",
});
export const step = (action) => ({
  Start: "an agent starts", PrReady: "the PR is ready", MergeClose: "a PR merges and closes it",
  HandClose: "someone closes it by hand", Reopen: "someone reopens it", Run: "the workflow runs",
}[action] ?? action);
