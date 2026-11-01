// Command reconcile-agent-lifecycle reconciles native GitHub issue
// dependencies and parent/sub-issue state.
//
// It is one standard-library file so `go run` works in a repository with no Go
// module. setup-harness copies it, and the harness copy and the deployed copy
// must stay byte-identical; reconcile_agent_lifecycle_test.go fails when they
// drift.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// issue is the subset of the GitHub issue payload the reconciler reads.
type issue struct {
	Number        int             `json:"number"`
	State         string          `json:"state"`
	Labels        labelList       `json:"labels"`
	RepositoryURL string          `json:"repository_url"`
	PullRequest   json.RawMessage `json:"pull_request"`
	// Foreign marks an issue from another repository, such as a cross-repository sub-issue or blocker. Its number names a different issue here, and this repository's token cannot relabel it, so the reconciler reads its merge evidence from its own repository and never updates it.
	Foreign bool `json:"-"`
}

// label is one entry of the API's label objects.
type label struct {
	Name string `json:"name"`
}

// labelList is an issue's labels. The REST API returns each as a label object;
// a label update carries bare names and never round-trips through this type.
type labelList []string

func (l *labelList) UnmarshalJSON(data []byte) error {
	var raw []label
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	names := make(labelList, 0, len(raw))
	for _, item := range raw {
		names = append(names, item.Name)
	}
	*l = names
	return nil
}

// event is the subset of the issue timeline the parent enrollment check reads.
type event struct {
	Event string `json:"event"`
	Label label  `json:"label"`
}

// issueAPI is the reconciler's issue-API boundary. GitHubAPI backs it with gh
// in production and the tests back it with an in-memory fake.
type issueAPI interface {
	issue(number int) (issue, error)
	related(number int, relation string) ([]issue, error)
	events(number int) ([]event, error)
	blockerSatisfied(value issue) (bool, error)
	update(number int, changes map[string]any) error
	openIssues(label string) ([]issue, error)
}

// gitHubAPI backs issueAPI with the gh CLI, which supplies the runner's token.
type gitHubAPI struct {
	repository string
	binary     string
	merged     map[string]bool
	// issues memoizes issue reads within one run, seeded by the sweep's listing, so a swept issue is not fetched again per check; update drops the entry it changes.
	issues map[int]issue
}

func (g *gitHubAPI) gh(args ...string) ([]byte, error) {
	command := exec.Command(g.binary, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, message)
		}
		return nil, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func (g *gitHubAPI) get(path string, paginate bool, fields ...string) ([]byte, error) {
	args := []string{
		"api", "--method", "GET",
		"repos/" + g.repository + "/" + path,
		"-H", "X-GitHub-Api-Version: 2022-11-28",
	}
	if paginate {
		args = append(args, "--paginate", "--slurp", "-f", "per_page=100")
	}
	for _, field := range fields {
		args = append(args, "-f", field)
	}
	return g.gh(args...)
}

// repositoryOf returns the owner/name an API repository_url points at, or "" when the URL has no /repos/ segment.
func repositoryOf(url string) string {
	_, name, ok := strings.Cut(url, "/repos/")
	if !ok {
		return ""
	}
	return name
}

// mark flags the issues that belong to another repository. An issue without a repository_url is this repository's, as in an event payload.
func (g *gitHubAPI) mark(values []issue) []issue {
	for i := range values {
		if name := repositoryOf(values[i].RepositoryURL); name != "" && !strings.EqualFold(name, g.repository) {
			values[i].Foreign = true
		}
	}
	return values
}

// openIssues lists this repository's open issues carrying one label, without pull requests, which the issues endpoint also returns.
func (g *gitHubAPI) openIssues(label string) ([]issue, error) {
	out, err := g.get("issues", true, "state=open", "labels="+label)
	if err != nil {
		return nil, err
	}
	var pages [][]issue
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, err
	}
	var result []issue
	for _, page := range pages {
		for _, value := range page {
			if len(value.PullRequest) == 0 {
				g.remember(value)
				result = append(result, value)
			}
		}
	}
	return result, nil
}

