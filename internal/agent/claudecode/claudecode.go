// Package claudecode implements the agent.Provider contract for Claude Code.
//
// It delegates to internal/claudedir rather than reimplementing anything. That
// package is the version of this logic that has actually been run against real
// user data across three operating systems, so the provider abstraction is
// layered over it instead of replacing it: the seam is new, the behaviour is not.
//
// Importing this package registers the provider.
package claudecode

import (
	"github.com/gowtham-sai-yadav/claude-teleport/internal/agent"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
)

// Keys for the Claude-specific locations carried in agent.Roots.Extra. Only this
// package reads them; they exist so agent.Roots does not need a field per tool.
const (
	KeyJSONPath    = "claudeJSONPath" // ~/.claude.json
	KeyProjectsDir = "projectsDir"    // ~/.claude/projects
)

// Provider is the Claude Code implementation of agent.Provider.
type Provider struct{}

func init() { agent.Register(Provider{}) }

func (Provider) ID() agent.ID { return agent.ClaudeCode }

func (Provider) DisplayName() string { return "Claude Code" }

// Locate resolves ~/.claude (honouring an override, then CLAUDE_CONFIG_DIR).
//
// Presence is judged by the projects directory rather than the config directory:
// `claude --version` alone creates ~/.claude with no history in it, and a tool
// with no sessions is not a tool worth listing. An override is always treated as
// present, because the user naming a directory is a statement of intent - and it
// keeps --config-dir usable for a fresh import target that does not exist yet.
func (Provider) Locate(override string) (agent.Roots, bool, error) {
	p, err := claudedir.Locate(override)
	if err != nil {
		return agent.Roots{}, false, err
	}
	roots := Wrap(p)
	if override != "" {
		return roots, true, nil
	}
	return roots, claudedir.Exists(p), nil
}

// ListSessions returns Claude Code's sessions as agent.Session values, keeping
// each claudedir.Session in Ref so a later phase can pack one without re-reading
// the disk to find it.
func (Provider) ListSessions(r agent.Roots) ([]agent.Session, error) {
	sessions, err := claudedir.ListSessions(Unwrap(r))
	if err != nil {
		return nil, err
	}
	out := make([]agent.Session, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, agent.Session{
			Provider:    agent.ClaudeCode,
			ID:          s.ID,
			ShortID:     s.ShortID(),
			ProjectPath: s.ProjectPath,
			GroupKey:    s.Folder,
			Title:       s.Title,
			Messages:    s.Messages,
			ModTime:     s.ModTime,
			Size:        s.Size,
			Ref:         s,
		})
	}
	return out, nil
}

// Wrap converts located Claude paths into the shared Roots shape.
func Wrap(p claudedir.Paths) agent.Roots {
	return agent.Roots{
		Home:      p.Home,
		ConfigDir: p.ConfigDir,
		Extra: map[string]string{
			KeyJSONPath:    p.JSONPath,
			KeyProjectsDir: p.ProjectsDir,
		},
	}
}

// Unwrap recovers the Claude paths from Roots. Callers that still speak
// claudedir directly (the exporter and importer, for now) use this at the
// boundary so there is exactly one place the two representations meet.
func Unwrap(r agent.Roots) claudedir.Paths {
	return claudedir.Paths{
		Home:        r.Home,
		ConfigDir:   r.ConfigDir,
		JSONPath:    r.Get(KeyJSONPath),
		ProjectsDir: r.Get(KeyProjectsDir),
	}
}

// SessionRef recovers the claudedir.Session a listed session came from. ok is
// false if the session did not come from this provider.
func SessionRef(s agent.Session) (claudedir.Session, bool) {
	cs, ok := s.Ref.(claudedir.Session)
	return cs, ok
}
