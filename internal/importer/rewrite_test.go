package importer

import (
	"testing"

	"claude-port/internal/manifest"
	"claude-port/internal/paths"
)

func TestRewriteUnixToUnix(t *testing.T) {
	rw := newPathRewriter("/home/kali/Desktop", "/Users/gowtham/Desktop", false)
	in := []byte(`{"type":"mode","cwd":"/home/kali/Desktop","x":"/home/kali/Desktop/a.txt"}`)
	got := string(rw.line(in))
	// cwd rewritten; the path inside another field is left alone (safe default)
	want := `{"type":"mode","cwd":"/Users/gowtham/Desktop","x":"/home/kali/Desktop/a.txt"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestRewriteSpacedCwd(t *testing.T) {
	rw := newPathRewriter("/home/kali/Desktop", "/Users/gowtham/Desktop", false)
	in := []byte(`{"cwd": "/home/kali/Desktop"}`)
	if got := string(rw.line(in)); got != `{"cwd": "/Users/gowtham/Desktop"}` {
		t.Fatalf("got %s", got)
	}
}

func TestRewriteUnixToWindows(t *testing.T) {
	rw := newPathRewriter("/home/kali/Desktop", `C:\Users\bob\Desktop`, false)
	in := []byte(`{"cwd":"/home/kali/Desktop"}`)
	// the new Windows path must land back on disk JSON-escaped
	if got := string(rw.line(in)); got != `{"cwd":"C:\\Users\\bob\\Desktop"}` {
		t.Fatalf("got %s", got)
	}
}

func TestRewriteWindowsToUnix(t *testing.T) {
	// a transcript written on Windows stores escaped backslashes
	rw := newPathRewriter(`C:\Users\bob\Desktop`, "/home/kali/Desktop", false)
	in := []byte(`{"cwd":"C:\\Users\\bob\\Desktop"}`)
	if got := string(rw.line(in)); got != `{"cwd":"/home/kali/Desktop"}` {
		t.Fatalf("got %s", got)
	}
}

func TestRewriteDeep(t *testing.T) {
	rw := newPathRewriter(`C:\Users\bob`, "/home/kali", true)
	in := []byte(`{"cwd":"C:\\Users\\bob","msg":"see C:\\Users\\bob\\f.txt"}`)
	got := string(rw.line(in))
	want := `{"cwd":"/home/kali","msg":"see /home/kali\\f.txt"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestBuildMappingsCrossOSDedupe(t *testing.T) {
	man := manifest.Manifest{
		Source: manifest.Source{OS: "windows", Home: `C:\Users\bob`},
		Projects: []manifest.Project{
			{OriginalPath: `C:\Users\bob\Desktop\proj`},
			{OriginalPath: `C:\Users\bob\code`},
		},
	}
	m := buildMappings(man, "/home/kali", nil)
	if len(m) != 1 {
		t.Fatalf("want 1 deduped mapping, got %d: %+v", len(m), m)
	}
	if m[0].Old != "C:/Users/bob" || m[0].New != "/home/kali" {
		t.Fatalf("got %+v", m[0])
	}
}

func TestBuildMappingsExplicitWins(t *testing.T) {
	man := manifest.Manifest{
		Source:   manifest.Source{OS: "linux", Home: "/home/kali"},
		Projects: []manifest.Project{{OriginalPath: "/home/kali/Desktop"}},
	}
	explicit := []paths.Mapping{{Old: "/home/kali", New: "/custom/place"}}
	m := buildMappings(man, "/home/kali-default", explicit)
	if len(m) != 1 || m[0].New != "/custom/place" {
		t.Fatalf("explicit rule should win, got %+v", m)
	}
}
