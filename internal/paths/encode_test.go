package paths

import "testing"

func TestEncode(t *testing.T) {
	cases := map[string]string{
		"/home/kali/Desktop":                            "-home-kali-Desktop",
		"/Users/gowthamsaig/Desktop/Bleed-red":          "-Users-gowthamsaig-Desktop-Bleed-red",
		"/home/kali/Desktop/Bug_Bounty":                 "-home-kali-Desktop-Bug-Bounty",
		"/home/kali/Desktop/crawler_scraper":            "-home-kali-Desktop-crawler-scraper",
		"/home/kali/Desktop/Appsec/secure-code-review":  "-home-kali-Desktop-Appsec-secure-code-review",
		`C:\Users\bob\proj`:                             "-C-Users-bob-proj",
		`D:\dev\my.app`:                                 "-D-dev-my-app",
		"/home/kali/my project":                         "-home-kali-my-project",
	}
	for in, want := range cases {
		if got := Encode(in); got != want {
			t.Errorf("Encode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemapUnix(t *testing.T) {
	m := []Mapping{{Old: "/home/kali", New: "/Users/gowtham"}}
	if got := Remap("/home/kali/Desktop/sensei", m, "linux"); got != "/Users/gowtham/Desktop/sensei" {
		t.Fatalf("subdir: got %q", got)
	}
	if got := Remap("/home/kali", m, "linux"); got != "/Users/gowtham" {
		t.Fatalf("exact home: got %q", got)
	}
	if got := Remap("/opt/x", m, "linux"); got != "/opt/x" {
		t.Fatalf("no match should be unchanged: got %q", got)
	}
}

func TestRemapToWindows(t *testing.T) {
	m := []Mapping{{Old: "/home/kali", New: `C:\Users\bob`}}
	if got := Remap("/home/kali/Desktop/app", m, "windows"); got != `C:\Users\bob\Desktop\app` {
		t.Fatalf("got %q", got)
	}
}

func TestRemapFromWindows(t *testing.T) {
	m := []Mapping{{Old: `C:\Users\bob`, New: "/home/kali"}}
	if got := Remap(`C:\Users\bob\Desktop\app`, m, "linux"); got != "/home/kali/Desktop/app" {
		t.Fatalf("got %q", got)
	}
}

func TestRemapLongestWins(t *testing.T) {
	m := []Mapping{
		{Old: "/home/kali", New: "/a"},
		{Old: "/home/kali/Desktop", New: "/b"},
	}
	if got := Remap("/home/kali/Desktop/x", m, "linux"); got != "/b/x" {
		t.Fatalf("got %q", got)
	}
}

func TestHomeRoot(t *testing.T) {
	cases := map[string]string{
		"/home/kali/Desktop/x":       "/home/kali",
		"/Users/gowthamsaig/Desktop": "/Users/gowthamsaig",
		"/root/proj":                 "/root",
		`C:\Users\bob\proj`:          "C:/Users/bob",
		"/opt/weird":                 "",
	}
	for in, want := range cases {
		if got := HomeRoot(in); got != want {
			t.Errorf("HomeRoot(%q) = %q, want %q", in, got, want)
		}
	}
}
