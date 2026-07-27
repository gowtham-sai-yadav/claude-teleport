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
	SchemaVersion      = 1
	MaxSupportedSchema = 1

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
}

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
