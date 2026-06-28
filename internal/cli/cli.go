// Package cli wires the export/import/inspect subcommands together.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"claude-port/internal/bundle"
	"claude-port/internal/exporter"
	"claude-port/internal/importer"
	"claude-port/internal/manifest"
	"claude-port/internal/paths"
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
	case "version", "-v", "--version":
		fmt.Println("claude-port", Version)
		return nil
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: export, import, inspect)", args[0])
	}
}

func printHelp() {
	fmt.Print("claude-port " + Version + " — move your Claude Code history between machines\n\n" +
		"USAGE:\n" +
		"  claude-port export  [--out FILE] [--config-dir DIR]\n" +
		"  claude-port import  <bundle> [--dry-run] [--map OLD=NEW]... [--overwrite] [--deep] [--yes]\n" +
		"  claude-port inspect <bundle>\n\n" +
		"EXPORT runs on the OLD machine and writes a portable bundle.\n" +
		"IMPORT runs on the NEW machine and restores it, fixing paths for this OS.\n" +
		"Your login is never copied — log in once after importing.\n")
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
	cfg := fs.String("config-dir", "", "override the target Claude config dir")
	var maps multiFlag
	fs.Var(&maps, "map", "remap OLD=NEW path prefix (repeatable)")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("usage: claude-port import <bundle> [flags]")
	}
	parsed, err := parseMaps(maps)
	if err != nil {
		return err
	}
	return importer.Run(importer.Options{
		Bundle:     pos[0],
		TargetHome: *home,
		ConfigDir:  *cfg,
		DryRun:     *dry,
		Overwrite:  *overwrite,
		Deep:       *deep,
		AssumeYes:  *yes,
		Maps:       parsed,
	})
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
		return fmt.Errorf("usage: claude-port inspect <bundle>")
	}
	mb, err := bundle.ReadManifest(args[0])
	if err != nil {
		return err
	}
	if len(mb) == 0 {
		return fmt.Errorf("no manifest.json found — is %q a claude-port bundle?", args[0])
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
