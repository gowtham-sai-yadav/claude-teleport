// Package exporter packs a machine's portable Claude Code data into a bundle.
// It deliberately leaves out credentials and machine-local junk, and strips
// device identity from the copied config.
package exporter

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
)

type Options struct {
	Out       string
	ConfigDir string
	Version   string
}

type Result struct {
	Path         string
	Projects     int
	Sessions     int
	UnknownPaths int
	Bytes        int64
}

// dropKeys are fields in ~/.claude.json that identify the old device or are
// pure local UI/cache state. We never carry them to a new machine.
var dropKeys = []string{
	"machineID", "userID", "anonymousId", "oauthAccount",
	"customApiKeyResponses", "cachedGrowthBookFeatures",
	"cachedExperimentFeatures", "cachedChangelog", "tipsHistory",
	"tipLifetimeShownCounts", "numStartups", "firstStartTime",
	"fallbackAvailableWarningThreshold", "lastReleaseNotesSeen",
}

func Run(opts Options) (Result, error) {
	var res Result
	p, err := claudedir.Locate(opts.ConfigDir)
	if err != nil {
		return res, err
	}
	projects, err := claudedir.Discover(p)
	if err != nil {
		return res, err
	}

	out := opts.Out
	if out == "" {
		out = fmt.Sprintf("claude-teleport-backup-%s.tgz", time.Now().Format("20060102-150405"))
	}

	man := manifest.Manifest{
		Tool:          manifest.Tool,
		ToolVersion:   opts.Version,
		SchemaVersion: manifest.SchemaVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Source:        manifest.Source{OS: runtime.GOOS, Home: p.Home},
	}
	for _, pr := range projects {
		man.Projects = append(man.Projects, manifest.Project{
			OriginalPath:  pr.OriginalPath,
			EncodedFolder: pr.Folder,
			Sessions:      pr.Sessions,
			HasMemory:     pr.HasMemory,
			PathSource:    pr.Source,
		})
		res.Sessions += pr.Sessions
		if pr.OriginalPath == "" {
			res.UnknownPaths++
		}
	}
	res.Projects = len(projects)

	cfgItems := []struct{ rel, name string }{
		{"settings.json", "config/settings.json"},
		{"settings.local.json", "config/settings.local.json"},
		{"history.jsonl", "config/history.jsonl"},
		{"plugins/installed_plugins.json", "plugins/installed_plugins.json"},
		{"plugins/known_marketplaces.json", "plugins/known_marketplaces.json"},
	}

	// Precompute the include list so the manifest can be the first entry.
	var includes []string
	claudeJSONok := fileExists(p.JSONPath)
	if claudeJSONok {
		includes = append(includes, "config/claude.json")
	}
	for _, c := range cfgItems {
		if fileExists(filepath.Join(p.ConfigDir, c.rel)) {
			includes = append(includes, c.name)
		}
	}
	plansDir := filepath.Join(p.ConfigDir, "plans")
	plansOk := dirExists(plansDir)
	if plansOk {
		includes = append(includes, "plans/")
	}
	if len(projects) > 0 {
		includes = append(includes, "projects/")
	}
	man.Includes = includes

	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return res, err
	}

	w, err := bundle.Create(out)
	if err != nil {
		return res, err
	}
	committed := false
	defer func() {
		if !committed {
			w.Close()
		}
	}()

	if err := w.AddBytes("manifest.json", mb); err != nil {
		return res, err
	}
	if claudeJSONok {
		if data, err := os.ReadFile(p.JSONPath); err == nil {
			if san, err := sanitize(data); err == nil {
				if err := w.AddBytes("config/claude.json", san); err != nil {
					return res, err
				}
			}
		}
	}
	for _, c := range cfgItems {
		src := filepath.Join(p.ConfigDir, c.rel)
		if fileExists(src) {
			if err := w.AddFile(c.name, src); err != nil {
				return res, err
			}
		}
	}
	if plansOk {
		if err := addTree(w, plansDir, "plans"); err != nil {
			return res, err
		}
	}
	for _, pr := range projects {
		if err := addTree(w, pr.FolderPath, "projects/"+pr.Folder); err != nil {
			return res, err
		}
	}

	if err := w.Close(); err != nil {
		return res, err
	}
	committed = true

	if fi, err := os.Stat(out); err == nil {
		res.Bytes = fi.Size()
	}
	res.Path = out
	return res, nil
}

func sanitize(raw []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	for _, k := range dropKeys {
		delete(m, k)
	}
	return json.MarshalIndent(m, "", "  ")
}

func addTree(w *bundle.Writer, srcDir, prefix string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		return w.AddFile(prefix+"/"+filepath.ToSlash(rel), path)
	})
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
