package transfer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
)

// TestAppIDIsWireCompatibility pins the literal AppID. Two peers meet only if it
// matches exactly and there is no negotiation, so changing this value silently
// breaks send/receive against every released version. The sibling test in
// config_test.go compares the client field to the constant, which cannot catch a
// change to the constant itself.
func TestAppIDIsWireCompatibility(t *testing.T) {
	const want = "github.com/gowtham-sai-yadav/claude-teleport"
	if AppID != want {
		t.Fatalf("AppID = %q, want %q.\n"+
			"This is a wire-compatibility constant: changing it breaks transfers against every "+
			"installed version, including after a product rename.", AppID, want)
	}
}

// TestIsReachabilityError checks the retry trigger. Over-matching would retry a
// wrong code on a second server and fail twice; under-matching leaves the user
// stuck on a blocked network, which is the bug this exists to fix.
func TestIsReachabilityError(t *testing.T) {
	// The real failure observed on a conference network, verbatim.
	venueWifi := errors.New(`start transfer: dial ws://relay.magic-wormhole.io:4000/v1: ` +
		`failed to WebSocket dial: failed to send handshake request: ` +
		`Get "http://relay.magic-wormhole.io:4000/v1": dial tcp 69.164.206.178:4000: i/o timeout`)

	retry := []struct {
		name string
		err  error
	}{
		{"observed venue wifi failure", venueWifi},
		{"typed OpError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}},
		{"typed DNSError", &net.DNSError{Err: "no such host", Name: "relay.magic-wormhole.io"}},
		{"wrapped OpError", fmt.Errorf("connect: %w", &net.OpError{Op: "dial", Err: errors.New("boom")})},
		{"refused text", errors.New("dial tcp 1.2.3.4:4000: connect: connection refused")},
		{"unreachable text", errors.New("dial tcp: network is unreachable")},
		{"tls text", errors.New("tls handshake timeout")},
	}
	for _, c := range retry {
		if !isReachabilityError(c.err) {
			t.Errorf("%s: want retryable, got not retryable (%v)", c.name, c.err)
		}
	}

	keep := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"wrong code", errors.New("wrong password")},
		{"nameplate gone", errors.New("nameplate is crowded")},
		{"protocol", errors.New("expected a single bundle, got a text transfer")},
		{"user cancelled", context.Canceled},
		{"deadline", context.DeadlineExceeded},
		{"wrapped cancel", fmt.Errorf("send: %w", context.Canceled)},
	}
	for _, c := range keep {
		if isReachabilityError(c.err) {
			t.Errorf("%s: want NOT retryable, got retryable (%v)", c.name, c.err)
		}
	}
}

// TestCanFallback: an explicitly chosen server is never second-guessed. Someone
// running their own relay must see a real failure, not a silent redirect of
// their data onto a public server they did not pick.
func TestCanFallback(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"defaults", Config{}, true},
		{"explicit rendezvous", Config{RendezvousURL: "ws://mine.internal/v1"}, false},
		{"explicit relay only", Config{TransitRelay: "mine.internal:4001"}, true},
		{"opted out", Config{NoFallback: true}, false},
		{"opted out and explicit", Config{RendezvousURL: "ws://mine/v1", NoFallback: true}, false},
	}
	for _, c := range cases {
		if got := c.cfg.canFallback(); got != c.want {
			t.Errorf("%s: canFallback() = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestWithFallback: the alternates are TLS on 443 (the whole point), and a relay
// the user chose explicitly survives the switch.
func TestWithFallback(t *testing.T) {
	got := Config{CodeWords: 3}.withFallback()
	if got.RendezvousURL != FallbackRendezvousURL {
		t.Errorf("RendezvousURL = %q, want %q", got.RendezvousURL, FallbackRendezvousURL)
	}
	if got.TransitRelay != FallbackTransitRelay {
		t.Errorf("TransitRelay = %q, want %q", got.TransitRelay, FallbackTransitRelay)
	}
	if got.CodeWords != 3 {
		t.Errorf("CodeWords should be preserved, got %d", got.CodeWords)
	}
	if !strings.HasPrefix(FallbackRendezvousURL, "wss://") {
		t.Errorf("fallback mailbox must be TLS to pass restrictive firewalls, got %q", FallbackRendezvousURL)
	}

	// An explicit relay is a deliberate choice and must not be replaced.
	keep := Config{TransitRelay: "mine.internal:4001"}.withFallback()
	if keep.TransitRelay != "mine.internal:4001" {
		t.Errorf("explicit TransitRelay was overwritten with %q", keep.TransitRelay)
	}
}

// TestUnreachableMessage: the error a user actually sees should name both
// attempts and suggest something actionable, not just echo a port number.
func TestUnreachableMessage(t *testing.T) {
	err := unreachable("ws://primary:4000/v1", FallbackRendezvousURL, errors.New("i/o timeout"))
	msg := err.Error()
	for _, want := range []string{"ws://primary:4000/v1", FallbackRendezvousURL, "firewall", "--rendezvous"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	if !errors.Is(err, err) || !strings.Contains(msg, "i/o timeout") {
		t.Errorf("cause should be wrapped and visible:\n%s", msg)
	}

	// Single-attempt form (retry was impossible) should not claim two tries.
	single := unreachable("ws://primary:4000/v1", "", errors.New("nope")).Error()
	if strings.Contains(single, "then") {
		t.Errorf("single-attempt message should not describe a second try:\n%s", single)
	}
}

// TestPrimaryMailbox reports something usable in messages for both the default
// and an override.
func TestPrimaryMailbox(t *testing.T) {
	if got := (Config{RendezvousURL: "ws://mine/v1"}).primaryMailbox(); got != "ws://mine/v1" {
		t.Errorf("override: got %q", got)
	}
	if got := (Config{}).primaryMailbox(); got == "" {
		t.Error("default: primaryMailbox() should name the library default, got empty string")
	}
}
