// Package webui serves entangle's browser UI on localhost: the list of every
// coding session on this machine, and a point-and-click wizard for restoring a
// whole-machine export onto it.
//
// Built entirely on the standard library, with the page embedded, so it ships
// inside the single binary and works with no network at all. `entangle gui`
// starts it and opens the browser; the page calls the same agent and importer
// code the CLI and the cockpit use, so the three cannot disagree about what is
// on disk.
package webui

import (
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/gowtham-sai-yadav/entangle/internal/agent"
	"github.com/gowtham-sai-yadav/entangle/internal/agentshare"
	"github.com/gowtham-sai-yadav/entangle/internal/claudedir"
	"github.com/gowtham-sai-yadav/entangle/internal/importer"
	"github.com/gowtham-sai-yadav/entangle/internal/paths"
)

// errForeignBundle explains, rather than showing an empty wizard, why a bundle
// this page cannot handle produced no projects.
//
// The wizard's whole job is choosing which projects of a whole-machine Claude
// export to restore where. A single session from another agent has no projects to
// pick and attaches to one directory instead, so there is nothing here to click -
// point the user at the one command that does it.
func errForeignBundle(name string) error {
	return fmt.Errorf("this bundle holds one %s session, and this wizard restores whole Claude Code exports.\n"+
		"Import it from a terminal instead, standing in the project folder it should attach to:\n"+
		"    entangle import <bundle>", name)
}

// checkBundleKind reports a foreign single-session bundle before any planning
// work, so both API handlers fail the same way.
func checkBundleKind(bundlePath string) error {
	id, foreign, err := agentshare.PeekAgent(bundlePath)
	if err != nil {
		return err
	}
	if foreign {
		name := string(id)
		if p, ok := agent.Get(id); ok {
			name = p.DisplayName()
		}
		return errForeignBundle(name)
	}
	return nil
}

//go:embed index.html
var assets embed.FS

// Serve starts the GUI on 127.0.0.1:<port> (port 0 picks a free one), opens the
// browser, and blocks until interrupted.
//
// version is shown in the masthead. It is passed in rather than read here because
// it is stamped at build time and only the cli package knows it.
func Serve(port int, bundle, version string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/env", envHandler(bundle, version))
	mux.HandleFunc("/api/sessions", sessionsHandler)
	mux.HandleFunc("/api/plan", planHandler)
	mux.HandleFunc("/api/import", importHandler)

	fmt.Println("entangle GUI is running at", url)
	fmt.Println("Your browser should open automatically. Press Ctrl+C here to stop.")
	openBrowser(url)
	return http.Serve(ln, mux)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := assets.ReadFile("index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func envHandler(bundle, version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tp, err := claudedir.Locate("")
		home := ""
		if err == nil {
			home = tp.Home
		}
		writeJSON(w, map[string]any{
			"home":    home,
			"os":      runtime.GOOS,
			"bundle":  bundle,
			"version": version,
		})
	}
}

// sessionRow is the wire shape of one session, declared here rather than
// marshalling agent.Session directly.
//
// agent.Session carries a Ref, which is a provider-private handle to open files
// and internal state - documented as opaque and never serialised. Encoding the
// struct as-is would either fail on it or publish it to a browser. A hand-written
// row also keeps this page's contract independent of a struct that changes
// whenever a new tool is added.
type sessionRow struct {
	Tool     string `json:"tool"`
	ShortID  string `json:"shortId"`
	Title    string `json:"title"`
	Project  string `json:"project"`
	Messages int    `json:"messages"`
	Modified string `json:"modified"` // RFC3339; the browser knows the local zone
	Bytes    int64  `json:"bytes"`
}

// sessionsHandler lists what is on this machine, across every tool.
//
// The wizard used to be the whole page, which made the GUI a thing you opened
// once while moving laptops. The list is what makes it somewhere you can look at
// your work, and it is the same sweep the cockpit runs.
func sessionsHandler(w http.ResponseWriter, r *http.Request) {
	inv := agent.Scan("")
	rows := make([]sessionRow, 0, len(inv.Sessions))
	for _, s := range inv.Sessions {
		rows = append(rows, sessionRow{
			Tool:     string(s.Provider),
			ShortID:  s.ShortID,
			Title:    s.Title,
			Project:  s.ProjectPath,
			Messages: s.Messages,
			Modified: s.ModTime.Format(time.RFC3339),
			Bytes:    s.Size,
		})
	}
	tools := make([]string, 0, len(inv.Present))
	for _, id := range inv.Present {
		tools = append(tools, string(id))
	}
	// Problems ride along instead of becoming an error: one tool failing must not
	// blank a list that still has everything else in it.
	writeJSON(w, map[string]any{
		"sessions": rows,
		"tools":    tools,
		"problems": inv.Problems,
	})
}

type planReq struct {
	Bundle     string          `json:"bundle"`
	TargetHome string          `json:"targetHome"`
	TargetOS   string          `json:"targetOS"`
	Maps       []paths.Mapping `json:"maps"`
}

type importReq struct {
	planReq
	Projects  []string `json:"projects"`
	Deep      bool     `json:"deep"`
	Overwrite bool     `json:"overwrite"`
}

func planHandler(w http.ResponseWriter, r *http.Request) {
	var req planReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	if err := checkBundleKind(req.Bundle); err != nil {
		writeErr(w, err)
		return
	}
	man, err := importer.LoadManifest(req.Bundle)
	if err != nil {
		writeErr(w, err)
		return
	}
	tp, err := claudedir.Locate("")
	if err != nil {
		writeErr(w, err)
		return
	}
	plan := importer.BuildPlan(man, tp, importer.Options{
		Bundle:     req.Bundle,
		TargetHome: req.TargetHome,
		TargetOS:   req.TargetOS,
		Maps:       req.Maps,
	})
	writeJSON(w, plan)
}

func importHandler(w http.ResponseWriter, r *http.Request) {
	var req importReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, err)
		return
	}
	if err := checkBundleKind(req.Bundle); err != nil {
		writeErr(w, err)
		return
	}
	man, err := importer.LoadManifest(req.Bundle)
	if err != nil {
		writeErr(w, err)
		return
	}
	tp, err := claudedir.Locate("")
	if err != nil {
		writeErr(w, err)
		return
	}
	opts := importer.Options{
		Bundle:     req.Bundle,
		TargetHome: req.TargetHome,
		TargetOS:   req.TargetOS,
		Maps:       req.Maps,
		Projects:   req.Projects,
		Deep:       req.Deep,
		Overwrite:  req.Overwrite,
		AssumeYes:  true,
	}
	plan := importer.BuildPlan(man, tp, opts)
	res, err := importer.Import(tp, plan, opts)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, res)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	_ = exec.Command(cmd, args...).Start() // best effort; harmless if it fails
}
