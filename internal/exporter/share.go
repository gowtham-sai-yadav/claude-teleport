package exporter

import (
	"encoding/json"
	"fmt"
	"io"
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
	Project       string // optional project filter to disambiguate a shared id
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
// SessionBundle is a built, in-memory single-session bundle, ready to be
// written to a file or streamed over a transfer. Build it with PrepareShare.
type SessionBundle struct {
	// Name is a suggested filename, e.g. claude-teleport-session-<shortid>.tgz.
	Name string
	// Preview describes what the bundle contains, for a confirmation prompt.
	Preview SharePreview
	// Session is the resolved session this bundle was built from.
	Session claudedir.Session

	manifestJSON []byte
	items        []packItem
}

// WriteBundle writes the bundle to w: the manifest first (so it reads fast),
// then each file. It flushes the archive framing but does not close w. (Not
// named WriteTo, to avoid implying the io.WriterTo contract.)
func (b *SessionBundle) WriteBundle(w io.Writer) error {
	bw := bundle.NewWriter(w)
	if err := bw.AddBytes("manifest.json", b.manifestJSON); err != nil {
		return err
	}
	for _, it := range b.items {
		if err := bw.AddBytes(it.name, it.data); err != nil {
			return err
		}
	}
	return bw.Close()
}

// PrepareShare locates a session and builds its bundle in memory, scrubbing
// secrets, without writing anything. The caller chooses the destination: a file
// (RunShare) or a wormhole (the send command). This keeps the "what leaves your
// machine" logic in exactly one place.
func PrepareShare(opts ShareOptions) (*SessionBundle, error) {
	p, err := claudedir.Locate(opts.ConfigDir)
	if err != nil {
		return nil, err
	}

	var sess claudedir.Session
	if opts.Last {
		sess, err = claudedir.LastSession(p)
	} else {
		sess, err = claudedir.FindSession(p, opts.SessionPrefix, opts.Project)
	}
	if err != nil {
		return nil, err
	}
	if sess.ProjectPath == "" {
		return nil, fmt.Errorf("this session's project path could not be recovered, so a teammate could not re-home it")
	}

	prefix := "projects/" + sess.Folder + "/"
	var items []packItem
	masked := 0

	// The transcript itself.
	tb, err := os.ReadFile(sess.File)
	if err != nil {
		return nil, err
	}
	tb, n := maybeScrub(tb, opts.Redact)
	masked += n
	items = append(items, packItem{prefix + sess.ID + ".jsonl", tb})

	// Sidecar directory (subagent transcripts), if any.
	if sess.SidecarDir != "" {
		sub, n, err := gatherTree(sess.SidecarDir, prefix+sess.ID, opts.Redact)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		masked += n
		items = append(items, mem...)
	}

	title := sess.Title
	if title == "" {
		title = "(untitled session)"
	}

	// Total uncompressed size of everything packed (transcript, sidecar, and
	// memory if included), so the preview reflects what actually leaves the
	// machine rather than just the transcript.
	var contentBytes int64
	for _, it := range items {
		contentBytes += int64(len(it.data))
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
		return nil, err
	}

	name := opts.Out
	if name == "" {
		name = fmt.Sprintf("claude-teleport-session-%s.tgz", sess.ShortID())
	}

	return &SessionBundle{
		Name:    name,
		Session: sess,
		Preview: SharePreview{
			Title:         title,
			ShortID:       sess.ShortID(),
			ProjectPath:   sess.ProjectPath,
			Messages:      sess.Messages,
			Bytes:         contentBytes,
			SecretsMasked: masked,
			WithContext:   includeMemory,
			Redact:        opts.Redact,
		},
		manifestJSON: mb,
		items:        items,
	}, nil
}

// RunShare builds a single-session bundle, previews it, and on confirmation
// writes it to a file for a teammate to import.
func RunShare(opts ShareOptions) error {
	b, err := PrepareShare(opts)
	if err != nil {
		return err
	}
	if !opts.AssumeYes && opts.Confirm != nil {
		if !opts.Confirm(b.Preview) {
			fmt.Println("Aborted - nothing was written.")
			return nil
		}
	}

	out := b.Name
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := b.WriteBundle(f); err != nil {
		f.Close()
		os.Remove(out)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	res := ShareResult{
		Path:          out,
		SessionID:     b.Session.ID,
		ProjectPath:   b.Session.ProjectPath,
		Messages:      b.Session.Messages,
		SecretsMasked: b.Preview.SecretsMasked,
	}
	if fi, err := os.Stat(out); err == nil {
		res.Bytes = fi.Size()
	}
	printShareResult(res, opts.Redact)
	return nil
}

func printShareResult(res ShareResult, redacted bool) {
	fmt.Printf("Shared session %s -> %s (%s)\n", shortID(res.SessionID), res.Path, HumanSize(res.Bytes))
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

// HumanSize formats a byte count compactly, scaling the unit so small files
// read as "4 KB" instead of "0.0 MB". Bytes are shown as a whole number.
func HumanSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	case n < gb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
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
