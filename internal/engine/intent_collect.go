package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Intent collection reads the local Claude Code, Codex and Pi session logs
// that were started inside the reviewed worktree, picks the session whose
// conversation mentions the changed files, and returns the user's own
// requests from it. It never calls a model and never summarizes.

const (
	intentWindow       = 72 * time.Hour
	intentSessionBytes = 64 << 20
	intentLimit        = 8192
	intentDecisive     = 0.85
)

var errNoIntent = errors.New("no local agent session started in this worktree mentions the changed files")

type intentSession struct {
	Agent    string
	ID       string
	Path     string
	Started  time.Time
	Last     time.Time
	Requests []string
	Score    float64
}

type intentMatch struct {
	Session    *intentSession
	Candidates []*intentSession
}

func (s *intentSession) Label() string {
	when := s.Last.Local().Format("2006-01-02 15:04")
	return fmt.Sprintf("%s session %s (%s)", backendNames[s.Agent], short8(s.ID), when)
}

func short8(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Intent returns the requests with their source, and false when they do not
// fit the controller's intent limit; requests are never cut.
func (s *intentSession) Intent() (string, bool) {
	text := collectedPrefix + s.Label() + ". These are the user's own words from that session, not a summary:\n\n" + strings.Join(s.Requests, "\n\n")
	return text, len(text) <= intentLimit
}

func (s *intentSession) FirstLine() string {
	if len(s.Requests) == 0 {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(s.Requests[0]), "\n")
	return line
}

func committerTime(worktree, rev string) (time.Time, error) {
	b, err := gitRead(worktree, "show", "-s", "--format=%ct", rev, "--")
	if err != nil {
		return time.Time{}, err
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	return time.Unix(seconds, 0), err
}

func collectIntent(ctx context.Context, home, worktree, target, head string) (intentMatch, error) {
	var match intentMatch
	worktree, err := filepath.EvalSymlinks(worktree)
	if err != nil {
		return match, err
	}
	names, err := gitRead(worktree, "diff", "--name-only", "--no-renames", target, head, "--")
	if err != nil {
		return match, errors.New("cannot list the changed files")
	}
	files := strings.Fields(string(names))
	if len(files) == 0 {
		return match, errNoIntent
	}
	base, err := committerTime(worktree, target)
	if err != nil {
		return match, err
	}
	headTime, err := committerTime(worktree, head)
	if err != nil {
		return match, err
	}
	since := base.Add(-intentWindow)
	var sessions []*intentSession
	for _, discover := range []func(string, string, time.Time) []*intentSession{claudeSessions, codexSessions, piSessions} {
		for _, s := range discover(home, worktree, since) {
			if err := ctx.Err(); err != nil {
				return match, err
			}
			if !s.Started.IsZero() && s.Started.After(headTime.Add(time.Hour)) {
				continue
			}
			if loadIntentSession(ctx, s, worktree, files) && len(s.Requests) > 0 {
				sessions = append(sessions, s)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return match, err
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].Score != sessions[j].Score {
			return sessions[i].Score > sessions[j].Score
		}
		return sessions[i].Last.After(sessions[j].Last)
	})
	switch {
	case len(sessions) == 0:
		return match, errNoIntent
	case len(sessions) == 1 || sessions[0].Score >= intentDecisive && sessions[1].Score < intentDecisive:
		match.Session = sessions[0]
	default:
		match.Candidates = sessions[:min(len(sessions), 4)]
	}
	return match, nil
}

func inside(dir, root string) bool {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	return dir == root || strings.HasPrefix(dir, root+string(filepath.Separator))
}

// encodedPath mirrors how Claude Code and Pi name per-directory session
// folders, so discovery opens only folders for this worktree.
func encodedPath(path string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, path)
}

func recentLogs(dir string, since time.Time) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || !strings.HasSuffix(e.Name(), ".jsonl") || info.ModTime().Before(since) || info.Size() > intentSessionBytes {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func scopedDirs(root, prefix string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && (e.Name() == prefix || strings.HasPrefix(e.Name(), prefix+"-")) {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

func firstRecord(path string, v any, accept func() bool) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for i := 0; i < 50 && scanner.Scan(); i++ {
		if json.Unmarshal(scanner.Bytes(), v) == nil && accept() {
			return true
		}
	}
	return false
}

func parseTime(value string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, value)
	return t
}

func modTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func claudeSessions(home, worktree string, since time.Time) []*intentSession {
	var out []*intentSession
	for _, dir := range scopedDirs(filepath.Join(home, ".claude", "projects"), encodedPath(worktree)) {
		for _, path := range recentLogs(dir, since) {
			var first struct {
				CWD       string `json:"cwd"`
				Timestamp string `json:"timestamp"`
				SessionID string `json:"sessionId"`
			}
			if !firstRecord(path, &first, func() bool { return first.CWD != "" }) || !inside(first.CWD, worktree) {
				continue
			}
			id := first.SessionID
			if id == "" {
				id = strings.TrimSuffix(filepath.Base(path), ".jsonl")
			}
			out = append(out, &intentSession{Agent: backendClaude, ID: id, Path: path, Started: parseTime(first.Timestamp), Last: modTime(path)})
		}
	}
	return out
}

func piSessions(home, worktree string, since time.Time) []*intentSession {
	var out []*intentSession
	prefix := "--" + encodedPath(strings.TrimPrefix(worktree, "/"))
	root := filepath.Join(home, ".pi", "agent", "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		for _, path := range recentLogs(filepath.Join(root, e.Name()), since) {
			var first struct {
				Type      string `json:"type"`
				ID        string `json:"id"`
				CWD       string `json:"cwd"`
				Timestamp string `json:"timestamp"`
			}
			if !firstRecord(path, &first, func() bool { return first.Type == "session" && first.CWD != "" }) || !inside(first.CWD, worktree) {
				continue
			}
			out = append(out, &intentSession{Agent: backendPi, ID: first.ID, Path: path, Started: parseTime(first.Timestamp), Last: modTime(path)})
		}
	}
	return out
}

func codexSessions(home, worktree string, since time.Time) []*intentSession {
	var out []*intentSession
	root := filepath.Join(home, ".codex", "sessions")
	for day := time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.Local); !day.After(time.Now()); day = day.AddDate(0, 0, 1) {
		for _, path := range recentLogs(filepath.Join(root, day.Format("2006"), day.Format("01"), day.Format("02")), since) {
			var first struct {
				Type    string `json:"type"`
				Payload struct {
					ID        string `json:"id"`
					CWD       string `json:"cwd"`
					Timestamp string `json:"timestamp"`
				} `json:"payload"`
			}
			if !firstRecord(path, &first, func() bool { return first.Type == "session_meta" && first.Payload.CWD != "" }) || !inside(first.Payload.CWD, worktree) {
				continue
			}
			out = append(out, &intentSession{Agent: backendCodex, ID: first.Payload.ID, Path: path, Started: parseTime(first.Payload.Timestamp), Last: modTime(path)})
		}
	}
	return out
}

