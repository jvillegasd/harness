package scenarios

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The delivery outcomes of one recorded attempt. Only an adhered treatment with passing seed and hidden acceptance tests is delivered; a treatment that adhered but failed correctness is failed; everything the evidence cannot establish is unverified.
const (
	deliveryDelivered  = "delivered"
	deliveryFailed     = "failed"
	deliveryUnverified = "unverified"
)

// deliveryTreatments are exactly the supported delivery arms.
var deliveryTreatments = []string{"inline", "native", "dispatch"}
var deliveryContracts = []string{"fix", "feature", "refactor"}

// validateDelivery checks a delivery-treatment declaration before any model runs: exactly the supported arms, and representative contracts whose seed and hidden acceptance tests exist under root.
func validateDelivery(root string, decl map[string]any) []string {
	var errs []string
	arms, ok := decl["treatments"].([]any)
	if !ok {
		errs = append(errs, "treatments must be a list")
	}
	seen := map[string]bool{}
	for _, raw := range arms {
		arm, _ := raw.(string)
		switch {
		case !containsString(deliveryTreatments, arm):
			errs = append(errs, fmt.Sprintf("unsupported treatment %q", arm))
		case seen[arm]:
			errs = append(errs, fmt.Sprintf("duplicate treatment %q", arm))
		}
		seen[arm] = true
	}
	for _, arm := range deliveryTreatments {
		if ok && !seen[arm] {
			errs = append(errs, "missing treatment "+arm)
		}
	}
	contracts := asList(decl["contracts"])
	if len(contracts) == 0 {
		errs = append(errs, "at least one representative contract is required")
	}
	for i, raw := range contracts {
		contract, ok := raw.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Sprintf("contracts[%d] must be an object", i))
			continue
		}
		if getString(contract, "id") == "" {
			errs = append(errs, fmt.Sprintf("contracts[%d].id must be declared", i))
		} else if !containsString(deliveryContracts, getString(contract, "id")) {
			errs = append(errs, fmt.Sprintf("contracts[%d].id %q is unsupported", i, getString(contract, "id")))
		}
		for _, field := range []string{"seed_tests", "acceptance_tests"} {
			rel := getString(contract, field)
			switch {
			case rel == "":
				errs = append(errs, fmt.Sprintf("contracts[%d].%s must be declared", i, field))
			case !filepath.IsLocal(rel):
				errs = append(errs, fmt.Sprintf("contracts[%d].%s must stay inside the seed root", i, field))
			case !deliveryInputExists(root, rel):
				errs = append(errs, fmt.Sprintf("contracts[%d].%s: missing seed input %s", i, field, rel))
			}
		}
	}
	return errs
}

func deliveryInputExists(root, relative string) bool {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	resolvedInput, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return false
	}
	relativeInput, err := filepath.Rel(resolvedRoot, resolvedInput)
	if err != nil || relativeInput == ".." || strings.HasPrefix(relativeInput, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeInput) {
		return false
	}
	info, err := os.Stat(resolvedInput)
	return err == nil && info.Mode().IsRegular()
}

// deliveryEvent is one recorded tool event. Narration is never an event.
type deliveryEvent struct {
	Tool      string `json:"tool"`
	Subagent  string `json:"subagent"` // the subagent type a spawn event named
	Command   string `json:"command"`  // the shell command a Bash event ran
	Succeeded bool   `json:"succeeded"`
}

func (e deliveryEvent) spawnsAgent() bool { return e.Tool == "Agent" || e.Tool == "Task" }

func (e deliveryEvent) attemptsDispatch() bool {
	if e.Tool != "Bash" {
		return false
	}
	for _, segment := range shellSegments(e.Command) {
		fields := shellFields(segment)
		if dispatchInvocation(fields) || shellCommand(fields) == "eval" {
			return true
		}
		for index := 0; index+1 < len(fields); index++ {
			if filepath.Base(fields[index]) == "bash" || filepath.Base(fields[index]) == "sh" || filepath.Base(fields[index]) == "zsh" {
				if fields[index+1] == "-c" && index+2 < len(fields) && strings.Contains(fields[index+2], "dispatch") && strings.Contains(fields[index+2], "executor") && dispatchCommand(fields[index+2]) {
					return true
				}
			}
		}
	}
	return false
}

