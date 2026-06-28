// Package paths handles the two path problems at the heart of migration:
// turning an absolute path into the folder name Claude Code uses, and
// rewriting an old machine's paths to the new machine.
package paths

import "strings"

// NormSep normalises Windows backslashes to forward slashes so the rest of
// the code only has to reason about one separator.
func NormSep(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}

// Encode converts an absolute path into the folder name Claude Code stores
// sessions under (e.g. "/home/kali/Desktop" -> "-home-kali-Desktop").
//
// Every separator, dot, underscore and space collapses to a single dash, and
// a Windows drive colon is dropped ("C:\Users\bob" -> "-C-Users-bob").
//
// NOTE: this transform is lossy — '/', '\\', '.', '_' and ' ' all map to '-',
// so the original path can never be recovered from the folder name. That is
// exactly why the manifest stores the true path alongside it.
func Encode(absPath string) string {
	s := strings.ReplaceAll(absPath, "\\", "/")
	s = strings.ReplaceAll(s, ":", "")
	r := strings.NewReplacer(
		"/", "-",
		".", "-",
		"_", "-",
		" ", "-",
	)
	s = r.Replace(s)
	// A Windows path ("C:/Users/...") loses its leading separator above, so
	// make sure the leading dash Claude always uses is present.
	if !strings.HasPrefix(s, "-") {
		s = "-" + s
	}
	return s
}

// Mapping rewrites paths that start with Old so they start with New instead.
type Mapping struct {
	Old string `json:"old"`
	New string `json:"new"`
}

func hasPathPrefix(path, prefix string) bool {
	p := strings.TrimRight(NormSep(path), "/")
	pre := strings.TrimRight(NormSep(prefix), "/")
	if pre == "" {
		return false
	}
	if p == pre {
		return true
	}
	return strings.HasPrefix(p, pre+"/")
}

// Remap rewrites absPath using the first matching mapping, preferring the
// longest (most specific) Old prefix. The result is rendered in targetOS
// style: backslashes for "windows", forward slashes otherwise.
func Remap(absPath string, mappings []Mapping, targetOS string) string {
	src := NormSep(absPath)
	best := -1
	bestLen := -1
	for i := range mappings {
		if hasPathPrefix(src, mappings[i].Old) {
			l := len(strings.TrimRight(NormSep(mappings[i].Old), "/"))
			if l > bestLen {
				bestLen = l
				best = i
			}
		}
	}
	if best == -1 {
		return absPath
	}
	oldN := strings.TrimRight(NormSep(mappings[best].Old), "/")
	rest := strings.TrimPrefix(src, oldN) // keeps the leading "/" of the remainder (or "")
	out := strings.TrimRight(NormSep(mappings[best].New), "/") + rest
	if targetOS == "windows" {
		out = strings.ReplaceAll(out, "/", "\\")
	}
	return out
}

// HomeRoot guesses the home-directory root of an absolute path, e.g.
// "/home/kali/Desktop/x" -> "/home/kali". It understands Unix /home and
// /Users layouts, /root, and Windows "C:/Users/<user>". Returns "" when it
// cannot confidently identify a home root.
func HomeRoot(p string) string {
	s := strings.TrimRight(NormSep(p), "/")
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "/") {
		parts := strings.Split(strings.TrimPrefix(s, "/"), "/")
		if len(parts) >= 1 && parts[0] == "root" {
			return "/root"
		}
		if len(parts) >= 2 && (parts[0] == "home" || parts[0] == "Users") {
			return "/" + parts[0] + "/" + parts[1]
		}
		return ""
	}
	// Windows-style: C:/Users/<user>
	parts := strings.Split(s, "/")
	if len(parts) >= 3 && strings.HasSuffix(parts[0], ":") {
		return parts[0] + "/" + parts[1] + "/" + parts[2]
	}
	return ""
}
