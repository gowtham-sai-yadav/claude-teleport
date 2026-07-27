//go:build integration

package transfer

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/psanford/wormhole-william/rendezvous/rendezvousservertest"
)

// A completed wormhole transfer needs the two peers to reach each other directly,
// and that does not work everywhere. These tests point the transit relay at a dead
// address so the only route left is a direct local connection, which is what keeps
// them hermetic - but it also means an environment that will not let one local
// process connect to another cannot run them at all.
//
// Without a guard, that shows up as the whole test binary hanging until the
// -timeout fires, taking every later test with it and telling you nothing. So each
// test in this package that needs a real transfer calls requireLocalWormhole
// first, which spends a few seconds finding out and skips with a reason.
//
// A skip is not a pass. It means the property was not checked here, and that is
// worth saying out loud rather than hiding behind a green run.

// localWormholeProbe caches the answer, since it costs a few seconds.
var (
	probeOnce sync.Once
	probeOK   bool
	probeWhy  string
)

// requireLocalWormhole skips the calling test unless a minimal local transfer can
// complete in this environment.
func requireLocalWormhole(t *testing.T) {
	t.Helper()
	probeOnce.Do(func() { probeOK, probeWhy = tryLocalTransfer() })
	if !probeOK {
		t.Skipf("local wormhole transfers do not complete in this environment (%s).\n"+
			"These tests need one local process to connect directly to another, since the\n"+
			"transit relay is deliberately dead. The property under test was NOT verified here.", probeWhy)
	}
}

// probeTimeout bounds the probe. It is wall-clock, not a context deadline, on
// purpose: wormhole-william's Receive blocks on an internal channel while waiting
// for the peer's key exchange and does not select on the context there, so a
// cancelled context will not unstick it. Anything that waits on that call has to
// race a timer and walk away.
const probeTimeout = 12 * time.Second

// tryLocalTransfer runs the smallest possible send-and-receive against an
// in-process rendezvous server and gives up after probeTimeout.
//
// The whole attempt runs in a goroutine that is abandoned rather than joined if it
// stalls, because it may be parked inside a library call that cannot be
// interrupted. That leaks a goroutine for the remainder of the test binary's life,
// which is the right trade here: the alternative is the binary hanging until
// -timeout kills it and reports nothing useful.
func tryLocalTransfer() (ok bool, why string) {
	type outcome struct {
		ok  bool
		why string
	}
	result := make(chan outcome, 1)

	go func() {
		rs := rendezvousservertest.NewServer()
		defer rs.Close()

		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		defer cancel()

		cfg := Config{RendezvousURL: rs.WebSocketURL(), TransitRelay: "127.0.0.1:1"}
		const probePayload = "probe"

		codeCh := make(chan string, 1)
		sendDone := make(chan error, 1)
		go func() {
			sendDone <- Send(ctx, cfg, "probe.bin", []byte(probePayload),
				func(c string) { codeCh <- c }, nil)
		}()

		var code string
		select {
		case code = <-codeCh:
		case err := <-sendDone:
			result <- outcome{false, "send failed before producing a code: " + errText(err)}
			return
		case <-ctx.Done():
			result <- outcome{false, "no transfer code within the probe deadline"}
			return
		}

		in, err := Receive(ctx, cfg, code)
		if err != nil {
			result <- outcome{false, "receive failed: " + errText(err)}
			return
		}
		got, err := io.ReadAll(in)
		if err != nil {
			result <- outcome{false, "reading the payload failed: " + errText(err)}
			return
		}
		if string(got) != probePayload {
			result <- outcome{false, "payload did not survive the local round trip"}
			return
		}
		if err := <-sendDone; err != nil {
			result <- outcome{false, "send reported: " + errText(err)}
			return
		}
		result <- outcome{true, ""}
	}()

	select {
	case r := <-result:
		return r.ok, r.why
	case <-time.After(probeTimeout + 3*time.Second):
		return false, "a local transfer did not complete within " + probeTimeout.String() +
			" (peers could not reach each other directly)"
	}
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
