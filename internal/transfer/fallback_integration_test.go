//go:build integration

// These tests cover the fallback-and-race path, which the original round-trip test
// does not reach: it names a rendezvous server explicitly, and naming one disables
// fallback by design. That gap is exactly why two real bugs shipped, so both are
// pinned here.
//
//	go test -tags integration -timeout 120s ./internal/transfer/
package transfer

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/psanford/wormhole-william/rendezvous/rendezvousservertest"
)

const payload = "a shared session's bytes"

// sendInBackground starts a send and returns the code it produced.
func sendInBackground(t *testing.T, ctx context.Context, cfg Config) (code string, wait func() error) {
	t.Helper()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Send(ctx, cfg, "bundle.tgz", bytes.NewReader([]byte(payload)),
			func(c string) { codeCh <- c }, nil)
	}()
	select {
	case c := <-codeCh:
		return c, func() error { return <-errCh }
	case err := <-errCh:
		t.Fatalf("send ended before producing a code: %v", err)
	case <-time.After(45 * time.Second):
		t.Fatal("no code within 45s")
	}
	return "", nil
}

// TestRaceReceiveKeepsWinnerAlive is the regression test for a bug that made every
// receive fail with "context canceled".
//
// Receive races two mailboxes and cancels the loser. The first version cancelled
// both on return, which killed the very stream it had just handed back - and the
// caller had not read a byte yet. Reading the payload here is the whole point of
// the test: a version that cancels the winner cannot pass it.
func TestRaceReceiveKeepsWinnerAlive(t *testing.T) {
	requireLocalWormhole(t)

	primary := rendezvousservertest.NewServer()
	defer primary.Close()
	fallback := rendezvousservertest.NewServer()
	defer fallback.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// The sender is on the primary; the receiver must race and pick it.
	cfg := Config{
		testPrimary:  primary.WebSocketURL(),
		testFallback: fallback.WebSocketURL(),
		TransitRelay: "127.0.0.1:1", // dead on purpose, forcing the direct local path
	}
	code, wait := sendInBackground(t, ctx, cfg)

	in, err := Receive(ctx, cfg, code)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	got, err := io.ReadAll(in)
	if err != nil {
		t.Fatalf("reading the received stream failed - the winning attempt was cancelled: %v", err)
	}
	if string(got) != payload {
		t.Errorf("payload = %q, want %q", got, payload)
	}
	if err := wait(); err != nil {
		t.Errorf("send: %v", err)
	}
}

// TestSendStallsOverToFallback is the regression test for the bug the user hit: a
// mailbox that accepts the connection and then never answers.
//
// Retrying "on error" never fired, because nothing errored - the send simply hung
// and the user watched a spinner forever. A stalled server is simulated by a
// listener that accepts and holds, which is what the public default was doing.
func TestSendStallsOverToFallback(t *testing.T) {
	requireLocalWormhole(t)

	stalled := newStalledListener(t)
	defer stalled.Close()

	fallback := rendezvousservertest.NewServer()
	defer fallback.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var (
		mu         sync.Mutex
		fellBackTo string
	)
	cfg := Config{
		testPrimary:  stalled.URL(),
		testFallback: fallback.WebSocketURL(),
		TransitRelay: "127.0.0.1:1",
		OnFallback: func(mailbox, _ string, _ error) {
			mu.Lock()
			fellBackTo = mailbox
			mu.Unlock()
		},
	}

	start := time.Now()
	code, wait := sendInBackground(t, ctx, cfg)
	elapsed := time.Since(start)

	mu.Lock()
	got := fellBackTo
	mu.Unlock()
	if got != fallback.WebSocketURL() {
		t.Errorf("expected a fallback to %q, got %q", fallback.WebSocketURL(), got)
	}
	// It must give up on the stalled server rather than wait for the caller's
	// deadline, and it must not give up so eagerly that a slow server is abandoned.
	if elapsed < handshakeTimeout {
		t.Errorf("fell back after %v, sooner than the %v handshake window", elapsed, handshakeTimeout)
	}
	if elapsed > handshakeTimeout+30*time.Second {
		t.Errorf("took %v to fall back; the stall window is not bounding it", elapsed)
	}

	// And the transfer still completes, on the server that answered.
	in, err := Receive(ctx, cfg, code)
	if err != nil {
		t.Fatalf("Receive after fallback: %v", err)
	}
	body, err := io.ReadAll(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != payload {
		t.Errorf("payload = %q, want %q", body, payload)
	}
	if err := wait(); err != nil {
		t.Errorf("send: %v", err)
	}
}

// TestExplicitServerIsNotSecondGuessed: someone running their own relay must see a
// real failure, not have their data quietly redirected to a public server.
func TestExplicitServerIsNotSecondGuessed(t *testing.T) {
	stalled := newStalledListener(t)
	defer stalled.Close()
	fallback := rendezvousservertest.NewServer()
	defer fallback.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	fellBack := false
	cfg := Config{
		RendezvousURL: stalled.URL(), // named explicitly
		testFallback:  fallback.WebSocketURL(),
		TransitRelay:  "127.0.0.1:1",
		OnFallback:    func(string, string, error) { fellBack = true },
	}

	done := make(chan error, 1)
	go func() {
		done <- Send(ctx, cfg, "b.tgz", bytes.NewReader([]byte(payload)), func(string) {}, nil)
	}()
	select {
	case <-done:
	case <-time.After(19 * time.Second):
		// Hanging on the user's own server is the correct behaviour here; what must
		// not happen is a silent switch.
	}
	if fellBack {
		t.Error("an explicitly named server must never be swapped for a public one")
	}
}

// stalledListener accepts TCP connections and then does nothing with them, which
// is how the public default was behaving: the port is open, an ordinary HTTP
// request even succeeds, but the wormhole handshake never completes.
type stalledListener struct {
	ln   net.Listener
	done chan struct{}
	held []net.Conn
	mu   sync.Mutex
}

func newStalledListener(t *testing.T) *stalledListener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &stalledListener{ln: ln, done: make(chan struct{})}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold the connection open and never reply.
			s.mu.Lock()
			s.held = append(s.held, c)
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *stalledListener) URL() string { return "ws://" + s.ln.Addr().String() + "/v1" }

func (s *stalledListener) Close() {
	close(s.done)
	_ = s.ln.Close()
	s.mu.Lock()
	for _, c := range s.held {
		_ = c.Close()
	}
	s.mu.Unlock()
}
