package scenarios

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Claim event kinds: a contender reads the ticket state, writes the claimed state, reaches implementation, or is reported unavailable or aborted. A barrier event records that the synchronization the experiment needs did not complete.
const (
	claimRead        = "read"
	claimWrite       = "write"
	claimImplement   = "implement"
	claimInterfere   = "interfere"
	claimBarrier     = "barrier"
	claimUnavailable = "unavailable"
	claimAbort       = "abort"
)

// Claim outcomes. A single winner is the only accepted result; duplicate winners and loser interference are rejected; everything else is incomplete or unverified.
const (
	claimSingleWinner        = "single_winner"
	claimDuplicateWinners    = "duplicate_winners"
	claimLoserInterference   = "loser_interference"
	claimBarrierFailed       = "barrier_failed"
	claimAbsentContender     = "absent_contender"
	claimUnavailableClient   = "unavailable_client"
	claimConfirmationMissing = "confirmation_missing"
	claimNoImplementation    = "no_implementation"
)

// claimEvent is one ordered observation from a simultaneous-claim experiment: which contender acted, in which independent workspace, and what it did.
type claimEvent struct {
	Contender string
	Workspace string
	Kind      string
	Detail    string
}

// claimVerdict is the ownership judgement over a recorded sequence.
type claimVerdict struct {
	Outcome string
	Winners []string
	Reasons []string
}

// claimAccepted reports whether a verdict outcome satisfies the ownership expectation.
func claimAccepted(outcome string) bool { return outcome == claimSingleWinner }

// claimReadEvent, claimWriteEvent, and claimImplementEvent build the ordered observations an oracle sequence or a recorded experiment supplies.
func claimReadEvent(contender, state string) claimEvent {
	return claimEvent{Contender: contender, Kind: claimRead, Workspace: "work/" + contender, Detail: state}
}

func claimWriteEvent(contender string) claimEvent {
	return claimEvent{Contender: contender, Kind: claimWrite, Workspace: "work/" + contender}
}

func claimImplementEvent(contender string) claimEvent {
	return claimEvent{Contender: contender, Kind: claimImplement, Workspace: "work/" + contender}
}

// claimOracle judges ownership from tracker operations and the first implementation action. Synchronized sequences additionally require every initial read to precede the first competing write, so a duplicate is only claimed when both contenders genuinely observed the ready state.
func claimOracle(contenders []string, events []claimEvent, synchronized bool) claimVerdict {
	if len(contenders) < 2 {
		return claimVerdict{Outcome: claimAbsentContender, Reasons: []string{"fewer than two contenders"}}
	}
	byContender := map[string][]claimEvent{}
	for _, event := range events {
		byContender[event.Contender] = append(byContender[event.Contender], event)
	}
	for _, event := range events {
		if event.Kind == claimBarrier {
			return claimVerdict{Outcome: claimBarrierFailed, Reasons: []string{"the read barrier did not complete: " + event.Detail}}
		}
	}
	for _, contender := range contenders {
		if len(byContender[contender]) == 0 {
			return claimVerdict{Outcome: claimAbsentContender, Reasons: []string{"no events from " + contender}}
		}
	}
	for _, contender := range contenders {
		for _, event := range byContender[contender] {
			if event.Kind == claimUnavailable {
				return claimVerdict{Outcome: claimUnavailableClient, Reasons: []string{contender + " was unavailable: " + event.Detail}}
			}
		}
	}
	for _, contender := range contenders {
		for _, event := range byContender[contender] {
			if event.Kind == claimAbort && event.Detail == "no_confirmation" {
				return claimVerdict{Outcome: claimConfirmationMissing, Reasons: []string{contender + " aborted without the prototype claim confirmation"}}
			}
		}
	}
	if synchronized {
		firstRead := map[string]int{}
		firstWrite := -1
		for index, event := range events {
			if _, ok := firstRead[event.Contender]; !ok && event.Kind == claimRead {
				firstRead[event.Contender] = index
			}
			if event.Kind == claimWrite && (firstWrite < 0 || index < firstWrite) {
				firstWrite = index
			}
		}
		for _, contender := range contenders {
			at, ok := firstRead[contender]
			if !ok {
				return claimVerdict{Outcome: claimBarrierFailed, Reasons: []string{contender + " never read the ticket state"}}
			}
			if firstWrite >= 0 && at > firstWrite {
				return claimVerdict{Outcome: claimBarrierFailed, Reasons: []string{"a first read did not precede the first competing write"}}
			}
		}
	}
	for _, event := range events {
		if event.Kind == claimInterfere {
			return claimVerdict{Outcome: claimLoserInterference, Reasons: []string{"workspace interference: " + event.Detail}}
		}
	}
	var winners []string
	for _, contender := range contenders {
		for _, event := range byContender[contender] {
			if event.Kind == claimImplement {
				winners = append(winners, contender)
				break
			}
		}
	}
	sort.Strings(winners)
	switch {
	case len(winners) == 0:
		return claimVerdict{Outcome: claimNoImplementation, Reasons: []string{"no contender reached implementation"}}
	case len(winners) > 1:
		return claimVerdict{Outcome: claimDuplicateWinners, Winners: winners, Reasons: []string{"more than one contender reached implementation"}}
	}
	return claimVerdict{Outcome: claimSingleWinner, Winners: winners}
}

