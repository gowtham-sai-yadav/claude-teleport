// Package claudedir locates Claude Code's data on disk and discovers the
// projects it has session history for, recovering each project's true
// absolute path (which the folder name alone cannot give us).
package claudedir

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
)

type Paths struct {
	Home        string
	ConfigDir   string // ~/.claude
	JSONPath    string // ~/.claude.json
	ProjectsDir string // ~/.claude/projects
}

// Locate resolves where Claude Code keeps its data, honouring an explicit
// override, then CLAUDE_CONFIG_DIR, then the default of ~/.claude.
func Locate(override string) (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	var configDir, jsonPath string
	switch {
	case override != "":
		configDir = override
		jsonPath = filepath.Join(override, ".claude.json")
	case os.Getenv("CLAUDE_CONFIG_DIR") != "":
		configDir = os.Getenv("CLAUDE_CONFIG_DIR")
		jsonPath = filepath.Join(configDir, ".claude.json")
	default:
		configDir = filepath.Join(home, ".claude")
		jsonPath = filepath.Join(home, ".claude.json")
	}
	return Paths{
		Home:        home,
		ConfigDir:   configDir,
		JSONPath:    jsonPath,
		ProjectsDir: filepath.Join(configDir, "projects"),
	}, nil
}

type Project struct {
	Folder       string // encoded folder name
	FolderPath   string // absolute path to the folder on disk
	OriginalPath string // recovered true absolute path ("" if unknown)
	Sessions     int
	HasMemory    bool
	Source       string // how OriginalPath was found
}

// Discover lists every project folder and recovers its real path, preferring
// the raw keys in .claude.json (which keep underscores/dots intact) and
// falling back to the cwd recorded inside a transcript.
func Discover(p Paths) ([]Project, error) {
	enc2orig := map[string]string{}
	if data, err := os.ReadFile(p.JSONPath); err == nil {
		var cj struct {
			Projects map[string]json.RawMessage `json:"projects"`
		}
		if json.Unmarshal(data, &cj) == nil {
			for orig := range cj.Projects {
				enc2orig[paths.Encode(orig)] = orig
			}
		}
	}

	entries, err := os.ReadDir(p.ProjectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Project
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		folder := e.Name()
		fp := filepath.Join(p.ProjectsDir, folder)
		orig := enc2orig[folder]
		src := "claude.json"
		if orig == "" {
			orig = recoverPath(fp)
			if orig == "" {
				src = "unknown"
			} else {
				src = "transcript"
			}
		}
		sessions, hasMem := scanProject(fp)
		out = append(out, Project{
			Folder:       folder,
			FolderPath:   fp,
			OriginalPath: orig,
			Sessions:     sessions,
			HasMemory:    hasMem,
			Source:       src,
		})
	}
	return out, nil
}

func scanProject(dir string) (sessions int, hasMemory bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, false
	}
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == "memory" {
				hasMemory = true
			}
			continue
		}
		if strings.HasSuffix(e.Name(), ".jsonl") {
			sessions++
		}
	}
	return sessions, hasMemory
}

func recoverPath(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if cwd := cwdFromFile(filepath.Join(dir, e.Name())); cwd != "" {
			return cwd
		}
	}
	return ""
}

// ReadCwd returns the working directory recorded in a transcript file, or ""
// if none is found. Exported for the verify step.
func ReadCwd(path string) string { return cwdFromFile(path) }

// cwdFromFile reads the early events of a transcript looking for the "cwd"
// field. It skips absurdly long lines (some events carry megabytes of tool
// output) because the cwd we want lives in the small header events.
func cwdFromFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	r := bufio.NewReader(f)
	for i := 0; i < 200; i++ {
		line, err := r.ReadString('\n')
		if len(line) > 0 && len(line) < 1<<20 {
			var m struct {
				Cwd string `json:"cwd"`
			}
			if json.Unmarshal([]byte(strings.TrimSpace(line)), &m) == nil && m.Cwd != "" {
				return m.Cwd
			}
		}
		if err != nil {
			break
		}
	}
	return ""
}
