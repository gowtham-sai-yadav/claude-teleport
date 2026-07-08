package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScrubMasksSecrets(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"anthropic", "my key is sk-ant-api03-abcdefghij1234567890ABCDEFXYZ done"},
		{"github", "token gh" + "p_ABCDEFGHIJ1234567890abcdefXYZ0123456 here"},
		{"aws", "id AKIAIOSFODNN7EXAMPLE end"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
		{"bearer", "Authorization: Bearer abcdef1234567890XYZ"},
		{"password", `password="hunter2secret"`},
		{"apikey", `api_key: 'abcdef123456'`},
		{"privatekey", "-----BEGIN RSA PRIVATE KEY-----\nMIIabc123\n-----END RSA PRIVATE KEY-----"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, n := Scrub([]byte(c.in))
			if n == 0 {
				t.Fatalf("expected a secret to be masked, got 0")
			}
			if !strings.Contains(string(out), mask) {
				t.Fatalf("expected mask in output, got %q", out)
			}
		})
	}
}

func TestScrubKeepsKeyName(t *testing.T) {
	out, n := Scrub([]byte(`password="hunter2secret"`))
	if n != 1 {
		t.Fatalf("want 1 hit, got %d", n)
	}
	if !strings.Contains(string(out), "password") {
		t.Fatalf("key name should survive, got %q", out)
	}
	if strings.Contains(string(out), "hunter2secret") {
		t.Fatalf("secret value should be gone, got %q", out)
	}
}

func TestScrubLeavesOrdinaryTextAlone(t *testing.T) {
	in := `{"type":"user","message":{"role":"user","content":"please refactor the parser"}}`
	out, n := Scrub([]byte(in))
	if n != 0 {
		t.Fatalf("ordinary text should not be flagged, got %d hits", n)
	}
	if string(out) != in {
		t.Fatalf("ordinary text should be unchanged")
	}
}

func TestScrubMasksSecretInJSONEscapedQuotes(t *testing.T) {
	// A secret a user pasted into chat is stored with escaped quotes.
	in := `{"content":"run with password=\"hunter2secret\" now"}`
	out, n := Scrub([]byte(in))
	if n == 0 {
		t.Fatalf("escaped-quote secret should be masked, got %q", out)
	}
	if strings.Contains(string(out), "hunter2secret") {
		t.Fatalf("secret value should be gone, got %q", out)
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("scrubbed line is no longer valid JSON: %v\n%s", err, out)
	}
}

func TestScrubbedLineStaysValidJSON(t *testing.T) {
	in := `{"type":"user","message":{"role":"user","content":"here is my key sk-ant-api03-abcdefghij1234567890ABCDEFXYZ"}}`
	out, n := Scrub([]byte(in))
	if n == 0 {
		t.Fatal("expected a secret to be masked")
	}
	var v map[string]any
	if err := json.Unmarshal(out, &v); err != nil {
		t.Fatalf("scrubbed line is no longer valid JSON: %v\n%s", err, out)
	}
}

func TestCountMatchesScrub(t *testing.T) {
	in := []byte("sk-ant-api03-abcdefghij1234567890ABCDEFXYZ and AKIAIOSFODNN7EXAMPLE")
	c := Count(in)
	_, n := Scrub(in)
	if c != n {
		t.Fatalf("Count=%d but Scrub reported %d", c, n)
	}
	if c != 2 {
		t.Fatalf("want 2 secrets, got %d", c)
	}
}
