// Package webui serves a small point-and-click import wizard on localhost.
// It is built entirely on the standard library (no external dependencies), so
// it ships inside the single binary. `claude-teleport gui` starts it and opens
// the browser; the page talks to the same importer code the CLI uses.
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

	"github.com/gowtham-sai-yadav/claude-teleport/internal/claudedir"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/importer"
	"github.com/gowtham-sai-yadav/claude-teleport/internal/paths"
)

//go:embed index.html
var assets embed.FS

// Serve starts the wizard on 127.0.0.1:<port> (port 0 picks a free one),
// opens the browser, and blocks until interrupted.
func Serve(port int, bundle string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveIndex)
	mux.HandleFunc("/api/env", envHandler(bundle))
	mux.HandleFunc("/api/plan", planHandler)
	mux.HandleFunc("/api/import", importHandler)

	fmt.Println("claude-teleport GUI is running at", url)
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

func envHandler(bundle string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tp, err := claudedir.Locate("")
		home := ""
		if err == nil {
			home = tp.Home
		}
		writeJSON(w, map[string]any{
			"home":   home,
			"os":     runtime.GOOS,
			"bundle": bundle,
		})
	}
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