func shellSegments(command string) []string {
	var segments []string
	start := 0
	var quote byte
	for index := 0; index < len(command); index++ {
		if quote != 0 {
			if command[index] == quote {
				quote = 0
			}
			if command[index] == '\\' && quote == '"' && index+1 < len(command) {
				index++
			}
			continue
		}
		if command[index] == '\'' || command[index] == '"' {
			quote = command[index]
			continue
		}
		if command[index] == '\\' && index+1 < len(command) && (command[index+1] == '\n' || command[index+1] == '\r') {
			index++
			continue
		}
		if !strings.ContainsRune(";|&\n\r", rune(command[index])) {
			continue
		}
		segments = append(segments, command[start:index])
		if index+1 < len(command) && command[index+1] == command[index] {
			index++
		}
		start = index + 1
	}
	return append(segments, command[start:])
}

func shellFields(command string) []string {
	var fields []string
	var field strings.Builder
	var quote byte
	haveField := false
	for index := 0; index < len(command); index++ {
		char := command[index]
		if quote != 0 {
			if char == quote {
				quote = 0
				continue
			}
			if char == '\\' && quote == '"' && index+1 < len(command) {
				if command[index+1] == '\n' || command[index+1] == '\r' {
					index++
					continue
				}
				field.WriteByte(command[index+1])
				index++
				continue
			}
			field.WriteByte(char)
			haveField = true
			continue
		}
		switch char {
		case '\'', '"':
			quote = char
			haveField = true
		case '\\':
			if index+1 < len(command) && (command[index+1] == '\n' || command[index+1] == '\r') {
				index++
				continue
			}
			field.WriteByte(char)
			haveField = true
		case ' ', '\t', '\n', '\r':
			if haveField {
				fields = append(fields, field.String())
				field.Reset()
				haveField = false
			}
		default:
			field.WriteByte(char)
			haveField = true
		}
	}
	if haveField {
		fields = append(fields, field.String())
	}
	return fields
}

func dispatchExecutorToken(field string) bool {
	if field == "executor" {
		return true
	}
	return strings.HasPrefix(field, "executor") && strings.ContainsAny(field[len("executor"):], "$`")
}

func shellCommand(fields []string) string {
	commandIndex := shellCommandIndex(fields)
	if commandIndex < 0 {
		return ""
	}
	return filepath.Base(fields[commandIndex])
}

func dispatchInvocation(fields []string) bool {
	commandIndex := shellCommandIndex(fields)
	if commandIndex < 0 {
		return false
	}
	return commandIndex+2 < len(fields) && filepath.Base(fields[commandIndex]) == "harness" && fields[commandIndex+1] == "dispatch" && dispatchExecutorToken(fields[commandIndex+2])
}

func shellCommandIndex(fields []string) int {
	if len(fields) == 0 {
		return -1
	}
	if filepath.Base(fields[0]) != "env" {
		return 0
	}
	for index := 1; index < len(fields); index++ {
		field := fields[index]
		if field == "--" {
			return index + 1
		}
		if strings.HasPrefix(field, "-") {
			if envOptionNeedsValue(field) && !strings.Contains(field, "=") {
				index++
			}
			continue
		}
		if strings.Contains(field, "=") {
			continue
		}
		return index
	}
	return -1
}

func envOptionNeedsValue(field string) bool {
	switch field {
	case "-u", "--unset", "-P", "--path", "-C", "--chdir", "-S", "--split-string":
		return true
	default:
		return false
	}
}

func dispatchCommand(command string) bool {
	for _, segment := range shellSegments(command) {
		fields := shellFields(segment)
		if dispatchInvocation(fields) {
			return true
		}
	}
	return false
}

