package updater

import "testing"

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.4.0", "0.3.0", true},
		{"v0.3.1", "0.3.0", true},
		{"0.3.0", "v0.3.0", false},
		{"v0.3.0", "0.3.1", false},
		{"v0.10.0", "0.9.0", true}, // numeric, not lexicographic
		{"v1.0.0", "0.99.99", true},
		{"v0.3.0", "0.3.0-rc1", false}, // pre-release/build suffixes are ignored (documented)
		{"v0.4.0", "0.3.0-rc1", true},  // but a higher core still wins
		{"v0.3.0", "dev", true},        // unparseable current treated as 0.0.0
	}
	for _, c := range cases {
		if got := Newer(c.latest, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.latest, c.current, got, c.want)
		}
	}
}

func TestAssetFor(t *testing.T) {
	cases := []struct {
		goos, goarch, want string
		wantErr            bool
	}{
		{"darwin", "arm64", "claude-teleport-darwin-arm64", false},
		{"darwin", "amd64", "claude-teleport-darwin-amd64", false},
		{"linux", "amd64", "claude-teleport-linux-amd64", false},
		{"linux", "arm64", "claude-teleport-linux-arm64", false},
		{"windows", "amd64", "claude-teleport-windows-amd64.exe", false},
		{"windows", "arm64", "claude-teleport-windows-amd64.exe", false}, // falls back to amd64
		{"plan9", "amd64", "", true},
		{"linux", "386", "", true},
	}
	for _, c := range cases {
		got, err := AssetFor(c.goos, c.goarch)
		if c.wantErr {
			if err == nil {
				t.Errorf("AssetFor(%q,%q) expected error", c.goos, c.goarch)
			}
			continue
		}
		if err != nil {
			t.Errorf("AssetFor(%q,%q) unexpected error: %v", c.goos, c.goarch, err)
		}
		if got != c.want {
			t.Errorf("AssetFor(%q,%q) = %q, want %q", c.goos, c.goarch, got, c.want)
		}
	}
}