// loadIntentSession reads the user's requests and scores how many changed
// files the session mentions. Tool output counts toward matching only; it
// never becomes intent text.
func loadIntentSession(ctx context.Context, s *intentSession, worktree string, files []string) bool {
	f, err := os.Open(s.Path)
	if err != nil {
		return false
	}
	defer f.Close()
	mentioned := map[string]bool{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64<<10), 16<<20)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return false
		}
		line := scanner.Bytes()
		for _, file := range files {
			if !mentioned[file] && (strings.Contains(string(line), worktree+"/"+file) || strings.Contains(string(line), `"`+file+`"`) || strings.Contains(string(line), " "+file)) {
				mentioned[file] = true
			}
		}
		if text := userRequest(s.Agent, line); text != "" && !has(s.Requests, text) {
			s.Requests = append(s.Requests, string(redact([]byte(text))))
		}
	}
	overlap := len(mentioned)
	s.Score = float64(overlap) / float64(len(files))
	if overlap == 0 || len(files) > 1 && (overlap < 2 || s.Score < 0.5) {
		return false
	}
	return !(s.Last.Before(time.Now().Add(-intentWindow)) && s.Score < 0.8)
}

func userRequest(agent string, line []byte) string {
	var text string
	switch agent {
	case backendClaude:
		var r struct {
			Type    string `json:"type"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &r) != nil || r.Type != "user" || r.IsMeta {
			return ""
		}
		text = contentText(r.Message.Content)
	case backendPi:
		var r struct {
			Type    string `json:"type"`
			Message struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(line, &r) != nil || r.Type != "message" || r.Message.Role != "user" {
			return ""
		}
		text = contentText(r.Message.Content)
	case backendCodex:
		var r struct {
			Type    string `json:"type"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &r) != nil || r.Type != "event_msg" || r.Payload.Type != "user_message" {
			return ""
		}
		text = r.Payload.Message
	}
	return cleanRequest(text)
}

func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var items []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &items) != nil {
		return ""
	}
	var parts []string
	for _, item := range items {
		if item.Type == "text" || item.Type == "input_text" {
			parts = append(parts, item.Text)
		}
	}
	return strings.Join(parts, "\n")
}

var syntheticPrefixes = []string{"<command-name>", "<command-message>", "<local-command", "Caveat:", "[Request interrupted", "<task-notification>", "<bash-"}

func cleanRequest(text string) string {
	for {
		start := strings.Index(text, "<system-reminder>")
		if start < 0 {
			break
		}
		end := strings.Index(text[start:], "</system-reminder>")
		if end < 0 {
			text = text[:start]
			break
		}
		text = text[:start] + text[start+end+len("</system-reminder>"):]
	}
	text = strings.TrimSpace(text)
	for _, prefix := range syntheticPrefixes {
		if strings.HasPrefix(text, prefix) {
			return ""
		}
	}
	return text
}
