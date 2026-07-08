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
	"github.com/gowtham-sai-yadav/claude-teleport/internal/redact"
)

// ShareOptions configures packing a single session for a teammate.
type ShareOptions struct {
	ConfigDir     string
	Version       string
	Out           string
	SessionPrefix string // id prefix to share (ignored if Last)
	Last          bool   // share the most recent session instead
	WithContext   bool   // also include the project's memory files
	Redact        bool   // scrub likely secrets before packing
	AssumeYes     bool   // skip the confirmation callback
	Confirm       func(SharePreview) bool
}

// SharePreview is what the caller shows the user before anything is written.
type SharePreview struct {
	Title         string
	ShortID       string
	ProjectPath   string
	Messages      int
	Bytes         int64 // uncompressed transcript size
	SecretsMasked int
	WithContext   bool
	Redact        bool
}

// ShareResult reports what a completed share produced.
type ShareResult struct {
	Path          string
	SessionID     string
	ProjectPath   string
	Messages      int
	Bytes         int64 // output file size
	SecretsMasked int
}

type packItem struct {
	name string
	data []byte
}

// RunShare finds one session, scrubs it, previews it, and (on confirmation)
// writes a single-session bundle a teammate can import.
func RunShare(opts ShareOptions) error {
	p, err := claudedir.Locate(opts.ConfigDir)
	if err != nil {
		return err
	}

	var sess claudedir.Session
	if opts.Last {
		sess, err = claudedir.LastSession(p)
	} else {
		sess, err = claudedir.FindSession(p, opts.SessionPrefix)
	}
	if err != nil {
		return err
	}
	if sess.ProjectPath == "" {
		return fmt.Errorf("this session's project path could not be recovered, so a teammate could not re-home it; nothing was written")
	}

	prefix := "projects/" + sess.Folder + "/"
	var items []packItem
	masked := 0

	// The transcript itself.
	tb, err := os.ReadFile(sess.File)
	if err != nil {
		return err
	}
	tb, n := maybeScrub(tb, opts.Redact)
	masked += n
	items = append(items, packItem{prefix + sess.ID + ".jsonl", tb})

	// Sidecar directory (subagent transcripts), if any.
	if sess.SidecarDir != "" {
		sub, n, err := gatherTree(sess.SidecarDir, prefix+sess.ID, opts.Redact)
		if err != nil {
			return err
		}
		masked += n
		items = append(items, sub...)
	}

	// Project memory, only when explicitly asked for.
	memDir := filepath.Join(sess.FolderPath, "memory")
	includeMemory := opts.WithContext && dirExists(memDir)
	if includeMemory {
		mem, n, err := gatherTree(memDir, prefix+"memory", opts.Redact)
		if err != nil {
			return err
		}
		masked += n
		items = append(items, mem...)
	}

	title := sess.Title
	if title == "" {
		title = "(untitled session)"
	}

	preview := SharePreview{
		Title:         title,
		ShortID:       sess.ShortID(),
		ProjectPath:   sess.ProjectPath,
		Messages:      sess.Messages,
		Bytes:         sess.Size,
		SecretsMasked: masked,
		WithContext:   includeMemory,
		Redact:        opts.Redact,
	}
	if !opts.AssumeYes && opts.Confirm != nil {
		if !opts.Confirm(preview) {
			fmt.Println("Aborted - nothing was written.")
			return nil
		}
	}

	man := manifest.Manifest{
		Tool:          manifest.Tool,
		ToolVersion:   opts.Version,
		SchemaVersion: manifest.SchemaVersion,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		Kind:          manifest.KindSession,
		SessionID:     sess.ID,
		Redacted:      opts.Redact,
		Source:        manifest.Source{OS: runtime.GOOS, Home: p.Home},
		Includes:      []string{"projects/"},
		Projects: []manifest.Project{{
			OriginalPath:  sess.ProjectPath,
			EncodedFolder: sess.Folder,
			Sessions:      1,
			HasMemory:     includeMemory,
			PathSource:    "session",
		}},
	}
	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}

	out := opts.Out
	if out == "" {
		out = fmt.Sprintf("claude-teleport-session-%s.tgz", sess.ShortID())
	}

	w, err := bundle.Create(out)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			w.Close()
		}
	}()
	if err := w.AddBytes("manifest.json", mb); err != nil {
		return err
	}
	for _, it := range items {
		if err := w.AddBytes(it.name, it.data); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	committed = true

	res := ShareResult{Path: out, SessionID: sess.ID, ProjectPath: sess.ProjectPath, Messages: sess.Messages, SecretsMasked: masked}
	if fi, err := os.Stat(out); err == nil {
		res.Bytes = fi.Size()
	}
	printShareResult(res, opts.Redact)
	return nil
}

func printShareResult(res ShareResult, redacted bool) {
	fmt.Printf("Shared session %s -> %s (%.1f MB)\n", shortID(res.SessionID), res.Path, float64(res.Bytes)/(1024*1024))
	if redacted {
		fmt.Printf("Masked %d likely secret(s). This is best effort - open the file if you want to be sure.\n", res.SecretsMasked)
	} else {
		fmt.Println("WARNING: secrets were NOT scrubbed (--no-redact). The raw transcript is in this file.")
	}
	fmt.Println("Your teammate imports it from inside their copy of the project:")
	fmt.Printf("  cd <their-project-dir> && claude-teleport import %s\n", res.Path)
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func maybeScrub(b []byte, do bool) ([]byte, int) {
	if !do {
		return b, 0
	}
	return redact.Scrub(b)
}

// gatherTree reads every file under srcDir, optionally scrubs it, and returns
// pack items rooted at prefix. It returns the total number of secrets masked.
func gatherTree(srcDir, prefix string, doRedact bool) ([]packItem, int, error) {
	var items []packItem
	masked := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
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
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		data, n := maybeScrub(data, doRedact)
		masked += n
		items = append(items, packItem{prefix + "/" + filepath.ToSlash(rel), data})
		return nil
	})
	return items, masked, err
}
