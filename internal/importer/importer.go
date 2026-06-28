// Package importer restores a bundle onto the new machine: it works out where
// each project should now live, renames the encoded folders to match, rewrites
// the old paths baked into transcripts, and merges everything in without
// destroying what is already there.
package importer

import (
	"archive/tar"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"claude-port/internal/bundle"
	"claude-port/internal/claudedir"
	"claude-port/internal/manifest"
	"claude-port/internal/paths"
)

type Options struct {
	Bundle     string
	TargetHome string
	ConfigDir  string
	DryRun     bool
	Overwrite  bool
	Deep       bool
	AssumeYes  bool
	Maps       []paths.Mapping
}

type planItem struct {
	OldPath  string
	NewPath  string
	OldEnc   string
	NewEnc   string
	Sessions int
}

func Run(opts Options) error {
	mb, err := bundle.ReadManifest(opts.Bundle)
	if err != nil {
		return fmt.Errorf("read manifest: %w", err)
	}
	if len(mb) == 0 {
		return fmt.Errorf("no manifest.json found — is %q a claude-port bundle?", opts.Bundle)
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	tp, err := claudedir.Locate(opts.ConfigDir)
	if err != nil {
		return err
	}
	targetHome := opts.TargetHome
	if targetHome == "" {
		targetHome = tp.Home
	}
	targetOS := runtime.GOOS

	mappings := buildMappings(man, targetHome, opts.Maps)

	byOldEnc := map[string]planItem{}
	var plan []planItem
	for _, pr := range man.Projects {
		it := planItem{OldPath: pr.OriginalPath, OldEnc: pr.EncodedFolder, Sessions: pr.Sessions}
		if pr.OriginalPath == "" {
			it.NewEnc = pr.EncodedFolder // can't remap a path we never recovered
		} else {
			it.NewPath = paths.Remap(pr.OriginalPath, mappings, targetOS)
			it.NewEnc = paths.Encode(it.NewPath)
		}
		byOldEnc[pr.EncodedFolder] = it
		plan = append(plan, it)
	}

	printSummary(man, targetHome, targetOS, mappings, plan)

	if opts.DryRun {
		fmt.Println("\nDry run — nothing was written.")
		return nil
	}
	if !opts.AssumeYes && !confirm("Proceed with import?") {
		fmt.Println("Aborted.")
		return nil
	}

	j := &job{opts: opts, tp: tp, byOldEnc: byOldEnc, mappings: mappings, targetOS: targetOS}
	if err := j.execute(); err != nil {
		return err
	}

	fmt.Printf("\nDone. %d files written, %d skipped (already present).\n", j.written, j.skipped)
	fmt.Println("\nIMPORTANT: your login was NOT transferred — credentials never are.")
	fmt.Println("Open Claude Code, log in once, then run `claude --resume` inside a project.")
	return nil
}

// buildMappings produces the old->new path rules. Explicit --map rules win;
// then the bundle's source home; then any other home roots seen in the project
// list (so a profile that already mixes machines is handled in one pass).
func buildMappings(man manifest.Manifest, targetHome string, explicit []paths.Mapping) []paths.Mapping {
	byOld := map[string]string{}
	var order []string
	add := func(old, nw string) {
		if old == "" || nw == "" {
			return
		}
		if _, ok := byOld[old]; !ok {
			order = append(order, old)
		} else {
			return // first writer wins (explicit rules are added first)
		}
		byOld[old] = nw
	}
	for _, m := range explicit {
		add(m.Old, m.New)
	}
	add(man.Source.Home, targetHome)
	for _, pr := range man.Projects {
		if r := paths.HomeRoot(pr.OriginalPath); r != "" {
			add(r, targetHome)
		}
	}
	out := make([]paths.Mapping, 0, len(order))
	for _, o := range order {
		out = append(out, paths.Mapping{Old: o, New: byOld[o]})
	}
	return out
}

func printSummary(man manifest.Manifest, targetHome, targetOS string, mappings []paths.Mapping, plan []planItem) {
	fmt.Println("claude-port import")
	fmt.Printf("  bundle source : %s (home %s)\n", man.Source.OS, man.Source.Home)
	fmt.Printf("  this machine  : %s (home %s)\n", targetOS, targetHome)
	fmt.Printf("  projects      : %d\n", len(plan))

	fmt.Println("\nPath remapping:")
	for _, m := range mappings {
		fmt.Printf("  %s  ->  %s\n", m.Old, m.New)
	}

	fmt.Println("\nProjects:")
	sort.Slice(plan, func(i, j int) bool { return plan[i].OldPath < plan[j].OldPath })
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  OLD\tNEW\tSESSIONS")
	for _, it := range plan {
		old, nw := it.OldPath, it.NewPath
		if old == "" {
			old = "(unknown) " + it.OldEnc
			nw = "(folder kept as-is)"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%d\n", old, nw, it.Sessions)
	}
	tw.Flush()
}

func confirm(q string) bool {
	fmt.Printf("%s [y/N]: ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
}

type job struct {
	opts       Options
	tp         claudedir.Paths
	byOldEnc   map[string]planItem
	mappings   []paths.Mapping
	targetOS   string
	written    int
	skipped    int
	claudeJSON []byte
}

func (j *job) execute() error {
	err := bundle.ForEach(j.opts.Bundle, func(h *tar.Header, r io.Reader) error {
		name := h.Name
		switch {
		case name == "manifest.json":
			return nil
		case name == "config/claude.json":
			b, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			j.claudeJSON = b
			return nil
		case name == "config/history.jsonl":
			return j.writeHistory(filepath.Join(j.tp.ConfigDir, "history.jsonl"), r)
		case strings.HasPrefix(name, "config/"):
			base := strings.TrimPrefix(name, "config/")
			return j.writePlain(filepath.Join(j.tp.ConfigDir, filepath.FromSlash(base)), r)
		case strings.HasPrefix(name, "projects/"):
			return j.writeProjectEntry(name, r)
		case strings.HasPrefix(name, "plans/"), strings.HasPrefix(name, "plugins/"):
			return j.writePlain(filepath.Join(j.tp.ConfigDir, filepath.FromSlash(name)), r)
		default:
			return nil
		}
	})
	if err != nil {
		return err
	}
	return j.mergeClaudeJSON()
}

func (j *job) writeProjectEntry(name string, r io.Reader) error {
	rest := strings.TrimPrefix(name, "projects/")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return nil
	}
	oldEnc, sub := rest[:slash], rest[slash+1:]
	it, ok := j.byOldEnc[oldEnc]
	newEnc := oldEnc
	if ok {
		newEnc = it.NewEnc
	}
	dest := filepath.Join(j.tp.ProjectsDir, newEnc, filepath.FromSlash(sub))

	if strings.HasSuffix(sub, ".jsonl") && ok && it.OldPath != "" && it.NewPath != "" && it.OldPath != it.NewPath {
		return j.writeTranscript(dest, r, it.OldPath, it.NewPath)
	}
	return j.writePlain(dest, r)
}

// shouldWrite reports whether dest may be written. Existing files are left
// alone unless --overwrite, in which case the old file is backed up first.
func (j *job) shouldWrite(dest string) bool {
	if _, err := os.Stat(dest); err == nil {
		if !j.opts.Overwrite {
			return false
		}
		_ = os.Rename(dest, dest+".bak-"+time.Now().Format("20060102-150405"))
	}
	return true
}

func (j *job) writePlain(dest string, r io.Reader) error {
	if !j.shouldWrite(dest) {
		j.skipped++
		_, _ = io.Copy(io.Discard, r)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return err
	}
	j.written++
	return nil
}

// writeTranscript streams a transcript out line by line, replacing the old
// project path in the structural "cwd" field (and, with --deep, everywhere).
func (j *job) writeTranscript(dest string, r io.Reader, oldPath, newPath string) error {
	if !j.shouldWrite(dest) {
		j.skipped++
		_, _ = io.Copy(io.Discard, r)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)

	cwdOldCompact := []byte(`"cwd":"` + oldPath + `"`)
	cwdNewCompact := []byte(`"cwd":"` + newPath + `"`)
	cwdOldSpaced := []byte(`"cwd": "` + oldPath + `"`)
	cwdNewSpaced := []byte(`"cwd": "` + newPath + `"`)
	oldBytes := []byte(oldPath)
	newBytes := []byte(newPath)

	br := bufio.NewReader(r)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.ReplaceAll(line, cwdOldCompact, cwdNewCompact)
			line = bytes.ReplaceAll(line, cwdOldSpaced, cwdNewSpaced)
			if j.opts.Deep {
				line = bytes.ReplaceAll(line, oldBytes, newBytes)
			}
			if _, err := bw.Write(line); err != nil {
				return err
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	j.written++
	return nil
}

func (j *job) writeHistory(dest string, r io.Reader) error {
	if !j.shouldWrite(dest) {
		j.skipped++
		_, _ = io.Copy(io.Discard, r)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriter(f)

	br := bufio.NewReader(r)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, err := bw.Write(j.remapHistoryLine(line)); err != nil {
				return err
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	j.written++
	return nil
}

func (j *job) remapHistoryLine(line []byte) []byte {
	trimmed := strings.TrimRight(string(line), "\n")
	if trimmed == "" {
		return line
	}
	var m map[string]any
	if json.Unmarshal([]byte(trimmed), &m) != nil {
		return line
	}
	if proj, ok := m["project"].(string); ok && proj != "" {
		m["project"] = paths.Remap(proj, j.mappings, j.targetOS)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return line
	}
	return append(b, '\n')
}

// mergeClaudeJSON folds the bundle's per-project config into the target's
// existing ~/.claude.json, re-keying each project to its new path and never
// clobbering the new machine's own identity fields.
func (j *job) mergeClaudeJSON() error {
	if j.claudeJSON == nil {
		return nil
	}
	var src map[string]any
	if json.Unmarshal(j.claudeJSON, &src) != nil {
		return nil
	}
	srcProjects, _ := src["projects"].(map[string]any)
	if srcProjects == nil {
		return nil
	}

	var target map[string]any
	if data, err := os.ReadFile(j.tp.JSONPath); err == nil {
		_ = json.Unmarshal(data, &target)
	}
	if target == nil {
		target = map[string]any{}
	}
	tgtProjects, _ := target["projects"].(map[string]any)
	if tgtProjects == nil {
		tgtProjects = map[string]any{}
	}

	added := 0
	for k, v := range srcProjects {
		nk := paths.Remap(k, j.mappings, j.targetOS)
		if _, exists := tgtProjects[nk]; exists && !j.opts.Overwrite {
			continue
		}
		tgtProjects[nk] = v
		added++
	}
	target["projects"] = tgtProjects

	out, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return err
	}
	if _, err := os.Stat(j.tp.JSONPath); err == nil {
		_ = copyFile(j.tp.JSONPath, j.tp.JSONPath+".bak-"+time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(filepath.Dir(j.tp.JSONPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(j.tp.JSONPath, out, 0o600); err != nil {
		return err
	}
	fmt.Printf("Merged %d project entr%s into %s\n", added, plural(added), j.tp.JSONPath)
	return nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
