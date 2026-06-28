// Package cli wires the export/import/inspect subcommands together.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/exporter"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/importer"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/webui"
)

const Version = "0.1.0"

func Run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	switch args[0] {
	case "export":
		return runExport(args[1:])
	case "import":
		return runImport(args[1:])
	case "inspect":
		return runInspect(args[1:])
	case "verify":
		return runVerify(args[1:])
	case "gui":
		return runGUI(args[1:])
	case "version", "-v", "--version":
		fmt.Println("claude-teleport", Version)
		return nil
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: export, import, inspect, verify, gui)", args[0])
	}
}

func printHelp() {
	fmt.Print("claude-teleport " + Version + " - move your Claude Code history between machines\n\n" +
		"USAGE:\n" +
		"  claude-teleport export  [--out FILE] [--config-dir DIR]\n" +
		"  claude-teleport import  <bundle> [--dry-run] [--map OLD=NEW]... [--project P]... [--target-os OS] [--overwrite] [--deep] [--yes]\n" +
		"  claude-teleport inspect <bundle>\n" +
		"  claude-teleport verify  [--config-dir DIR]\n" +
		"  claude-teleport gui     [bundle] [--port N]\n\n" +
		"EXPORT runs on the OLD machine and writes a portable bundle.\n" +
		"IMPORT runs on the NEW machine and restores it, translating paths for this OS\n" +
		"(Linux, macOS, or Windows - drive letters and backslashes handled).\n" +
		"GUI opens a point-and-click wizard in your browser. VERIFY checks that migrated\n" +
		"sessions are resume-ready. Your login is never copied - log in once after importing.\n")
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("out", "", "output bundle path")
	cfg := fs.String("config-dir", "", "override Claude config dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := exporter.Run(exporter.Options{Out: *out, ConfigDir: *cfg, Version: Version})
	if err != nil {
		return err
	}
	fmt.Printf("Exported %d project(s), %d session(s) -> %s (%.1f MB)\n",
		res.Projects, res.Sessions, res.Path, float64(res.Bytes)/(1024*1024))
	if res.UnknownPaths > 0 {
		fmt.Printf("Note: %d folder(s) had no recoverable path; they import under their original name.\n", res.UnknownPaths)
	}
	return nil
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func runImport(args []string) error {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	dry := fs.Bool("dry-run", false, "show the plan without writing anything")
	overwrite := fs.Bool("overwrite", false, "overwrite existing files (backs up first)")
	deep := fs.Bool("deep", false, "rewrite old paths everywhere in transcripts, not just cwd")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	home := fs.String("target-home", "", "override the target home directory")
	tos := fs.String("target-os", "", "render paths for this OS: linux|darwin|windows (default: this machine)")
	cfg := fs.String("config-dir", "", "override the target Claude config dir")
	var maps multiFlag
	fs.Var(&maps, "map", "remap OLD=NEW path prefix (repeatable)")
	var projects multiFlag
	fs.Var(&projects, "project", "import only this project, by path or folder (repeatable; default: all)")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("usage: claude-teleport import <bundle> [flags]")
	}
	parsed, err := parseMaps(maps)
	if err != nil {
		return err
	}
	return importer.Run(importer.Options{
		Bundle:     pos[0],
		TargetHome: *home,
		TargetOS:   *tos,
		ConfigDir:  *cfg,
		DryRun:     *dry,
		Overwrite:  *overwrite,
		Deep:       *deep,
		AssumeYes:  *yes,
		Maps:       parsed,
		Projects:   projects,
	})
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	cfg := fs.String("config-dir", "", "override the Claude config dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tp, err := claudedir.Locate(*cfg)
	if err != nil {
		return err
	}
	results := importer.VerifyDir(tp)
	if len(results) == 0 {
		fmt.Println("No projects found under", tp.ProjectsDir)
		return nil
	}
	ok := 0
	for _, v := range results {
		status := "ok"
		if !v.OK {
			status = "FAIL: " + v.Detail
		} else {
			ok++
		}
		fmt.Printf("  [%s] %s  (%d session(s))\n", status, v.Folder, v.Sessions)
	}
	fmt.Printf("\n%d/%d project(s) resume-ready.\n", ok, len(results))
	return nil
}

func runGUI(args []string) error {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	port := fs.Int("port", 0, "port to listen on (0 = pick a free one)")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	bundlePath := ""
	if len(pos) > 0 {
		bundlePath = pos[0]
	}
	return webui.Serve(*port, bundlePath)
}

// parseInterleaved lets flags and positional arguments appear in any order.
// Go's flag package stops at the first positional, so we consume one
// positional at a time and re-parse the remainder.
func parseInterleaved(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		positionals = append(positionals, rest[0])
		args = rest[1:]
	}
	return positionals, nil
}

func parseMaps(in []string) ([]paths.Mapping, error) {
	var out []paths.Mapping
	for _, s := range in {
		i := strings.IndexByte(s, '=')
		if i < 0 {
			return nil, fmt.Errorf("bad --map %q (want OLD=NEW)", s)
		}
		out = append(out, paths.Mapping{Old: s[:i], New: s[i+1:]})
	}
	return out, nil
}

func runInspect(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: claude-teleport inspect <bundle>")
	}
	mb, err := bundle.ReadManifest(args[0])
	if err != nil {
		return err
	}
	if len(mb) == 0 {
		return fmt.Errorf("no manifest.json found - is %q a claude-teleport bundle?", args[0])
	}
	var man manifest.Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		return err
	}
	fmt.Printf("tool        : %s %s\n", man.Tool, man.ToolVersion)
	fmt.Printf("created     : %s\n", man.CreatedAt)
	fmt.Printf("source OS   : %s\n", man.Source.OS)
	fmt.Printf("source home : %s\n", man.Source.Home)
	fmt.Printf("includes    : %s\n", strings.Join(man.Includes, ", "))
	fmt.Printf("projects    : %d\n", len(man.Projects))
	for _, p := range man.Projects {
		path := p.OriginalPath
		if path == "" {
			path = "(unknown) " + p.EncodedFolder
		}
		fmt.Printf("  - %s  [%d session(s)]\n", path, p.Sessions)
	}
	return nil
}
