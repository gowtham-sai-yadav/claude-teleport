// Package cli wires the export/import/inspect subcommands together.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gowtham-sai-yadav/claude-teleport/internal/bundle"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/exporter"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/importer"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/manifest"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/transfer"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/updater"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/webui"
)

// version is stamped by the linker at release time via
// -X ...internal/cli.version=<tag> (see .goreleaser.yaml). For `go install`
// builds it stays empty and we fall back to the module version in the build
// info, so the reported version tracks the git tag with no manual bumping.
var version string

// Version returns the running version, without a leading "v".
func Version() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

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
	case "sessions":
		return runSessions(args[1:])
	case "share":
		return runShare(args[1:])
	case "send":
		return runSend(args[1:])
	case "receive":
		return runReceive(args[1:])
	case "update", "upgrade":
		return runUpdate(args[1:])
	case "gui":
		return runGUI(args[1:])
	case "version", "-v", "--version":
		fmt.Println("claude-teleport", Version())
		return nil
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: export, import, inspect, verify, gui)", args[0])
	}
}

func printHelp() {
	fmt.Print("claude-teleport " + Version() + " - move your Claude Code history between machines\n\n" +
		"USAGE:\n" +
		"  claude-teleport export  [--out FILE] [--config-dir DIR]\n" +
		"  claude-teleport import  <bundle> [--dry-run] [--map OLD=NEW]... [--project P]... [--target-os OS] [--overwrite] [--deep] [--yes]\n" +
		"  claude-teleport inspect <bundle>\n" +
		"  claude-teleport verify  [--config-dir DIR]\n" +
		"  claude-teleport sessions [--project P] [--config-dir DIR]\n" +
		"  claude-teleport share   <session-id-prefix | --last> [--project P] [--out FILE] [--with-context] [--no-redact] [--yes]\n" +
		"  claude-teleport send    <session-id-prefix | --last> [--project P] [--with-context] [--no-redact] [--yes]\n" +
		"  claude-teleport receive <code> [--config-dir DIR] [--map OLD=NEW]... [--yes]\n" +
		"  claude-teleport update  [--check] [--yes]\n" +
		"  claude-teleport gui     [bundle] [--port N]\n\n" +
		"EXPORT runs on the OLD machine and writes a portable bundle.\n" +
		"IMPORT runs on the NEW machine and restores it, translating paths for this OS\n" +
		"(Linux, macOS, or Windows - drive letters and backslashes handled).\n" +
		"SESSIONS lists your conversations so you can find one to hand off. SHARE packs a\n" +
		"single session into a file for a teammate (secrets scrubbed first). SEND does the\n" +
		"same but streams it over an end-to-end-encrypted connection: you read out a short\n" +
		"code and they RECEIVE it, no file to move. GUI opens a point-and-click wizard.\n" +
		"VERIFY checks migrated sessions are resume-ready. Your login is never copied.\n")
}

