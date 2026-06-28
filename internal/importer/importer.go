// Package importer restores a bundle onto the new machine: it works out where
// each project should now live, renames the encoded folders to match, rewrites
// the old paths baked into transcripts, and merges everything in without
// destroying what is already there.
//
// The work is split so the CLI and the web GUI share one code path:
//
//	LoadManifest  -> read the bundle's manifest
//	BuildPlan     -> decide new paths, mappings, and which projects are selected
//	Import        -> execute the plan and return structured results + verification
//	VerifyDir     -> sanity-check that migrated sessions are resume-ready
package importer

import (
	"archive/tar"
	"bufio"
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

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
)

type Options struct {
	Bundle     string
	TargetHome string
	TargetOS   string // rendering style for rewritten paths; defaults to runtime.GOOS
	ConfigDir  string
	DryRun     bool
	Overwrite  bool
	Deep       bool
	AssumeYes  bool
	Maps       []paths.Mapping
	Projects   []string // selection: match by original path or encoded folder; empty = all
}

// PlanItem is one project's before/after, ready to show a user.
type PlanItem struct {
	OldPath   string `json:"oldPath"`
	NewPath   string `json:"newPath"`
	OldEnc    string `json:"oldEnc"`
	NewEnc    string `json:"newEnc"`
	Sessions  int    `json:"sessions"`
	HasMemory bool   `json:"hasMemory"`
	Selected  bool   `json:"selected"`
}

// PlanResult is the full preview of an import: where things go and how paths
// are remapped. It is JSON-serialisable for the GUI.
type PlanResult struct {
	SourceOS   string          `json:"sourceOS"`
	SourceHome string          `json:"sourceHome"`
	TargetOS   string          `json:"targetOS"`
	TargetHome string          `json:"targetHome"`
	Mappings   []paths.Mapping `json:"mappings"`
	Items      []PlanItem      `json:"items"`
	Unmatched  []string        `json:"unmatched"` // --project values that matched nothing
}

// RunResult is what an executed import produced.
type RunResult struct {
	Written        int            `json:"written"`
	Skipped        int            `json:"skipped"`
	MergedProjects int            `json:"mergedProjects"`
	Verify         []VerifyResult `json:"verify"`
}

