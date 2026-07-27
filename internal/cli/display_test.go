package cli

import (
	"strings"
	"testing"
)

// TestElideLeft: a session list is unreadable if one deeply nested path pads the
// whole table past the terminal width (a real 218-character path from an editor's
// cache directory did exactly that). The tail is kept because the end of a path is
// what identifies the project.
func TestElideLeft(t *testing.T) {
	const max = 46

	short := "/Users/dev/api"
	if got := elideLeft(short, max); got != short {
		t.Errorf("a short path must be untouched, got %q", got)
	}

	exact := strings.Repeat("a", max)
	if got := elideLeft(exact, max); got != exact {
		t.Errorf("a path at the limit must be untouched, got %q", got)
	}

	long := "/Users/gowtham/Library/Application Support/Omi/Artifacts/com.omi.omi-dev-local/att_b6e2d2c09a184470ac2a4ac9d396ade9"
	got := elideLeft(long, max)
	if n := len([]rune(got)); n != max {
		t.Errorf("elided length = %d, want %d (%q)", n, max, got)
	}
	if !strings.HasPrefix(got, "…") {
		t.Errorf("elision should be marked at the front, got %q", got)
	}
	if !strings.HasSuffix(long, strings.TrimPrefix(got, "…")) {
		t.Errorf("the tail must be preserved verbatim, got %q", got)
	}

	// Multi-byte paths must be cut on rune boundaries, not bytes.
	wide := "/Users/dev/" + strings.Repeat("プロジェクト", 12)
	w := elideLeft(wide, max)
	if n := len([]rune(w)); n != max {
		t.Errorf("multi-byte elided length = %d runes, want %d", n, max)
	}
	if !strings.ContainsRune(w, '…') {
		t.Errorf("multi-byte elision lost its marker: %q", w)
	}
}
