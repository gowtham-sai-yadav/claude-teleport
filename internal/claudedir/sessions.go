package claudedir

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Session is one recorded conversation: a <id>.jsonl transcript, the project
// it belongs to, and an optional <id>/ sidecar directory (subagent logs).
type Session struct {
	ID          string    // full session id (the transcript filename without .jsonl)
	Folder      string    // encoded project folder the session lives in
	FolderPath  string    // absolute path to that folder on disk
	ProjectPath string    // the project's true absolute path ("" if unknown)
	File        string    // absolute path to the <id>.jsonl transcript
	SidecarDir  string    // absolute path to the <id>/ directory, or "" if none
	Messages    int       // count of user/assistant turns (best effort)
	Title       string    // first human prompt, trimmed for display
	ModTime     time.Time // transcript last-modified time
	Size        int64     // transcript size in bytes
}

// ShortID is the first 8 characters of the id, enough to pick one out of a list.
func (s Session) ShortID() string {
	if len(s.ID) >= 8 {
		return s.ID[:8]
	}
	return s.ID
}

// ListSessions returns every session across all projects, newest first.
func ListSessions(p Paths) ([]Session, error) {
	projects, err := Discover(p)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, pr := range projects {
		entries, err := os.ReadDir(pr.FolderPath)
		if err != nil {
			continue
		}
		// Collect sidecar directory names so we can attach them to their session.
		sidecars := map[string]bool{}
		for _, e := range entries {
			if e.IsDir() && e.Name() != "memory" {
				sidecars[e.Name()] = true
			}
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".jsonl")
			file := filepath.Join(pr.FolderPath, e.Name())
			s := Session{
				ID:          id,
				Folder:      pr.Folder,
				FolderPath:  pr.FolderPath,
				ProjectPath: pr.OriginalPath,
				File:        file,
			}
			if sidecars[id] {
				s.SidecarDir = filepath.Join(pr.FolderPath, id)
			}
			if fi, err := e.Info(); err == nil {
				s.ModTime = fi.ModTime()
				s.Size = fi.Size()
			}
			s.Messages, s.Title = sessionMeta(file)
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// FindSession resolves an id prefix to exactly one session, erroring clearly on
// no match or an ambiguous prefix. An optional project filter (matched against
// the project path or encoded folder) narrows the search, which is how you pick
// when the same session id exists under more than one project.
func FindSession(p Paths, prefix, project string) (Session, error) {
	sessions, err := ListSessions(p)
	if err != nil {
		return Session{}, err
	}
	var matches []Session
	for _, s := range sessions {
		if prefix != "" && s.ID != prefix && !strings.HasPrefix(s.ID, prefix) {
			continue
		}
		if project != "" && !sessionInProject(s, project) {
			continue
		}
		matches = append(matches, s)
	}
	switch len(matches) {
	case 0:
		if project != "" {
			return Session{}, fmt.Errorf("no session matches %q in a project matching %q (run `claude-teleport sessions` to list them)", prefix, project)
		}
		return Session{}, fmt.Errorf("no session matches %q (run `claude-teleport sessions` to list them)", prefix)
	case 1:
		return matches[0], nil
	default:
		var lines []string
		for _, m := range matches {
			proj := m.ProjectPath
			if proj == "" {
				proj = m.Folder
			}
			lines = append(lines, fmt.Sprintf("%s  %s", m.ShortID(), proj))
		}
		return Session{}, fmt.Errorf("%q matches %d sessions; narrow it with --project:\n  %s", prefix, len(matches), strings.Join(lines, "\n  "))
	}
}

// sessionInProject reports whether a session's project path or encoded folder
// contains needle (case-insensitive).
func sessionInProject(s Session, needle string) bool {
	needle = strings.ToLower(needle)
	return strings.Contains(strings.ToLower(s.ProjectPath), needle) ||
		strings.Contains(strings.ToLower(s.Folder), needle)
}

// LastSession returns the most recently modified session, if any exist.
func LastSession(p Paths) (Session, error) {
	sessions, err := ListSessions(p)
	if err != nil {
		return Session{}, err
	}
	if len(sessions) == 0 {
		return Session{}, fmt.Errorf("no sessions found under %s", p.ProjectsDir)
	}
	return sessions[0], nil
}

// sessionMeta scans a transcript for a rough message count and the first human
// prompt to use as a title. It skips very long lines (tool output can be huge)
// and gives up after a bounded number of lines so listing stays fast.
func sessionMeta(path string) (messages int, title string) {
	f, err := os.Open(path)
	if err != nil {
		return 0, ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for i := 0; i < 5000; i++ {
		line, err := r.ReadString('\n')
		if len(line) > 0 && len(line) < 1<<20 {
			var ev struct {
				Type    string          `json:"type"`
				Message json.RawMessage `json:"message"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &ev) == nil {
				if ev.Type == "user" || ev.Type == "assistant" {
					messages++
				}
				if title == "" && ev.Type == "user" {
					title = firstPrompt(ev.Message)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return messages, title
}

// firstPrompt pulls displayable text out of a user message, whether its content
// is a plain string or an array of content blocks. It ignores tool results and
// bracketed system reminders so the title reads like something a person typed.
func firstPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return ""
	}
	var text string
	if json.Unmarshal(msg.Content, &text) != nil {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return ""
		}
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				text = b.Text
				break
			}
		}
	}
	return cleanTitle(text)
}

func cleanTitle(s string) string {
	s = strings.TrimSpace(s)
	// Skip content that is only a system-reminder/command tag, not a real prompt.
	if strings.HasPrefix(s, "<") {
		return ""
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	// Truncate by runes, not bytes, so a multibyte character at the cut point
	// is never sliced in half into a garbled glyph.
	const max = 60
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "..."
	}
	return s
}
