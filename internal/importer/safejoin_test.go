package importer

import (
	"path/filepath"
	"testing"
)

func TestSafeJoinContainment(t *testing.T) {
	base := filepath.Join(string(filepath.Separator)+"tmp", "target")

	if _, ok := safeJoin(base, "a", "b.jsonl"); !ok {
		t.Error("legit nested path was rejected")
	}
	if _, ok := safeJoin(base, "-Users-x-Desktop", "memory", "MEMORY.md"); !ok {
		t.Error("legit project path was rejected")
	}
	// Malicious entries that try to climb out must be refused.
	for _, bad := range []string{
		"../escape.txt",
		"../../escape.txt",
		"x/../../../../escape.txt",
		"../../../../../../etc/passwd",
	} {
		if _, ok := safeJoin(base, filepath.FromSlash(bad)); ok {
			t.Errorf("traversal not blocked: %q", bad)
		}
	}
}