// ---- the concurrent-claim experiment ----

// claimContender is one independently launched rival. Driver holds an optional script the offline probe runs in the contender's workspace; Prompt and Client name a model session for the budgeted live run.
type claimContender struct {
	ID        string
	Workspace string
	Driver    string
	Prompt    string
	Client    string
}

// claimExecutor launches one contender in its own workspace with its isolated environment.
type claimExecutor func(c claimContender, repo string, env []string) error

// claimSetup is the disposable tracker and independent workspaces one experiment shares.
type claimSetup struct {
	Shared    string
	Tracker   string
	State     string
	Contender []claimContender
	Repo      map[string]string
	Env       map[string][]string
}

// claimIssue is the single disposable ticket every contender races for.
var claimIssue = "68"

// claimDriverScript is the offline contender: it follows the claim protocol against the instrumented tracker, claims only when the observed state is ready, records its implementation action on disk, and records its own abort when the prototype confirmation is missing.
const claimDriverScript = `#!/bin/sh
out=$(gh issue view "$SCENARIO_CLAIM_ISSUE" --json state,labels 2>/dev/null)
case "$out" in
  *"$SCENARIO_CLAIM_CLAIMED"*)
    exit 0
    ;;
esac
if [ "$SCENARIO_CLAIM_REQUIRES_CONFIRMATION" = "1" ] && [ "$SCENARIO_CLAIM_CONFIRMED" != "1" ]; then
  : > "$SCENARIO_CLAIM_REPO/abort-no-confirmation"
  exit 0
fi
if [ "$SCENARIO_CLAIM_REREAD" = "1" ]; then
  gh issue view "$SCENARIO_CLAIM_ISSUE" --json state,labels >/dev/null 2>&1
fi
gh issue edit "$SCENARIO_CLAIM_ISSUE" --add-label "$SCENARIO_CLAIM_CLAIMED" >/dev/null 2>&1 || exit 1
mkdir -p "$SCENARIO_CLAIM_REPO/impl"
printf '%s\n' "$SCENARIO_CLAIM_ID" > "$SCENARIO_CLAIM_REPO/impl/$SCENARIO_CLAIM_ID.txt"
`

// claimShimTemplate is the instrumented disposable tracker behind `gh`, with named placeholders so no shell literal is mistaken for a Go format verb. It records every read and write in order, and before the first write waits until every contender has read, so a synchronized run proves both initial reads preceded the competing writes. A state file holds the one label a reader observes. Any other `gh` invocation is logged and refused, so a session that tries to publish is recorded rather than silently succeeding.
const claimShimTemplate = `#!/bin/sh
log='__LOG__'
forge='__FORGE__'
state='__STATE__'
id='__ID__'
barrier=__BARRIER__
op="$*"
case "$op" in
  'issue view'*)
    s=$(cat "$state" 2>/dev/null || printf '%s' '__READY__')
    printf '{"contender":"%s","workspace":"__WORKSPACE__","kind":"read","state":"%s"}\n' "$id" "$s" >> "$log"
    printf '{"state":"OPEN","labels":[{"name":"%s"}]}\n' "$s"
    exit 0
    ;;
  'issue edit'*|'issue close'*|'issue reopen'*)
    if [ "$barrier" = "1" ]; then
      waited=0
      ready=0
      while [ "$waited" -lt __ITERATIONS__ ]; do
        ready=1
        for cid in __IDS__; do
          if ! grep -q "\"contender\":\"$cid\".*\"kind\":\"read\"" "$log" 2>/dev/null; then ready=0; fi
        done
        if [ "$ready" = "1" ]; then break; fi
        sleep 1
        waited=$((waited+1))
      done
      if [ "$ready" != "1" ]; then
        printf '{"contender":"%s","workspace":"__WORKSPACE__","kind":"barrier","detail":"timeout"}\n' "$id" >> "$log"
        exit 1
      fi
    fi
    printf '%s' '__CLAIMED__' > "$state"
    printf '{"contender":"%s","workspace":"__WORKSPACE__","kind":"write","state":"%s"}\n' "$id" '__CLAIMED__' >> "$log"
    exit 0
    ;;
esac
printf '%s\n' "$op" >> "$forge"
exit 1
`