func (g *gitHubAPI) issue(number int) (issue, error) {
	if value, ok := g.issues[number]; ok {
		return value, nil
	}
	out, err := g.get(fmt.Sprintf("issues/%d", number), false)
	if err != nil {
		return issue{}, err
	}
	var value issue
	if err := json.Unmarshal(out, &value); err != nil {
		return issue{}, err
	}
	g.remember(value)
	return value, nil
}

func (g *gitHubAPI) remember(value issue) {
	if g.issues == nil {
		g.issues = map[int]issue{}
	}
	g.issues[value.Number] = value
}

func (g *gitHubAPI) related(number int, relation string) ([]issue, error) {
	if relation == "parent" {
		out, err := g.get(fmt.Sprintf("issues/%d/parent", number), false)
		if err != nil {
			if isNotFound(err) {
				return nil, nil
			}
			return nil, err
		}
		var value issue
		if err := json.Unmarshal(out, &value); err != nil {
			return nil, err
		}
		return g.mark([]issue{value}), nil
	}
	paths := map[string]string{
		"blocked_by": fmt.Sprintf("issues/%d/dependencies/blocked_by", number),
		"blocking":   fmt.Sprintf("issues/%d/dependencies/blocking", number),
		"sub_issues": fmt.Sprintf("issues/%d/sub_issues", number),
	}
	values, err := g.getIssues(paths[relation])
	return g.mark(values), err
}

// getIssues flattens a paginated issue list; --slurp returns an array of pages.
func (g *gitHubAPI) getIssues(path string) ([]issue, error) {
	out, err := g.get(path, true)
	if err != nil {
		return nil, err
	}
	var pages [][]issue
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, err
	}
	var result []issue
	for _, page := range pages {
		result = append(result, page...)
	}
	return result, nil
}

func (g *gitHubAPI) events(number int) ([]event, error) {
	out, err := g.get(fmt.Sprintf("issues/%d/events", number), true)
	if err != nil {
		return nil, err
	}
	var pages [][]event
	if err := json.Unmarshal(out, &pages); err != nil {
		return nil, err
	}
	var result []event
	for _, page := range pages {
		result = append(result, page...)
	}
	return result, nil
}

// blockerSatisfied reports whether a merged pull request closed the issue;
// a hand-closed issue does not satisfy the frontier. A foreign issue's
// evidence is read from its own repository; when that repository cannot be
// read, it counts as unmerged, so a dependent stays blocked and a parent open
// rather than released or closed on evidence nobody saw.
func (g *gitHubAPI) blockerSatisfied(value issue) (bool, error) {
	if value.State != "closed" {
		return false, nil
	}
	repository := g.repository
	if value.Foreign {
		repository = repositoryOf(value.RepositoryURL)
	}
	key := repository + "#" + strconv.Itoa(value.Number)
	if g.merged == nil {
		g.merged = map[string]bool{}
	}
	if satisfied, ok := g.merged[key]; ok {
		return satisfied, nil
	}
	out, err := g.gh("issue", "view", strconv.Itoa(value.Number), "--repo", repository, "--json", "closedByPullRequestsReferences")
	if err != nil {
		if value.Foreign {
			fmt.Fprintf(os.Stderr, "reconcile-agent-lifecycle: cannot read %s, so it counts as unmerged: %v\n", key, err)
			g.merged[key] = false
			return false, nil
		}
		return false, err
	}
	var view struct {
		ClosedByPullRequestsReferences []struct {
			URL string `json:"url"`
		} `json:"closedByPullRequestsReferences"`
	}
	if err := json.Unmarshal(out, &view); err != nil {
		return false, err
	}
	// Cache only a complete answer: a failed lookup must not leave "unmerged" behind for the next issue in a sweep, which would block its dependents on evidence nobody read.
	satisfied := false
	for _, reference := range view.ClosedByPullRequestsReferences {
		out, err := g.gh("pr", "view", reference.URL, "--json", "state,mergedAt")
		if err != nil {
			return false, err
		}
		var state struct {
			State    string  `json:"state"`
			MergedAt *string `json:"mergedAt"`
		}
		if err := json.Unmarshal(out, &state); err != nil {
			return false, err
		}
		if state.State == "MERGED" || state.MergedAt != nil {
			satisfied = true
			break
		}
	}
	g.merged[key] = satisfied
	return satisfied, nil
}

