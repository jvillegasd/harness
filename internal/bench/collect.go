package bench

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"

	"github.com/jvillegasd/harness/internal/checkout"
)

const claudeInterrupt = "[Request interrupted by user"

var claudeDenials = [][]string{
	{"The user doesn't want to proceed with this tool use"},
	{"Permission to use ", " has been denied"},
}

var claudeNonTurn = []string{claudeInterrupt, "<command-name>", "<command-message", "<local-command", "<system-reminder>", "<task-notification>"}

const opencodeQuery = `
select s.time_created,
       s.time_updated,
       s.directory,
       (select count(*) from message m
         where m.session_id = s.id and json_extract(m.data, '$.role') = 'user'),
       (select count(*) from part p
         where p.session_id = s.id and json_extract(p.data, '$.type') = 'tool'),
       (select count(*) from message m join session c on c.id = m.session_id
         where c.parent_id = s.id)
from session s
where s.parent_id is null and s.time_created >= ?
`

func patterns(c checkout.Checkout, stderr io.Writer) []string {
	for _, name := range []string{"cohorts.json", "cohorts.example.json"} {
		data, err := os.ReadFile(filepath.Join(c.Bench(), name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			fmt.Fprintf(stderr, "harness bench: %s: %v\n", name, err)
			return nil
		}
		var doc struct {
			Work []string `json:"work"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			fmt.Fprintf(stderr, "harness bench: %s: %v\n", name, err)
			return nil
		}
		return doc.Work
	}
	return nil
}

type session struct {
	client  string
	date    time.Time
	project *string
	values  map[string]any
	cohort  string
}

func newSession(client string, started time.Time, project *string, values map[string]any) *session {
	return &session{
		client:  client,
		date:    startOfDay(started),
		project: project,
		values:  values,
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *session) metric(name string) (float64, bool) {
	switch value := s.values[name].(type) {
	case int:
		return float64(value), true
	case float64:
		return value, true
	}
	return 0, false
}

func collect(home string, clients []string, work []string, since *time.Time) ([]*session, error) {
	if since != nil {
		day := startOfDay(*since)
		since = &day
	}
	var sessions []*session
	seen := make(map[string]bool, len(clients))
	for _, client := range clients {
		if seen[client] {
			continue
		}
		seen[client] = true
		var found []*session
		var err error
		switch client {
		case "claude":
			found, err = claudeSessions(home, since)
		case "codex":
			found, err = codexSessions(home, since)
		case "opencode":
			found, err = opencodeSessions(home, since)
		default:
			return nil, fmt.Errorf("unknown client %q", client)
		}
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, found...)
	}
	sessions = slices.DeleteFunc(sessions, func(s *session) bool { return inScratch(s.project, home) })
	for _, s := range sessions {
		s.cohort = cohort(s.project, work)
	}
	return sessions, nil
}

func claudeSessions(home string, since *time.Time) ([]*session, error) {
	root := filepath.Join(home, ".claude", "projects")
	if !isDir(root) {
		return nil, nil
	}

	paths, _ := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	sort.Strings(paths)
	var sessions []*session
	for _, path := range paths {
		s, err := readClaude(path, since)
		if err != nil {
			return nil, err
		}
		if s != nil {
			sessions = append(sessions, s)
		}
	}
	return sessions, nil
}

func readClaude(path string, since *time.Time) (*session, error) {
	var first, last time.Time
	seen, excluded := false, false
	turns, tools, commands, interrupts, denials, failures := 0, 0, 0, 0, 0, 0
	err := forEachEntry(path, func(entry map[string]any) bool {
		if stamp, ok := parseStamp(entry["timestamp"]); ok {
			if !seen {
				first = stamp
				seen = true

				if since != nil && startOfDay(first).Before(*since) {
					excluded = true
					return false
				}
			}
			last = stamp
		}
		blocks := blocksOf(messageContent(entry))
		if entry["type"] == "assistant" {
			for _, block := range blocks {
				if block["type"] == "tool_use" {
					tools++
				}
			}
			return true
		}
		if entry["type"] != "user" {
			return true
		}

		typed := false
		for _, block := range blocks {
			body := claudeText(block)
			if block["type"] == "tool_result" {

				if isDenial(body) {
					denials++
				} else if truthy(block["is_error"]) {
					failures++
				}
				continue
			}
			commands += strings.Count(body, "<command-name>")
			interrupts += strings.Count(body, claudeInterrupt)
			lead := strings.TrimLeftFunc(body, unicode.IsSpace)
			prompt := lead != "" && !hasNonTurnPrefix(lead)
			typed = typed || prompt || hasCommandArgs(lead)
		}

		if typed && !truthy(entry["isMeta"]) && !truthy(entry["isCompactSummary"]) {
			turns++
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if excluded || !seen {
		return nil, nil
	}
	delegation, err := claudeDelegation(path)
	if err != nil {
		return nil, err
	}
	project := filepath.Base(filepath.Dir(path))
	return newSession("claude", first, &project, map[string]any{
		"turns": turns, "tool_calls": tools, "minutes": round1(last.Sub(first).Seconds() / 60),
		"commands": commands, "delegation": delegation,
		"interrupts": interrupts, "denials": denials, "errors": failures,
	}), nil
}

func claudeDelegation(path string) (int, error) {
	stem := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	sidecars, _ := filepath.Glob(filepath.Join(filepath.Dir(path), stem, "subagents", "*.jsonl"))
	sort.Strings(sidecars)
	total := 0
	for _, sidecar := range sidecars {
		err := forEachEntry(sidecar, func(entry map[string]any) bool {
			if entry["type"] == "user" || entry["type"] == "assistant" {
				total++
			}
			return true
		})
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

func messageContent(entry map[string]any) any {
	message, ok := entry["message"].(map[string]any)
	if !ok {
		return nil
	}
	return message["content"]
}

func blocksOf(content any) []map[string]any {
	if list, ok := content.([]any); ok {
		var blocks []map[string]any
		for _, item := range list {
			if block, ok := item.(map[string]any); ok {
				blocks = append(blocks, block)
			}
		}
		return blocks
	}
	return []map[string]any{{"text": content}}
}

func claudeText(block map[string]any) string {
	value := block["text"]
	if !truthy(value) {
		value = block["content"]
	}
	if !truthy(value) {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func hasCommandArgs(body string) bool {
	if !strings.HasPrefix(body, "<command-name>") && !strings.HasPrefix(body, "<command-message") {
		return false
	}
	_, rest, ok := strings.Cut(body, "<command-args>")
	if !ok {
		return false
	}
	if end := strings.LastIndex(rest, "</command-args>"); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest) != ""
}

func hasNonTurnPrefix(body string) bool {
	for _, prefix := range claudeNonTurn {
		if strings.HasPrefix(body, prefix) {
			return true
		}
	}
	return false
}

func isDenial(body string) bool {
	for _, literal := range claudeDenials {
		matched := true
		for _, part := range literal {
			if !strings.Contains(body, part) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	case int:
		return v != 0
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	}
	return true
}

func parseStamp(value any) (time.Time, bool) {
	text, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	text = strings.ReplaceAll(text, "Z", "+00:00")

	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if stamp, err := time.Parse(layout, text); err == nil {
			return stamp, true
		}
	}
	return time.Time{}, false
}

const maxRecordBytes = 64 << 20

func forEachEntry(path string, fn func(entry map[string]any) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRecordBytes)
	for scanner.Scan() {
		var value any

		if json.Unmarshal(bytes.TrimSpace(scanner.Bytes()), &value) != nil {
			continue
		}
		if entry, ok := value.(map[string]any); ok {
			if !fn(entry) {
				return nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("%s: record exceeds %d bytes", path, maxRecordBytes)
		}
		return err
	}
	return nil
}

var codexDenials = []string{"rejected by user", "exec command rejected by user", "patch rejected by user"}

type codexRollout struct {
	id       string
	parent   string
	subagent bool
	items    int
	session  *session
}

func codexSessions(home string, since *time.Time) ([]*session, error) {
	var paths []string
	for _, dir := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, ".codex", dir)
		if !isDir(root) {
			continue
		}
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "rollout-") && strings.HasSuffix(entry.Name(), ".jsonl") {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	var sessions []*session
	byID := map[string]*session{}
	parents := map[string]string{}
	var subagents []*codexRollout
	for _, path := range paths {
		r, err := readCodex(path, since)
		if err != nil {
			return nil, err
		}
		switch {
		case r == nil:
		case r.subagent:
			parents[r.id] = r.parent
			subagents = append(subagents, r)
		default:
			sessions = append(sessions, r.session)
			if r.id != "" {
				byID[r.id] = r.session
			}
		}
	}

	for _, sub := range subagents {
		id := sub.parent
		for hops := 0; id != "" && hops <= len(parents); hops++ {
			if s, ok := byID[id]; ok {
				s.values["delegation"] = s.values["delegation"].(int) + sub.items
				break
			}
			id = parents[id]
		}
	}
	return sessions, nil
}

func readCodex(path string, since *time.Time) (*codexRollout, error) {
	var first, last time.Time
	seen, excluded, hasMeta := false, false, false
	r := &codexRollout{}
	var project *string
	turns, tools, interrupts, denials := 0, 0, 0, 0
	err := forEachEntry(path, func(entry map[string]any) bool {
		if stamp, ok := parseStamp(entry["timestamp"]); ok {
			if !seen {
				first = stamp
				seen = true
				if since != nil && startOfDay(first).Before(*since) {
					excluded = true
					return false
				}
			}
			last = stamp
		}
		payload, _ := entry["payload"].(map[string]any)
		switch entry["type"] {
		case "session_meta":

			if hasMeta {
				return true
			}
			hasMeta = true
			r.id, _ = payload["id"].(string)
			r.parent, r.subagent = codexParent(payload)
			if cwd, ok := payload["cwd"].(string); ok {
				project = &cwd
			}
		case "event_msg":
			switch payload["type"] {
			case "user_message":
				turns++
			case "turn_aborted":
				if payload["reason"] == "interrupted" {
					interrupts++
				}
			}
		case "response_item":
			kind, _ := payload["type"].(string)
			switch {
			case kind == "message":
				if role := payload["role"]; role == "user" || role == "assistant" {
					r.items++
				}
			case strings.HasSuffix(kind, "_call"):
				tools++
				r.items++
			case strings.HasSuffix(kind, "_output"):
				r.items++
				if output, ok := payload["output"].(string); ok && slices.Contains(codexDenials, output) {
					denials++
				}
			}
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if excluded || !seen || !hasMeta {
		return nil, nil
	}
	if r.subagent {
		return r, nil
	}

	r.session = newSession("codex", first, project, map[string]any{
		"turns": turns, "tool_calls": tools, "minutes": round1(last.Sub(first).Seconds() / 60),
		"delegation": 0, "interrupts": interrupts, "denials": denials,
	})
	return r, nil
}

func codexParent(meta map[string]any) (string, bool) {
	if parent, ok := meta["parent_thread_id"].(string); ok && parent != "" {
		return parent, true
	}
	source, ok := meta["source"].(map[string]any)
	if !ok {
		return "", false
	}
	if sub, ok := source["subagent"]; ok {
		spawn, _ := sub.(map[string]any)["thread_spawn"].(map[string]any)
		parent, _ := spawn["parent_thread_id"].(string)
		return parent, true
	}
	_, internal := source["internal"]
	return "", internal
}

func opencodeSessions(home string, since *time.Time) ([]*session, error) {
	path := filepath.Join(home, ".local", "share", "opencode", "opencode.db")
	if !isFile(path) {
		return nil, nil
	}

	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	boundary := int64(math.MinInt64)
	if since != nil {
		boundary = since.UnixMilli()
	}
	rows, err := db.Query(opencodeQuery, boundary)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sessions []*session
	for rows.Next() {
		var created, updated int64
		var directory sql.NullString
		var turns, tools, delegation int
		if err := rows.Scan(&created, &updated, &directory, &turns, &tools, &delegation); err != nil {
			return nil, err
		}
		var project *string
		if directory.Valid {
			project = &directory.String
		}
		started := time.UnixMilli(created).UTC()
		changed := time.UnixMilli(updated).UTC()

		sessions = append(sessions, newSession("opencode", started, project, map[string]any{
			"turns": turns, "tool_calls": tools,
			"minutes":    round1(changed.Sub(started).Seconds() / 60),
			"delegation": delegation,
		}))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sessions, nil
}

const scratchDir = ".scratch-projects"

func inScratch(project *string, home string) bool {
	if project == nil {
		return false
	}
	root := filepath.Join(home, scratchDir)
	if filepath.IsAbs(*project) {
		return *project == root || strings.HasPrefix(*project, root+string(filepath.Separator))
	}
	slug := claudeSlug(root)
	return *project == slug || strings.HasPrefix(*project, slug+"-")
}

func claudeSlug(path string) string {
	var b strings.Builder
	for _, r := range path {
		if r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func canon(value string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		if r == '-' || r == '/' {
			if !dash {
				b.WriteByte('/')
			}
			dash = true
			continue
		}
		dash = false
		b.WriteRune(r)
	}
	return b.String()
}

func cohort(project *string, work []string) string {
	if project == nil {
		return "personal"
	}
	folded := canon(*project)
	for _, pattern := range work {
		if strings.Contains(folded, canon(pattern)) {
			return "work"
		}
	}
	return "personal"
}

func round1(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 1, 64), 64)
	if err != nil {
		return value
	}
	return rounded
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