func (e deliveryEvent) dispatchesExecutor() bool {
	if e.Tool != "Bash" {
		return false
	}
	fields := strings.Fields(e.Command)
	if strings.Join(fields, " ") != e.Command {
		return false
	}
	if len(fields) < 5 || fields[0] != "harness" || fields[1] != "dispatch" || fields[2] != "executor" || !e.Succeeded {
		return false
	}
	if strings.ContainsAny(e.Command, "\n\r|;&<>$`") {
		return false
	}
	seenPrompt := false
	for index := 3; index < len(fields); index++ {
		field := fields[index]
		switch {
		case field == "--prompt-file":
			if seenPrompt || index+1 == len(fields) || strings.HasPrefix(fields[index+1], "-") {
				return false
			}
			seenPrompt = true
			index++
		case strings.HasPrefix(field, "--prompt-file=") && len(field) > len("--prompt-file="):
			if seenPrompt {
				return false
			}
			seenPrompt = true
		case field == "--worktree" || field == "--session":
			if index+1 == len(fields) || strings.HasPrefix(fields[index+1], "-") {
				return false
			}
			index++
		case strings.HasPrefix(field, "--worktree=") || strings.HasPrefix(field, "--session="):
		default:
			return false
		}
	}
	return seenPrompt
}

// deliveryEvidence is what one attempt recorded. SeedPass and AcceptancePass are nil when no result was recorded. Narration is carried only to show the oracle ignores it.
type deliveryEvidence struct {
	Treatment      string          `json:"treatment"`
	Available      bool            `json:"available"` // the route and model answered
	Events         []deliveryEvent `json:"events"`
	Narration      string          `json:"narration"`
	SeedPass       *bool           `json:"seed_pass"`
	AcceptancePass *bool           `json:"acceptance_pass"`
}

// deliveryAdhered derives adherence from tool events alone.
func deliveryAdhered(treatment string, events []deliveryEvent) (bool, string) {
	delegations, bashEvents, dispatchAttempts, successfulDispatches := 0, 0, 0, 0
	executor := false
	for _, e := range events {
		if e.Tool == "Bash" {
			bashEvents++
		}
		if e.spawnsAgent() {
			delegations++
			executor = executor || (e.Subagent == "executor" && e.Succeeded)
		}
		if e.attemptsDispatch() {
			dispatchAttempts++
		}
		if e.dispatchesExecutor() {
			successfulDispatches++
		}
	}
	switch treatment {
	case "inline":
		if delegations > 0 || dispatchAttempts > 0 {
			return false, "inline attempt spawned delegation"
		}
	case "native":
		if delegations != 1 || !executor || dispatchAttempts > 0 {
			return false, "native treatment lacks exclusive executor evidence"
		}
	case "dispatch":
		if bashEvents != 1 || dispatchAttempts != 1 || successfulDispatches != 1 || delegations > 0 {
			return false, "dispatch treatment lacks exclusive successful dispatch evidence"
		}
	default:
		return false, fmt.Sprintf("unsupported treatment %q", treatment)
	}
	return true, ""
}

// deliveryOracle classifies one recorded attempt offline.
func deliveryOracle(e deliveryEvidence) (string, []string) {
	if !e.Available {
		return deliveryUnverified, []string{"route or model unavailable"}
	}
	if len(e.Events) == 0 {
		return deliveryUnverified, []string{"recorded events are missing"}
	}
	if ok, reason := deliveryAdhered(e.Treatment, e.Events); !ok {
		return deliveryUnverified, []string{reason}
	}
	if e.SeedPass == nil || e.AcceptancePass == nil {
		return deliveryUnverified, []string{"seed or acceptance test result missing"}
	}
	if !*e.SeedPass || !*e.AcceptancePass {
		return deliveryFailed, []string{"treatment adhered but seed or acceptance tests failed"}
	}
	return deliveryDelivered, nil
}

var deliveryFixtureNames = []string{
	"inline-delivered",
	"native-delivered",
	"dispatch-delivered",
	"inline-failed",
	"inline-non-adherent",
	"native-non-adherent",
	"dispatch-non-adherent",
	"route-unavailable",
	"missing-evidence",
}

func readDeliveryFixture(root, name string) (deliveryEvidence, error) {
	raw, err := os.ReadFile(filepath.Join(root, "scenarios", "fixtures", "delivery", name+".json"))
	if err != nil {
		return deliveryEvidence{}, err
	}
	var evidence deliveryEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return deliveryEvidence{}, err
	}
	return evidence, nil
}