func (g *gitHubAPI) update(number int, changes map[string]any) error {
	delete(g.issues, number)
	body, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	command := exec.Command(
		g.binary,
		"api", "--method", "PATCH",
		fmt.Sprintf("repos/%s/issues/%d", g.repository, number),
		"-H", "X-GitHub-Api-Version: 2022-11-28",
		"--input", "-",
	)
	command.Stdin = bytes.NewReader(body)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "HTTP 404") || strings.Contains(message, "Not Found")
}

// labelSet is a set of label names.
type labelSet map[string]bool

func labelNames(value issue) labelSet {
	names := labelSet{}
	for _, name := range value.Labels {
		names[name] = true
	}
	return names
}

func (s labelSet) sorted() []string {
	names := make([]string, 0, len(s))
	for name := range s {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s labelSet) minus(other labelSet) labelSet {
	result := labelSet{}
	for name := range s {
		if !other[name] {
			result[name] = true
		}
	}
	return result
}

func (s labelSet) intersects(other labelSet) bool {
	for name := range s {
		if other[name] {
			return true
		}
	}
	return false
}

func (s labelSet) equals(other labelSet) bool {
	if len(s) != len(other) {
		return false
	}
	for name := range s {
		if !other[name] {
			return false
		}
	}
	return true
}

func lifecycleSet(labels map[string]string) labelSet {
	set := labelSet{}
	for _, value := range labels {
		set[value] = true
	}
	return set
}

func defaultLabels() map[string]string {
	return map[string]string{
		"blocked":         "blocked",
		"ready_for_agent": "ready-for-agent",
		"in_progress":     "in-progress",
		"ready_for_human": "ready-for-human",
	}
}

func reconcileFrontier(api issueAPI, number int, labels map[string]string, resetActive bool) error {
	value, err := api.issue(number)
	if err != nil {
		return err
	}
	if value.State != "open" {
		return nil
	}

	current := labelNames(value)
	active := labelSet{labels["in_progress"]: true, labels["ready_for_human"]: true}
	released := labelSet{labels["blocked"]: true, labels["ready_for_agent"]: true}
	if !current.intersects(lifecycleSet(labels)) {
		return nil
	}
	desired := current.minus(released)
	blockers, err := api.related(number, "blocked_by")
	if err != nil {
		return err
	}
	unsatisfied := false
	for _, blocker := range blockers {
		satisfied, err := api.blockerSatisfied(blocker)
		if err != nil {
			return err
		}
		if !satisfied {
			unsatisfied = true
			break
		}
	}
	switch {
	case unsatisfied:
		desired = desired.minus(active)
		desired[labels["blocked"]] = true
	case resetActive || !current.intersects(active) || current.intersects(released):
		ticket := desired.minus(active)
		ticket[labels["ready_for_agent"]] = true
		if ticket.equals(current) {
			// Already on the frontier: keep the labels as they are. desired has the released labels stripped, so leaving the switch without restoring it would take the ticket off the frontier.
			desired = current
			break
		}
		// A parent is a container, not work an agent picks up: a reopen does not reset it, and a release returns it to in-progress. Any sub-issue makes it a parent, as in reconcileParent. The lookup runs only when the labels would change, so a steady frontier costs no extra call.
		children, err := api.related(number, "sub_issues")
		if err != nil {
			return err
		}
		switch {
		case len(children) == 0:
			desired = ticket
		case resetActive && current.intersects(active) && !current.intersects(released):
			// A reopened parent keeps its active label.
		default:
			desired = desired.minus(active)
			desired[labels["in_progress"]] = true
		}
	}
	if !desired.equals(current) {
		return api.update(number, map[string]any{"labels": desired.sorted()})
	}
	return nil
}

func reconcileParent(api issueAPI, number int, labels map[string]string) (bool, error) {
	parent, err := api.issue(number)
	if err != nil {
		return false, err
	}
	children, err := api.related(number, "sub_issues")
	if err != nil {
		return false, err
	}
	if len(children) == 0 {
		return false, nil
	}
	lifecycle := lifecycleSet(labels)
	current := labelNames(parent)
	// Label history preserves enrollment after completed parents shed status labels.
	if !current.intersects(lifecycle) {
		events, err := api.events(number)
		if err != nil {
			return false, err
		}
		enrolled := false
		for _, value := range events {
			if value.Event == "labeled" && lifecycle[value.Label.Name] {
				enrolled = true
				break
			}
		}
		if !enrolled {
			return false, nil
		}
	}

	target := "open"
	allSatisfied := true
	for _, child := range children {
		satisfied, err := api.blockerSatisfied(child)
		if err != nil {
			return false, err
		}
		if !satisfied {
			allSatisfied = false
			break
		}
	}
	if allSatisfied {
		target = "closed"
	}

	desired := current.minus(lifecycle)
	changes := map[string]any{}
	if parent.State != target {
		changes["state"] = target
	}
	if target == "open" {
		// An open parent keeps its lifecycle label, except ready-for-agent: a parent is a container, so one that gained a sub-issue while ready goes back to in-progress.
		if len(changes) == 0 && current.intersects(lifecycle) && !current[labels["ready_for_agent"]] {
			return false, nil
		}
		desired[labels["in_progress"]] = true
	}
	if !desired.equals(current) {
		changes["labels"] = desired.sorted()
	}
	if len(changes) == 0 {
		return false, nil
	}
	if err := api.update(number, changes); err != nil {
		return false, err
	}
	return true, nil
}

func reconcile(api issueAPI, eventNumber int, labels map[string]string, eventAction string) error {
	type item struct {
		number      int
		resetActive bool
	}
	queue := []item{{eventNumber, eventAction == "reopened"}}
	seen := map[int]bool{}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current.number] {
			continue
		}
		seen[current.number] = true

		if err := reconcileFrontier(api, current.number, labels, current.resetActive); err != nil {
			return err
		}
		dependents, err := api.related(current.number, "blocking")
		if err != nil {
			return err
		}
		for _, dependent := range dependents {
			if dependent.Foreign {
				continue
			}
			if err := reconcileFrontier(api, dependent.Number, labels, false); err != nil {
				return err
			}
		}
		changed, err := reconcileParent(api, current.number, labels)
		if err != nil {
			return err
		}
		if err := reconcileChildren(api, current.number, labels); err != nil {
			return err
		}
		if changed {
			if err := reconcileFrontier(api, current.number, labels, false); err != nil {
				return err
			}
		}
		parents, err := api.related(current.number, "parent")
		if err != nil {
			return err
		}
		if len(parents) > 0 && !parents[0].Foreign {
			changed, err := reconcileParent(api, parents[0].Number, labels)
			if err != nil {
				return err
			}
			if err := reconcileChildren(api, parents[0].Number, labels); err != nil {
				return err
			}
			if changed {
				queue = append(queue, item{parents[0].Number, false})
			}
		}
	}
	return nil
}

