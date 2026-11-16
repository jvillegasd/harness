# Tracker reconciliation

For ticket-bound work only, after the merge is confirmed and `harness land sync` has updated the default branch. The landing request covers these scoped writes: name and verify the tickets and transitions without asking again. A transition outside that scope is reported and stops only the cleanup that depends on it.

The transitions are two: the ticket closes, and its parent closes once every child has merge evidence. A closing reference in the pull request closes the ticket on GitHub; confirm it did, and close it by hand when it did not. Close a parent by hand only when every required child is closed with merge evidence. Unblocked siblings need no write: the tracker's native blocked-by state already shows them free. Say which transitions you made by hand.