func runExport(args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("out", "", "output bundle path")
	cfg := fs.String("config-dir", "", "override Claude config dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	res, err := exporter.Run(exporter.Options{Out: *out, ConfigDir: *cfg, Version: Version()})
	if err != nil {
		return err
	}
	fmt.Printf("Exported %d project(s), %d session(s) -> %s (%s)\n",
		res.Projects, res.Sessions, res.Path, exporter.HumanSize(res.Bytes))
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

func runSessions(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	cfg := fs.String("config-dir", "", "override the Claude config dir")
	project := fs.String("project", "", "only sessions whose project path or folder contains this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tp, err := claudedir.Locate(*cfg)
	if err != nil {
		return err
	}
	sessions, err := claudedir.ListSessions(tp)
	if err != nil {
		return err
	}
	sessions = filterSessions(sessions, *project)
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tLAST ACTIVE\tMSGS\tPROJECT\tTITLE")
	for _, s := range sessions {
		proj := s.ProjectPath
		if proj == "" {
			proj = "(unknown)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
			s.ShortID(), s.ModTime.Format("2006-01-02 15:04"), s.Messages, proj, s.Title)
	}
	tw.Flush()
	fmt.Printf("\n%d session(s). Share one with: claude-teleport share <ID>\n", len(sessions))
	return nil
}

func filterSessions(in []claudedir.Session, needle string) []claudedir.Session {
	if needle == "" {
		return in
	}
	needle = strings.ToLower(needle)
	var out []claudedir.Session
	for _, s := range in {
		if strings.Contains(strings.ToLower(s.ProjectPath), needle) ||
			strings.Contains(strings.ToLower(s.Folder), needle) {
			out = append(out, s)
		}
	}
	return out
}

func runShare(args []string) error {
	fs := flag.NewFlagSet("share", flag.ContinueOnError)
	out := fs.String("out", "", "output file path")
	cfg := fs.String("config-dir", "", "override the Claude config dir")
	last := fs.Bool("last", false, "share your most recent session")
	project := fs.String("project", "", "disambiguate by project when the same id exists in more than one")
	withContext := fs.Bool("with-context", false, "also include the project's memory/context files")
	noRedact := fs.Bool("no-redact", false, "do NOT scrub secrets before packing (not recommended)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	prefix := ""
	if len(pos) > 0 {
		prefix = pos[0]
	}
	if prefix == "" && !*last {
		return fmt.Errorf("usage: claude-teleport share <session-id-prefix | --last>")
	}
	return exporter.RunShare(exporter.ShareOptions{
		ConfigDir:     *cfg,
		Version:       Version(),
		Out:           *out,
		SessionPrefix: prefix,
		Project:       *project,
		Last:          *last,
		WithContext:   *withContext,
		Redact:        !*noRedact,
		AssumeYes:     *yes,
		Confirm:       confirmShare,
	})
}

// confirmShare is passed into the exporter so the summary and prompt live in
// one place (the CLI) while the packing logic stays in the exporter.
func confirmShare(preview exporter.SharePreview) bool {
	fmt.Println("About to share ONE session. This leaves your machine, so read it:")
	fmt.Printf("  session : %s  (%s)\n", preview.Title, preview.ShortID)
	fmt.Printf("  project : %s\n", preview.ProjectPath)
	fmt.Printf("  content : %d message(s), %s\n", preview.Messages, exporter.HumanSize(preview.Bytes))
	if preview.WithContext {
		fmt.Println("  context : project memory INCLUDED (--with-context)")
	} else {
		fmt.Println("  context : conversation only (memory not included)")
	}
	if preview.Redact {
		fmt.Printf("  secrets : %d likely secret(s) masked (best effort, not a guarantee)\n", preview.SecretsMasked)
	} else {
		fmt.Println("  secrets : NOT scrubbed (--no-redact) - the raw transcript will be shared")
	}
	return confirm("Write this file?")
}

// runSend builds a single-session bundle and streams it to a teammate over an
// end-to-end-encrypted wormhole, identified by a short spoken code. No file
// changes hands.
func runSend(args []string) error {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	cfg := fs.String("config-dir", "", "override the Claude config dir")
	last := fs.Bool("last", false, "send your most recent session")
	project := fs.String("project", "", "disambiguate by project when the same id exists in more than one")
	withContext := fs.Bool("with-context", false, "also include the project's memory/context files")
	noRedact := fs.Bool("no-redact", false, "do NOT scrub secrets before sending (not recommended)")
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	rendezvous := fs.String("rendezvous", envOr("CLAUDE_TELEPORT_RENDEZVOUS", ""), "rendezvous server URL (default: public magic-wormhole)")
	relay := fs.String("relay", envOr("CLAUDE_TELEPORT_RELAY", ""), "transit relay host:port (default: public magic-wormhole)")
	words := fs.Int("code-words", 2, "number of words in the transfer code")
	timeout := fs.Duration("timeout", 15*time.Minute, "give up if the peer does not connect within this time")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	prefix := ""
	if len(pos) > 0 {
		prefix = pos[0]
	}
	if prefix == "" && !*last {
		return fmt.Errorf("usage: claude-teleport send <session-id-prefix | --last>")
	}

	b, err := exporter.PrepareShare(exporter.ShareOptions{
		ConfigDir:     *cfg,
		Version:       Version(),
		SessionPrefix: prefix,
		Project:       *project,
		Last:          *last,
		WithContext:   *withContext,
		Redact:        !*noRedact,
	})
	if err != nil {
		return err
	}
	if !*yes && !confirmSend(b.Preview) {
		fmt.Println("Aborted - nothing was sent.")
		return nil
	}

	// Build the bundle in memory so we can stream it straight into the wormhole.
	var buf bytes.Buffer
	if err := b.WriteBundle(&buf); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	tcfg := transfer.Config{RendezvousURL: *rendezvous, TransitRelay: *relay, CodeWords: *words}
	fmt.Println("Preparing a secure transfer...")
	err = transfer.Send(ctx, tcfg, b.Name, bytes.NewReader(buf.Bytes()),
		func(code string) {
			fmt.Printf("\nGive your teammate this code:\n\n    %s\n\n", code)
			fmt.Println("They run this from inside their copy of the project:")
			fmt.Printf("    claude-teleport receive %s\n\n", code)
			fmt.Println("Waiting for them to connect... (press Ctrl-C to cancel)")
		},
		progressPrinter("Sending"),
	)
	if err != nil {
		return fmt.Errorf("send failed: %w", err)
	}
	fmt.Println("\nDone. The session is on your teammate's machine.")
	return nil
}

// runReceive pulls a session bundle over a wormhole using the code, then hands
// it to the normal importer (which attaches it to the current directory).
func runReceive(args []string) error {
	fs := flag.NewFlagSet("receive", flag.ContinueOnError)
	cfg := fs.String("config-dir", "", "override the target Claude config dir")
	overwrite := fs.Bool("overwrite", false, "overwrite existing files (backs up first)")
	deep := fs.Bool("deep", false, "rewrite old paths everywhere in transcripts, not just cwd")
	yes := fs.Bool("yes", false, "skip the import confirmation prompt")
	home := fs.String("target-home", "", "override the target home directory")
	tos := fs.String("target-os", "", "render paths for this OS: linux|darwin|windows")
	rendezvous := fs.String("rendezvous", envOr("CLAUDE_TELEPORT_RENDEZVOUS", ""), "rendezvous server URL (default: public magic-wormhole)")
	relay := fs.String("relay", envOr("CLAUDE_TELEPORT_RELAY", ""), "transit relay host:port (default: public magic-wormhole)")
	timeout := fs.Duration("timeout", 15*time.Minute, "give up if the transfer does not start within this time")
	var maps multiFlag
	fs.Var(&maps, "map", "remap OLD=NEW path prefix (repeatable)")
	pos, err := parseInterleaved(fs, args)
	if err != nil {
		return err
	}
	if len(pos) < 1 {
		return fmt.Errorf("usage: claude-teleport receive <code>")
	}
	code := pos[0]
	parsedMaps, err := parseMaps(maps)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	tcfg := transfer.Config{RendezvousURL: *rendezvous, TransitRelay: *relay}
	fmt.Println("Connecting...")
	in, err := transfer.Receive(ctx, tcfg, code)
	if err != nil {
		return fmt.Errorf("receive failed: %w", err)
	}

	// Stream to a temp bundle, capped so a peer that lies about the size cannot
	// fill the disk. The importer then treats it exactly like a shared file.
	tmp, err := os.CreateTemp("", "claude-teleport-recv-*.tgz")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	const maxBytes = 2 << 30 // 2 GiB hard ceiling
	if _, err := copyCapped(tmp, in, in.Bytes, maxBytes, progressPrinter("Receiving")); err != nil {
		tmp.Close()
		return fmt.Errorf("receive failed: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	fmt.Println()

	return importer.Run(importer.Options{
		Bundle:     tmpPath,
		TargetHome: *home,
		TargetOS:   *tos,
		ConfigDir:  *cfg,
		Overwrite:  *overwrite,
		Deep:       *deep,
		AssumeYes:  *yes,
		Maps:       parsedMaps,
	})
}

// confirmSend shows what is about to leave the machine over the network and
// asks to proceed. It mirrors confirmShare but names the transport.
func confirmSend(preview exporter.SharePreview) bool {
	fmt.Println("About to send ONE session over an end-to-end-encrypted connection. Read it first:")
	fmt.Printf("  session : %s  (%s)\n", preview.Title, preview.ShortID)
	fmt.Printf("  project : %s\n", preview.ProjectPath)
	fmt.Printf("  content : %d message(s), %s\n", preview.Messages, exporter.HumanSize(preview.Bytes))
	if preview.WithContext {
		fmt.Println("  context : project memory INCLUDED (--with-context)")
	} else {
		fmt.Println("  context : conversation only (memory not included)")
	}
	if preview.Redact {
		fmt.Printf("  secrets : %d likely secret(s) masked (best effort, not a guarantee)\n", preview.SecretsMasked)
	} else {
		fmt.Println("  secrets : NOT scrubbed (--no-redact) - the raw transcript will be sent")
	}
	return confirm("Send it?")
}

// progressPrinter returns a progress callback that rewrites a single status
// line on stderr.
func progressPrinter(label string) transfer.Progress {
	return func(done, total int64) {
		if total <= 0 {
			fmt.Fprintf(os.Stderr, "\r%s %s   ", label, exporter.HumanSize(done))
			return
		}
		pct := float64(done) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "\r%s %3.0f%% (%s / %s)   ", label, pct, exporter.HumanSize(done), exporter.HumanSize(total))
	}
}

// copyCapped copies src to dst, reporting progress and refusing to write more
// than limit bytes (a peer controls the offered size, so it cannot be trusted).
func copyCapped(dst io.Writer, src io.Reader, total, limit int64, prog transfer.Progress) (int64, error) {
	buf := make([]byte, 32*1024)
	var done int64
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			if done+int64(n) > limit {
				return done, fmt.Errorf("incoming bundle exceeds the %s safety cap", exporter.HumanSize(limit))
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return done, werr
			}
			done += int64(n)
			if prog != nil {
				prog(done, total)
			}
		}
		if rerr == io.EOF {
			return done, nil
		}
		if rerr != nil {
			return done, rerr
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// runUpdate checks GitHub for a newer release and, unless --check, downloads it,
// verifies its checksum, and replaces this binary in place.
func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "only report whether a newer version exists")
	yes := fs.Bool("yes", false, "update without asking")
	repo := fs.String("repo", updater.DefaultRepo, "owner/repo to update from")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("Checking for updates...")
	latest, err := updater.LatestVersion(ctx, *repo)
	if err != nil {
		return fmt.Errorf("could not check the latest version: %w", err)
	}
	latestClean := strings.TrimPrefix(latest, "v")
	fmt.Printf("  installed : %s\n  latest    : %s\n", Version(), latestClean)

	if !updater.Newer(latest, Version()) {
		fmt.Println("You are already on the latest version.")
		return nil
	}
	if *check {
		fmt.Printf("A newer version is available (%s). Run `claude-teleport update` to install it.\n", latestClean)
		return nil
	}
	if !*yes && !confirm(fmt.Sprintf("Update to %s now?", latestClean)) {
		fmt.Println("Aborted.")
		return nil
	}
	if err := updater.Apply(ctx, *repo, latest, progressPrinter("Downloading")); err != nil {
		return fmt.Errorf("update failed: %w", err)
	}
	fmt.Printf("\nUpdated to %s.\n", latestClean)
	return nil
}

// confirm asks a yes/no question on the terminal (default no).
func confirm(q string) bool {
	fmt.Printf("%s [y/N]: ", q)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes"
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
	if man.IsSession() {
		fmt.Printf("kind        : single session (%s)\n", man.SessionID)
		fmt.Printf("redacted    : %v\n", man.Redacted)
	} else {
		fmt.Printf("kind        : full backup\n")
	}
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
