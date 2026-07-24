package claudedir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsEphemeralPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// polymath-style temp litter
		{"/private/var/folders/hd/qw2/T/ps-coding-Ht6yWc", true},
		{"/var/folders/hd/qw2/T/nut-6XNT0U", true},
		{"/tmp/scratch", true},
		{"/private/tmp/foo", true},
		{"/var/tmp/bar", true},
		{`C:\Users\bob\AppData\Local\Temp\claude-xyz`, true},
		// real projects must be kept
		{"/Users/gowtham/Desktop/claude-teleport", false},
		{"/Users/gowtham/Desktop", false},
		{"/Users/gowtham/tmp-notes/project", false}, // "tmp" only as a name part, not the root
		{"/home/dev/work/api", false},
		{`C:\Users\bob\projects\api`, false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsEphemeralPath(c.path); got != c.want {
			t.Errorf("IsEphemeralPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// TestDiscoverHidesTempProjects builds a fake projects dir and checks the temp
// folders are hidden by default and revealed by CLAUDE_TELEPORT_INCLUDE_TEMP.
func TestDiscoverHidesTempProjects(t *testing.T) {
	root := t.TempDir()
	projects := filepath.Join(root, "projects")

	mk := func(folder, cwd string) {
		dir := filepath.Join(projects, folder)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := "{}\n"
		if cwd != "" {
			line = `{"cwd":"` + cwd + `"}` + "\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mk("-Users-gowtham-Desktop-real", "/Users/gowtham/Desktop/real")                               // real
	mk("-private-var-folders-hd-T-ps-coding-Ht6yWc", "/private/var/folders/hd/T/ps-coding-Ht6yWc") // temp (path recovered)
	mk("-private-var-folders-hd-T-ps-coding-NoCwd", "")                                            // temp (no cwd -> encoded-name fallback)

	p := Paths{ProjectsDir: projects, JSONPath: filepath.Join(root, ".claude.json")}

	got, err := Discover(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Folder != "-Users-gowtham-Desktop-real" {
		var names []string
		for _, pr := range got {
			names = append(names, pr.Folder)
		}
		t.Fatalf("default Discover = %v, want only the real project", names)
	}

	t.Setenv("CLAUDE_TELEPORT_INCLUDE_TEMP", "1")
	all, err := Discover(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("with CLAUDE_TELEPORT_INCLUDE_TEMP, Discover returned %d projects, want 3", len(all))
	}
}