// sweep reconciles every open issue that carries a lifecycle label, in number order, and the parent of each, so an event the workflow's one-slot queue dropped heals on the next scheduled run: a merged blocker releases its dependents, a parent whose last child merged closes, and a closed parent whose child reopened reopens. A closed issue with no open enrolled child is not visited. Each swept issue costs about three REST calls (its blockers, its sub-issues, and its parent), since the listing supplies the issue itself. One issue's failure does not stop the others; the errors are returned together.
func sweep(api issueAPI, labels map[string]string) error {
	seen := map[int]bool{}
	var numbers []int
	for _, name := range lifecycleSet(labels).sorted() {
		values, err := api.openIssues(name)
		if err != nil {
			return err
		}
		for _, value := range values {
			if !seen[value.Number] {
				seen[value.Number] = true
				numbers = append(numbers, value.Number)
			}
		}
	}
	sort.Ints(numbers)
	var failures []error
	for _, number := range numbers {
		if err := reconcileFrontier(api, number, labels, false); err != nil {
			failures = append(failures, fmt.Errorf("#%d: %w", number, err))
			continue
		}
		if err := sweepParent(api, number, labels); err != nil {
			failures = append(failures, fmt.Errorf("#%d: %w", number, err))
			continue
		}
		parents, err := api.related(number, "parent")
		if err != nil {
			failures = append(failures, fmt.Errorf("#%d: %w", number, err))
			continue
		}
		if len(parents) > 0 && !parents[0].Foreign && !seen[parents[0].Number] {
			seen[parents[0].Number] = true
			if err := sweepParent(api, parents[0].Number, labels); err != nil {
				failures = append(failures, fmt.Errorf("#%d: %w", parents[0].Number, err))
			}
		}
	}
	return errors.Join(failures...)
}

