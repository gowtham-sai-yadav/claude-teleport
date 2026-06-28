// Package manifest defines the description that travels inside every bundle.
// It records the truth that the lossy folder names throw away: each project's
// real absolute path on the source machine.
package manifest

const (
	Tool          = "claude-port"
	SchemaVersion = 1
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
	Source        Source    `json:"source"`
	Includes      []string  `json:"includes"`
	Projects      []Project `json:"projects"`
}