// VerifyResult reports whether a migrated project looks resume-ready.
type VerifyResult struct {
	Folder   string `json:"folder"`
	Path     string `json:"path"`
	Sessions int    `json:"sessions"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
}

// LoadManifest reads and parses the manifest from a bundle.
func LoadManifest(bundlePath string) (manifest.Manifest, error) {
	mb, err := bundle.ReadManifest(bundlePath)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	if len(mb) == 0 {
		return manifest.Manifest{}, fmt.Errorf("no manifest.json found — is %q a claude-teleport bundle?", bundlePath)
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		return manifest.Manifest{}, fmt.Errorf("parse manifest: %w", err)
	}
	return man, nil
}

func resolveTargets(tp claudedir.Paths, opts Options) (home, osName string) {
	home = opts.TargetHome
	if home == "" {
		home = tp.Home
	}
	osName = opts.TargetOS
	if osName == "" {
		osName = runtime.GOOS
	}
	return home, osName
}

// BuildPlan computes the full preview without touching the filesystem.
func BuildPlan(man manifest.Manifest, tp claudedir.Paths, opts Options) *PlanResult {
	targetHome, targetOS := resolveTargets(tp, opts)
	mappings := buildMappings(man, targetHome, opts.Maps)
	selEnc, all, unmatched := resolveSelection(man, opts.Projects)

	res := &PlanResult{
		SourceOS:   man.Source.OS,
		SourceHome: man.Source.Home,
		TargetOS:   targetOS,
		TargetHome: targetHome,
		Mappings:   mappings,
		Unmatched:  unmatched,
	}
	for _, pr := range man.Projects {
		it := PlanItem{OldPath: pr.OriginalPath, OldEnc: pr.EncodedFolder, Sessions: pr.Sessions, HasMemory: pr.HasMemory}
		if pr.OriginalPath == "" {
			it.NewEnc = pr.EncodedFolder // can't remap a path we never recovered
		} else {
			it.NewPath = paths.Remap(pr.OriginalPath, mappings, targetOS)
			it.NewEnc = paths.Encode(it.NewPath)
		}
		it.Selected = all || selEnc[pr.EncodedFolder]
		res.Items = append(res.Items, it)
	}
	return res
}

// resolveSelection turns --project values (matched against original path or
// encoded folder) into a set of encoded folders. Empty selection means all.
func resolveSelection(man manifest.Manifest, sel []string) (selEnc map[string]bool, all bool, unmatched []string) {
	if len(sel) == 0 {
		return nil, true, nil
	}
	selEnc = map[string]bool{}
	for _, v := range sel {
		matched := false
		for _, pr := range man.Projects {
			if v == pr.OriginalPath || v == pr.EncodedFolder {
				selEnc[pr.EncodedFolder] = true
				matched = true
			}
		}
		if !matched {
			unmatched = append(unmatched, v)
		}
	}
	return selEnc, false, unmatched
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
		// Normalise the key to forward slashes (and drop a trailing separator)
		// so a Windows source home "C:\Users\bob" and a derived root
		// "C:/Users/bob" collapse to one rule instead of two.
		key := strings.TrimRight(paths.NormSep(old), "/")
		if key == "" {
			return
		}
		if _, ok := byOld[key]; ok {
			return // first writer wins (explicit rules are added first)
		}
		byOld[key] = nw
		order = append(order, key)
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

// Import executes the plan and returns structured results. It does not print.
func Import(tp claudedir.Paths, plan *PlanResult, opts Options) (*RunResult, error) {
	byOldEnc := map[string]PlanItem{}
	selEnc := map[string]bool{}
	selOrig := map[string]bool{}
	for _, it := range plan.Items {
		byOldEnc[it.OldEnc] = it
		if it.Selected {
			selEnc[it.OldEnc] = true
			if it.OldPath != "" {
				selOrig[it.OldPath] = true
			}
		}
	}
	j := &job{
		opts:      opts,
		tp:        tp,
		byOldEnc:  byOldEnc,
		mappings:  plan.Mappings,
		targetOS:  plan.TargetOS,
		selectAll: len(opts.Projects) == 0,
		selEnc:    selEnc,
		selOrig:   selOrig,
	}
	if err := j.execute(); err != nil {
		return nil, err
	}

	res := &RunResult{Written: j.written, Skipped: j.skipped, MergedProjects: j.merged}
	var folders []string
	for _, it := range plan.Items {
		if it.Selected {
			folders = append(folders, it.NewEnc)
		}
	}
	res.Verify = verifyFolders(tp.ProjectsDir, folders)
	return res, nil
}

// VerifyDir scans every project folder under the target and reports whether
// each one looks resume-ready (its first transcript's cwd encodes back to the
// folder it lives in).
func VerifyDir(tp claudedir.Paths) []VerifyResult {
	entries, err := os.ReadDir(tp.ProjectsDir)
	if err != nil {
		return nil
	}
	var folders []string
	for _, e := range entries {
		if e.IsDir() {
			folders = append(folders, e.Name())
		}
	}
	return verifyFolders(tp.ProjectsDir, folders)
}

func verifyFolders(projectsDir string, folders []string) []VerifyResult {
	var out []VerifyResult
	seen := map[string]bool{}
	for _, f := range folders {
		if seen[f] {
			continue
		}
		seen[f] = true

		vr := VerifyResult{Folder: f}
		dir := filepath.Join(projectsDir, f)
		entries, err := os.ReadDir(dir)
		if err != nil {
			vr.Detail = "folder missing"
			out = append(out, vr)
			continue
		}
		var first string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
				vr.Sessions++
				if first == "" {
					first = filepath.Join(dir, e.Name())
				}
			}
		}
		switch {
		case vr.Sessions == 0:
			vr.Detail = "no sessions"
		default:
			cwd := claudedir.ReadCwd(first)
			if cwd == "" {
				vr.Detail = "no cwd recorded in transcript"
			} else {
				vr.Path = cwd
				if paths.Encode(cwd) == f {
					vr.OK = true
				} else {
					vr.Detail = "transcript cwd does not match its folder"
				}
			}
		}
		out = append(out, vr)
	}
	return out
}

// Run is the CLI entry point: it loads, previews, confirms, executes, and
// prints. The GUI calls BuildPlan/Import directly instead.
func Run(opts Options) error {
	man, err := LoadManifest(opts.Bundle)
	if err != nil {
		return err
	}
	tp, err := claudedir.Locate(opts.ConfigDir)
	if err != nil {
		return err
	}
	plan := BuildPlan(man, tp, opts)
	printPlan(plan)
	if len(plan.Unmatched) > 0 {
		fmt.Printf("\nWarning: --project value(s) matched nothing: %s\n", strings.Join(plan.Unmatched, ", "))
	}

	if opts.DryRun {
		fmt.Println("\nDry run — nothing was written.")
		return nil
	}
	if !opts.AssumeYes && !confirm("Proceed with import?") {
		fmt.Println("Aborted.")
		return nil
	}

	res, err := Import(tp, plan, opts)
	if err != nil {
		return err
	}
	fmt.Printf("\nDone. %d file(s) written, %d skipped (already present); %d project entr%s merged into .claude.json.\n",
		res.Written, res.Skipped, res.MergedProjects, plural(res.MergedProjects))
	printVerify(res.Verify)
	fmt.Println("\nIMPORTANT: your login was NOT transferred — credentials never are.")
	fmt.Println("Open Claude Code, log in once, then run `claude --resume` inside a project.")
	return nil
}

func printPlan(p *PlanResult) {
	selected := 0
	for _, it := range p.Items {
		if it.Selected {
			selected++
		}
	}
	fmt.Println("claude-teleport import")
	fmt.Printf("  bundle source : %s (home %s)\n", p.SourceOS, p.SourceHome)
	fmt.Printf("  this machine  : %s (home %s)\n", p.TargetOS, p.TargetHome)
	fmt.Printf("  projects      : %d selected of %d\n", selected, len(p.Items))

	fmt.Println("\nPath remapping:")
	for _, m := range p.Mappings {
		fmt.Printf("  %s  ->  %s\n", m.Old, m.New)
	}

	fmt.Println("\nProjects:")
	items := append([]PlanItem(nil), p.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].OldPath < items[j].OldPath })
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  \tOLD\tNEW\tSESSIONS")
	for _, it := range items {
		mark := " "
		if it.Selected {
			mark = "+"
		}
		old, nw := it.OldPath, it.NewPath
		if old == "" {
			old = "(unknown) " + it.OldEnc
			nw = "(folder kept as-is)"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\n", mark, old, nw, it.Sessions)
	}
	tw.Flush()
}

func printVerify(vs []VerifyResult) {
	if len(vs) == 0 {
		return
	}
	ok := 0
	for _, v := range vs {
		if v.OK {
			ok++
		}
	}
	fmt.Printf("\nVerify: %d/%d migrated project(s) look resume-ready.\n", ok, len(vs))
	for _, v := range vs {
		if !v.OK {
			fmt.Printf("  ! %s — %s\n", v.Folder, v.Detail)
		}
	}
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
	byOldEnc   map[string]PlanItem
	mappings   []paths.Mapping
	targetOS   string
	selectAll  bool
	selEnc     map[string]bool
	selOrig    map[string]bool
	written    int
	skipped    int
	merged     int
	claudeJSON []byte
}

func (j *job) selectedEnc(oldEnc string) bool { return j.selectAll || j.selEnc[oldEnc] }

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
	if !j.selectedEnc(oldEnc) {
		_, _ = io.Copy(io.Discard, r) // not selected for this import
		return nil
	}
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

	rw := newPathRewriter(oldPath, newPath, j.opts.Deep)
	br := bufio.NewReader(r)
	for {
		line, rerr := br.ReadBytes('\n')
		if len(line) > 0 {
			if _, err := bw.Write(rw.line(line)); err != nil {
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
		if !j.selectAll && !j.selOrig[k] {
			continue // project not selected for this import
		}
		nk := paths.Remap(k, j.mappings, j.targetOS)
		if _, exists := tgtProjects[nk]; exists && !j.opts.Overwrite {
			continue
		}
		tgtProjects[nk] = v
		added++
	}
	if added == 0 {
		return nil
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
	j.merged = added
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