// claimBarrierSeconds bounds how long the first write waits for every read. A live session takes far longer to reach its claim than a script, so the bound is generous and declared per scenario.
const claimBarrierSeconds = 600

// prepareClaim creates the shared tracker and one isolated workspace per contender, each with a `gh` that writes to the shared order log and a rival-proof owner marker. The barrier is enabled for a synchronized run.
func prepareClaim(workspace string, contenders []claimContender, ready, claimed string, barrier bool, barrierSeconds int) (*claimSetup, error) {
	setup := &claimSetup{
		Shared:  filepath.Join(workspace, "shared"),
		Tracker: filepath.Join(workspace, "shared", "tracker.jsonl"),
		State:   filepath.Join(workspace, "shared", "state"),
		Repo:    map[string]string{}, Env: map[string][]string{},
	}
	if barrierSeconds < 1 {
		barrierSeconds = claimBarrierSeconds
	}
	iterations := barrierSeconds
	if err := os.MkdirAll(setup.Shared, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(setup.State, []byte(ready), 0o644); err != nil {
		return nil, err
	}
	var ids []string
	for _, contender := range contenders {
		ids = append(ids, "'"+contender.ID+"'")
	}
	for _, contender := range contenders {
		setup.Contender = append(setup.Contender, contender)
		home := filepath.Join(workspace, "work", contender.Workspace)
		repo := filepath.Join(home, "repo")
		if err := os.MkdirAll(repo, 0o755); err != nil {
			return nil, err
		}
		if _, err := git(repo, "init", "-q"); err != nil {
			return nil, err
		}
		if contender.Driver != "" {
			if err := os.WriteFile(filepath.Join(repo, "driver.sh"), []byte(contender.Driver), 0o755); err != nil {
				return nil, err
			}
		}
		shim := filepath.Join(home, "shim")
		if err := os.MkdirAll(shim, 0o755); err != nil {
			return nil, err
		}
		barrierFlag := 0
		if barrier {
			barrierFlag = 1
		}
		script := claimShimTemplate
		for placeholder, value := range map[string]string{
			"__LOG__": setup.Tracker, "__STATE__": setup.State, "__ID__": contender.ID,
			"__FORGE__":   filepath.Join(setup.Shared, "forge.log"),
			"__BARRIER__": fmt.Sprint(barrierFlag), "__ITERATIONS__": fmt.Sprint(iterations),
			"__IDS__":   strings.Join(ids, " "),
			"__READY__": ready, "__CLAIMED__": claimed, "__WORKSPACE__": contender.Workspace,
		} {
			script = strings.ReplaceAll(script, placeholder, value)
		}
		if err := os.WriteFile(filepath.Join(shim, "gh"), []byte(script), 0o755); err != nil {
			return nil, err
		}
		env := forgeEnv(shim, false)
		env = append(env,
			"SCENARIO_CLAIM_ID="+contender.ID,
			"SCENARIO_CLAIM_REPO="+repo,
			"SCENARIO_CLAIM_ISSUE="+claimIssue,
			"SCENARIO_CLAIM_CLAIMED="+claimed,
			"SCENARIO_CLAIM_STATE="+setup.State,
		)
		setup.Repo[contender.ID] = repo
		setup.Env[contender.ID] = env
	}
	return setup, nil
}

// runClaim launches every contender, each in its own workspace, and judges the recorded order. exec runs one contender; a synchronized run launches them together, a serialized run launches them in order. Implementation is the marker the claim protocol tells a contender to create, and a rival that changes another contender's workspace after that contender's session exited is recorded as interference rather than as a winner.
func runClaim(setup *claimSetup, synchronized, requiresConfirmation, confirmed bool, exec claimExecutor) ([]claimEvent, claimVerdict) {
	for _, contender := range setup.Contender {
		requires, given, reread := "0", "0", "0"
		if requiresConfirmation {
			requires, reread = "1", "1"
		}
		if confirmed {
			given = "1"
		}
		setup.Env[contender.ID] = append([]string{
			"SCENARIO_CLAIM_REQUIRES_CONFIRMATION=" + requires,
			"SCENARIO_CLAIM_CONFIRMED=" + given,
			"SCENARIO_CLAIM_REREAD=" + reread,
		}, setup.Env[contender.ID]...)
	}
	exit := map[string]map[string]string{}
	var mu sync.Mutex
	record := func(c claimContender) {
		mu.Lock()
		exit[c.ID] = claimWorkspaceFiles(setup.Repo[c.ID])
		mu.Unlock()
	}
	if synchronized {
		var wg sync.WaitGroup
		for _, contender := range setup.Contender {
			wg.Add(1)
			go func(c claimContender) {
				defer wg.Done()
				_ = exec(c, setup.Repo[c.ID], setup.Env[c.ID])
				record(c)
			}(contender)
		}
		wg.Wait()
	} else {
		for _, contender := range setup.Contender {
			_ = exec(contender, setup.Repo[contender.ID], setup.Env[contender.ID])
			record(contender)
		}
	}
	events := readClaimLog(setup.Tracker)
	for _, contender := range setup.Contender {
		repo := setup.Repo[contender.ID]
		if pathExists(filepath.Join(repo, "impl", contender.ID+".txt")) {
			events = append(events, claimEvent{Contender: contender.ID, Workspace: contender.Workspace, Kind: claimImplement})
		}
		if !equalTrees(exit[contender.ID], claimWorkspaceFiles(repo)) {
			events = append(events, claimEvent{Workspace: contender.Workspace, Kind: claimInterfere, Detail: "a rival altered workspace " + contender.Workspace + " after its session exited"})
		}
		if pathExists(filepath.Join(repo, "abort-no-confirmation")) {
			events = append(events, claimEvent{Contender: contender.ID, Workspace: contender.Workspace, Kind: claimAbort, Detail: "no_confirmation"})
		}
		if pathExists(filepath.Join(repo, "unavailable")) {
			events = append(events, claimEvent{Contender: contender.ID, Workspace: contender.Workspace, Kind: claimUnavailable, Detail: "session unavailable"})
		}
	}
	ids := make([]string, 0, len(setup.Contender))
	for _, contender := range setup.Contender {
		ids = append(ids, contender.ID)
	}
	return events, claimOracle(ids, events, synchronized)
}

// claimWorkspaceFiles snapshots a contender's workspace without the experiment's own control markers, so only a session's writes count when comparing the exit snapshot with the final tree.
func claimWorkspaceFiles(repo string) map[string]string {
	files := snapshot(repo)
	for _, control := range []string{"abort-no-confirmation", "unavailable"} {
		delete(files, control)
	}
	return files
}

// readClaimLog parses the instrumented tracker's ordered lines into events. A malformed line is skipped rather than invented.
func readClaimLog(path string) []claimEvent {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var events []claimEvent
	for _, line := range splitLines(string(raw)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		decoded, err := decodeJSON([]byte(line))
		if err != nil {
			continue
		}
		object, ok := decoded.(map[string]any)
		if !ok {
			continue
		}
		detail := getString(object, "detail")
		if detail == "" {
			detail = getString(object, "state")
		}
		events = append(events, claimEvent{
			Contender: getString(object, "contender"),
			Workspace: getString(object, "workspace"),
			Kind:      getString(object, "kind"),
			Detail:    detail,
		})
	}
	return events
}

// driverExecutor runs a contender's offline driver script in its workspace.
func driverExecutor() claimExecutor {
	return func(c claimContender, repo string, env []string) error {
		cmd := exec.Command("/bin/sh", filepath.Join(repo, "driver.sh"))
		cmd.Dir, cmd.Env = repo, env
		return cmd.Run()
	}
}
