---- MODULE Reconciler ----
\* Model of .github/scripts/reconcile-agent-lifecycle.go and .github/workflows/agent-lifecycle.yml.
\* Parent P has children A and B; B is blocked by A. Each workflow run is one atomic step that reads live state.
\* The workflow's concurrency group keeps one running and at most one pending run: a newer event replaces the pending one.
\* Closes and reopens made by the reconciler use GITHUB_TOKEN, so they start no workflow run.
EXTENDS Naturals

Children == {"A", "B"}
Issues == Children \cup {"P"}
None == <<"-", "-">>

VARIABLES state, merged, label, plabel, running, pending
vars == <<state, merged, label, plabel, running, pending>>

Satisfied(st, c) == st[c] = "closed" /\ merged[c]
Quiet == running = None /\ pending = None

Init ==
  /\ state = [i \in Issues |-> "open"]
  /\ merged = [c \in Children |-> FALSE]
  /\ label = [c \in Children |-> IF c = "B" THEN "blocked" ELSE "ready"]
  /\ plabel = "progress"
  /\ running = None
  /\ pending = None

Enqueue(e) ==
  IF running = None THEN running' = e /\ pending' = pending
  ELSE running' = running /\ pending' = e

\* People and agents.
Start(c) == /\ state[c] = "open" /\ label[c] = "ready"
            /\ label' = [label EXCEPT ![c] = "progress"]
            /\ UNCHANGED <<state, merged, plabel, running, pending>>
PrReady(c) == /\ state[c] = "open" /\ label[c] = "progress"
              /\ label' = [label EXCEPT ![c] = "human"]
              /\ UNCHANGED <<state, merged, plabel, running, pending>>
MergeClose(c) == /\ state[c] = "open"
                 /\ state' = [state EXCEPT ![c] = "closed"]
                 /\ merged' = [merged EXCEPT ![c] = TRUE]
                 /\ Enqueue(<<c, "closed">>)
                 /\ UNCHANGED <<label, plabel>>
HandClose(i) == /\ state[i] = "open"
                /\ state' = [state EXCEPT ![i] = "closed"]
                /\ Enqueue(<<i, "closed">>)
                /\ UNCHANGED <<merged, label, plabel>>
Reopen(i) == /\ state[i] = "closed"
             /\ state' = [state EXCEPT ![i] = "open"]
             /\ Enqueue(<<i, "reopened">>)
             /\ UNCHANGED <<merged, label, plabel>>

\* reconcileFrontier for a child, given labels l and state st.
Frontier(l, c, reset, st) ==
  IF st[c] # "open" THEN l[c]
  ELSE IF c = "B" /\ ~Satisfied(st, "A") THEN "blocked"
  ELSE IF reset \/ l[c] \in {"blocked", "ready"} THEN "ready"
  ELSE l[c]

\* reconcileFrontier for the parent: it carries only parent labels.
PFrontier(pl, reset, st) ==
  IF st["P"] # "open" \/ pl = "none" THEN pl
  ELSE IF reset \/ pl = "ready" THEN "ready"
  ELSE pl

AllSatisfied(st) == \A c \in Children : Satisfied(st, c)

\* One workflow run for the event at the head.
Run ==
  /\ running # None
  /\ LET n == running[1]
         reset == running[2] = "reopened"
         l1 == IF n \in Children THEN [label EXCEPT ![n] = Frontier(label, n, reset, state)] ELSE label
         \* Every run in the tree reconciles the parent, which now recomputes each child's frontier.
         l2 == [c \in Children |-> Frontier(l1, c, FALSE, state)]
         \* A reopened parent no longer resets like a ticket.
         p1 == IF n = "P" THEN PFrontier(plabel, FALSE, state) ELSE plabel
         target == IF AllSatisfied(state) THEN "closed" ELSE "open"
     IN /\ label' = l2
        /\ state' = [state EXCEPT !["P"] = target]
        /\ plabel' = IF target = "closed" THEN "none" ELSE IF p1 = "none" THEN "progress" ELSE p1
  /\ running' = pending
  /\ pending' = None
  /\ UNCHANGED merged

Next ==
  \/ \E c \in Children : Start(c) \/ PrReady(c) \/ MergeClose(c)
  \/ \E i \in Issues : HandClose(i) \/ Reopen(i)
  \/ Run

\* Rules, checked once no workflow run is running or pending.
ParentClosesOnlyWhenAllMerged == Quiet /\ state["P"] = "closed" => AllSatisfied(state)
ParentClosesWhenAllMerged == Quiet /\ AllSatisfied(state) => state["P"] = "closed"
BlockedExactlyWhileBlockerUnmerged ==
  Quiet /\ state["B"] = "open" => (label["B"] = "blocked" <=> ~Satisfied(state, "A"))
ParentNeverReadyForAgent == plabel # "ready"
====
