// Package redact does a best-effort scrub of obvious secrets before a session
// leaves the machine. It is deliberately conservative: it masks things that
// look like credentials and leaves everything else untouched. It is NOT a
// guarantee that a transcript is secret-free - a human should still glance at
// what they are sharing.
package redact

import "regexp"

const mask = "[REDACTED]"

// rule is one pattern to look for. When capturePrefix is true the regex has a
// leading capturing group (a key like `password=`) that is kept, and only the
// value after it is masked.
type rule struct {
	name          string
	re            *regexp.Regexp
	capturePrefix bool
}

var rules = []rule{
	// PEM private key blocks (RSA, EC, OPENSSH, PGP, ...).
	{name: "private-key", re: regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)},
	// Provider tokens with distinctive prefixes.
	{name: "anthropic-key", re: regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`)},
	{name: "openai-key", re: regexp.MustCompile(`sk-(?:proj-)?[A-Za-z0-9_\-]{20,}`)},
	{name: "github-token", re: regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`)},
	{name: "github-pat", re: regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`)},
	{name: "slack-token", re: regexp.MustCompile(`xox[baprs]-[A-Za-z0-9\-]{10,}`)},
	{name: "google-key", re: regexp.MustCompile(`AIza[A-Za-z0-9_\-]{35}`)},
	{name: "aws-access-key", re: regexp.MustCompile(`A(?:KIA|SIA|ROA|IDA)[0-9A-Z]{16}`)},
	{name: "hf-token", re: regexp.MustCompile(`hf_[A-Za-z0-9]{20,}`)},
	{name: "gitlab-token", re: regexp.MustCompile(`glpat-[A-Za-z0-9_\-]{20,}`)},
	{name: "stripe-key", re: regexp.MustCompile(`[rs]k_(?:live|test)_[A-Za-z0-9]{20,}`)},
	// JSON Web Tokens.
	{name: "jwt", re: regexp.MustCompile(`eyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+`)},
	// Authorization: Bearer <token>.
	{name: "bearer", re: regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9\-._~+/]{12,}=*`), capturePrefix: true},
	// key/value assignments: password=..., api_key: "...", secret => '...'.
	// The optional \\? before the quote steps over JSON-escaped quotes, because
	// inside a transcript a pasted `password="x"` is stored as `password=\"x\"`.
	{name: "keyed-secret", re: regexp.MustCompile(`(?i)((?:password|passwd|pwd|secret|api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret)\\?['"]?\s*[:=]+\s*\\?['"]?)[^\s'"\\]{6,}`), capturePrefix: true},
}

// Scrub returns b with likely secrets masked and a count of how many were
// masked. The input is not mutated.
func Scrub(b []byte) ([]byte, int) {
	total := 0
	for _, ru := range rules {
		matches := ru.re.FindAll(b, -1)
		if len(matches) == 0 {
			continue
		}
		total += len(matches)
		if ru.capturePrefix {
			b = ru.re.ReplaceAll(b, []byte("${1}"+mask))
		} else {
			b = ru.re.ReplaceAll(b, []byte(mask))
		}
	}
	return b, total
}

// Count reports how many likely secrets are in b without modifying it. It runs
// the same sequential masking as Scrub so overlapping rules (e.g. a key that
// matches two patterns) are counted exactly once, matching what Scrub reports.
func Count(b []byte) int {
	_, n := Scrub(b)
	return n
}
