//go:build integration

// These tests exercise a real transfer over an in-process rendezvous server and
// the localhost transit path. They are gated behind the "integration" build tag
// because they need loopback networking that some sandboxes block, and a stuck
// peer connection would otherwise hang the default test run. Run them with:
//
//	go test -tags integration -timeout 90s ./internal/transfer/
package transfer

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/psanford/wormhole-william/rendezvous/rendezvousservertest"
)

// TestSendReceiveRoundTrip stands up an in-process rendezvous server and moves a
// payload from Send to Receive end to end. The transit relay is pointed at a
// dead local address so the transfer must use the direct localhost path, which
// keeps the test hermetic (no public network).
func TestSendReceiveRoundTrip(t *testing.T) {
	rs := rendezvousservertest.NewServer()
	defer rs.Close()

	cfg := Config{RendezvousURL: rs.WebSocketURL(), TransitRelay: "127.0.0.1:1"}

	payload := bytes.Repeat([]byte("claude-teleport session bytes\n"), 4000) // ~120 KB

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	codeCh := make(chan string, 1)
	var sendErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sendErr = Send(ctx, cfg, "bundle.tgz", bytes.NewReader(payload),
			func(code string) { codeCh <- code },
			nil)
	}()

	var code string
	select {
	case code = <-codeCh:
	case <-ctx.Done():
		t.Fatal("timed out waiting for the transfer code")
	}
	if code == "" {
		t.Fatal("empty transfer code")
	}

	in, err := Receive(ctx, cfg, code)
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if in.Name != "bundle.tgz" {
		t.Errorf("Name = %q, want bundle.tgz", in.Name)
	}

	got, err := io.ReadAll(in)
	if err != nil {
		t.Fatalf("read incoming: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch: got %d bytes, want %d", len(got), len(payload))
	}

	wg.Wait()
	if sendErr != nil {
		t.Fatalf("Send returned error: %v", sendErr)
	}
}

// TestReceiveBadCode confirms a nonsense code fails fast rather than hanging.
func TestReceiveBadCode(t *testing.T) {
	rs := rendezvousservertest.NewServer()
	defer rs.Close()
	cfg := Config{RendezvousURL: rs.WebSocketURL(), TransitRelay: "127.0.0.1:1"}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := Receive(ctx, cfg, "0-not-a-real-code"); err == nil {
		t.Fatal("expected an error receiving with a bogus code")
	}
}
