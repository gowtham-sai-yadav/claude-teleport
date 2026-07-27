// Package manifest defines the description that travels inside every bundle.
// It records the truth that the lossy folder names throw away: each project's
// real absolute path on the source machine.
package manifest

import "fmt"

const (
	Tool = "claude-teleport"

	// SchemaVersion is stamped into every bundle this build writes.
	//
	// MaxSupportedSchema is the highest a build knows how to read. They are
	// separate so a future release can emit a layout this one must refuse:
	// without that check an old binary silently misreads a newer bundle, and
	// because import writes into the user's home directory, misreading is not a
	// harmless no-op. Raise MaxSupportedSchema only alongside code that can
	// actually handle the new layout.
	// SchemaVersion is what a Claude Code bundle is stamped with, and it must stay
	// 1 forever: every released binary reads that layout, and bumping it would
	// make older ones refuse bundles they can in fact handle.
	SchemaVersion = 1

	// SchemaVersionAgent is stamped on a bundle carrying another coding tool's
	// session. It is higher than SchemaVersion on purpose, so a binary released
	// before multi-tool support refuses the bundle outright instead of unpacking a
	// layout it does not understand into someone's home directory.
	SchemaVersionAgent = 2

	MaxSupportedSchema = 2

	// KindFull is a whole-machine backup; KindSession is a single shared session.
	KindFull    = "full"
	KindSession = "session"
)

type Project struct {
	OriginalPath  string `json:"originalPath"`
	EncodedFolder string `json:"encodedFolder"`
	Sessions      int    `json:"sessions"`
	HasMemory     bool   `json:"hasMemory"`
	PathSource    string `json:"pathSource"` // claude.json | transcript | unknown
}

type Source struct {
	OS            string `json:"os"`
	Home          string `json:"home"`
	ClaudeVersion string `json:"claudeVersion,omitempty"`
}

type Manifest struct {
	Tool          string    `json:"tool"`
	ToolVersion   string    `json:"toolVersion"`
	SchemaVersion int       `json:"schemaVersion"`
	CreatedAt     string    `json:"createdAt"`
	Kind          string    `json:"kind,omitempty"`      // "" or "full" = whole backup; "session" = one shared session
	SessionID     string    `json:"sessionId,omitempty"` // set only for Kind == "session"
	Redacted      bool      `json:"redacted,omitempty"`  // secrets were scrubbed before packing
	Source        Source    `json:"source"`
	Includes      []string  `json:"includes"`
	Projects      []Project `json:"projects"`

	// Agent names the coding tool whose session this is: "codex", "opencode", and
	// so on. It is omitted for Claude Code, both because that is what every
	// existing bundle looks like and so a Claude bundle keeps serialising exactly
	// as it did before this field existed. Read it through AgentID, never directly.
	Agent string `json:"agent,omitempty"`

	// AgentVersion is the version of that tool on the sending machine, recorded so
	// a confusing import can be diagnosed later. Advisory only.
	AgentVersion string `json:"agentVersion,omitempty"`

	// ProjectPath is the absolute directory the session belonged to on the sending
	// machine. Claude bundles carry this in Projects; tools that do not organise
	// sessions by project need somewhere plain to put it.
	ProjectPath string `json:"projectPath,omitempty"`
}

// AgentID returns the coding tool this bundle came from, defaulting to Claude
// Code. Every bundle written before the field existed is a Claude bundle, so an
// empty value has to keep meaning that - the same convention Kind already uses.
func (m Manifest) AgentID() string {
	if m.Agent == "" {
		return "claude-code"
	}
	return m.Agent
}

// IsForeignAgent reports whether this bundle carries a tool other than Claude
// Code, and therefore needs that tool's provider to unpack it.
func (m Manifest) IsForeignAgent() bool { return m.Agent != "" && m.Agent != "claude-code" }

// IsSession reports whether this bundle carries a single shared session.
func (m Manifest) IsSession() bool { return m.Kind == KindSession }

// Unsupported reports whether this bundle was written by a newer build whose
// layout we cannot be trusted to read, with a message that tells the user what
// to do about it. An absent schemaVersion (0) means an early bundle, which is
// the layout this build already understands, so it is accepted.
func (m Manifest) Unsupported() error {
	if m.SchemaVersion > MaxSupportedSchema {
		return &UnsupportedSchemaError{Got: m.SchemaVersion, Max: MaxSupportedSchema}
	}
	return nil
}

// UnsupportedSchemaError is returned for a bundle from a newer release.
type UnsupportedSchemaError struct {
	Got, Max int
}

func (e *UnsupportedSchemaError) Error() string {
	return fmt.Sprintf("this bundle was made by a newer claude-teleport (bundle format %d, this build reads up to %d). "+
		"Run `claude-teleport update` and try again.", e.Got, e.Max)
}