// sweepParent settles one issue as a parent, then its frontier when that changed it, as reconcile does.
func sweepParent(api issueAPI, number int, labels map[string]string) error {
	changed, err := reconcileParent(api, number, labels)
	if err != nil || !changed {
		return err
	}
	return reconcileFrontier(api, number, labels, false)
}

// reconcileChildren recomputes the frontier of a parent's open, enrolled children. The workflow's concurrency group keeps at most one pending run, so a newer event can replace a blocker's close event; reconciling the event issue's children and siblings on each run heals a dropped event within that family at once, and the scheduled sweep heals deeper and unrelated trees. Closed, planning-only, and foreign children are skipped from the listed payload so a large tree costs no extra calls for them.
func reconcileChildren(api issueAPI, parent int, labels map[string]string) error {
	children, err := api.related(parent, "sub_issues")
	if err != nil {
		return err
	}
	lifecycle := lifecycleSet(labels)
	for _, child := range children {
		if child.Foreign || child.State != "open" || !labelNames(child).intersects(lifecycle) {
			continue
		}
		if err := reconcileFrontier(api, child.Number, labels, false); err != nil {
			return err
		}
	}
	return nil
}

func configuredLabels(path string) (map[string]string, error) {
	labels := defaultLabels()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return labels, nil
		}
		return nil, err
	}
	var config struct {
		Labels map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	for key, value := range config.Labels {
		labels[key] = value
	}
	return labels, nil
}

func run() error {
	raw, err := os.ReadFile(os.Getenv("GITHUB_EVENT_PATH"))
	if err != nil {
		return err
	}
	var payload struct {
		Issue struct {
			Number int `json:"number"`
		} `json:"issue"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	labels, err := configuredLabels("docs/agents/harness.json")
	if err != nil {
		return err
	}
	repository := os.Getenv("GITHUB_REPOSITORY")
	if repository == "" {
		return fmt.Errorf("GITHUB_REPOSITORY is not set")
	}
	api := &gitHubAPI{repository: repository, binary: "gh"}
	switch os.Getenv("GITHUB_EVENT_NAME") {
	case "schedule", "workflow_dispatch":
		return sweep(api, labels)
	}
	return reconcile(api, payload.Issue.Number, labels, payload.Action)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "reconcile-agent-lifecycle:", err)
		os.Exit(1)
	}
}
