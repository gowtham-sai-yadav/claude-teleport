// Package transfer moves a claude-teleport bundle between two machines over an
// end-to-end-encrypted channel, so a session can be handed off with a short
// spoken code instead of a file the user has to copy around.
//
// It wraps wormhole-william (a Go implementation of the magic-wormhole
// protocol). The security model is magic-wormhole's: a short human code is
// turned into a strong shared key with a password-authenticated key exchange
// (PAKE/SPAKE2), so the rendezvous server that introduces the two peers never
// learns the code, and the bulk data travels end-to-end encrypted over either a
// direct connection or a relay that only ever sees ciphertext.
//
// This package deliberately exposes just what claude-teleport needs: send a
// bundle we already built, and receive one as a stream we can hand to the
// importer. Everything else (which server, how many code words) is
// configuration.
package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/psanford/wormhole-william/wormhole"
)

// AppID namespaces our transfers on the rendezvous server. Because two clients
// can only meet if their AppID matches, using our own means a claude-teleport
// code only ever pairs with another claude-teleport client, never a stranger on
// the public wormhole network who happened to be handed the same words.
//
// This string is wire compatibility: two peers meet only if it matches exactly,
// and there is no way to negotiate it. Never change it, not even to match a
// renamed product - doing so would silently break send/receive against every
// version already installed.
const AppID = "github.com/gowtham-sai-yadav/claude-teleport"

// The library's default mailbox speaks plain HTTP on port 4000 and its default
// transit relay uses 4001. Conference, campus, and office networks routinely
// allow only 80 and 443, so those defaults simply cannot be reached from a lot
// of the places people actually work. These public alternates (run by Least
// Authority, of Winden fame) serve the same protocol over TLS on 443, which
// almost every restrictive network permits.
const (
	FallbackRendezvousURL = "wss://mailbox.mw.leastauthority.com/v1"
	FallbackTransitRelay  = "relay.mw.leastauthority.com:4001"
)

// handshakeTimeout bounds how long the default mailbox gets to produce a code
// before we give up on it and try the alternate.
//
// This exists because of a failure that a connection error cannot describe: the
// public default has been observed accepting the TCP connection, answering an
// ordinary HTTP request with 200, and then never completing the wormhole
// handshake. Nothing fails, so retrying "on error" never triggers - the user just
// watches a spinner. Waiting for a code, rather than waiting for an error, is the
// only thing that catches it.
//
// A healthy send reaches its code in about 3.6 seconds end to end (measured, and
// that figure includes process start and building the bundle), so this leaves
// roughly three times the margin. It only ever delays the failure case.
const handshakeTimeout = 10 * time.Second

// errHandshakeStalled means the mailbox accepted a connection but never got as far
// as issuing a code.
var errHandshakeStalled = errors.New("the transfer server accepted the connection but stopped responding")

// Config selects the infrastructure a transfer uses. Zero values use the public
// magic-wormhole servers; set these to point at servers you host so you depend
// on no one else's uptime and your ciphertext transits only your own relay.
type Config struct {
	RendezvousURL string // mailbox server URL; "" = public default
	TransitRelay  string // transit relay host:port; "" = public default
	CodeWords     int    // words in the generated code (minimum 2)

	// NoFallback disables the automatic retry over TLS/443 when the default
	// mailbox cannot be reached. It has no effect when RendezvousURL is set,
	// because an explicit choice of server is always honoured as given.
	NoFallback bool

	// testPrimary and testFallback replace the public endpoints without counting as
	// a user's explicit choice, so a test can point the whole fallback-and-race
	// path at local servers. They are unexported: only this package's tests can set
	// them, and a caller cannot reach them at all.
	testPrimary  string
	testFallback string

	// OnFallback, if set, is called once when a transfer moves to the alternate
	// servers, with the cause. It is for telling the user what happened; no action
	// is needed from either side, because Receive tries both mailboxes at once and
	// finds the sender on whichever one answered.
	OnFallback func(mailbox, relay string, cause error)
}

func (c Config) client() *wormhole.Client {
	cl := &wormhole.Client{AppID: AppID}
	switch {
	case c.RendezvousURL != "":
		cl.RendezvousURL = c.RendezvousURL
	case c.testPrimary != "":
		cl.RendezvousURL = c.testPrimary
	}
	if c.TransitRelay != "" {
		cl.TransitRelayAddress = c.TransitRelay
	}
	if c.CodeWords >= 2 {
		cl.PassPhraseComponentLength = c.CodeWords
	}
	return cl
}

// canFallback reports whether an unreachable server should be retried on the
// alternates. Only the library default is ever second-guessed: if the user named
// a server, that is the server we use, and a failure is reported as a failure.
func (c Config) canFallback() bool {
	return !c.NoFallback && c.RendezvousURL == ""
}

// withFallback returns a copy pointed at the TLS/443 alternates, preserving an
// explicitly configured transit relay.
func (c Config) withFallback() Config {
	out := c
	out.RendezvousURL = FallbackRendezvousURL
	if c.testFallback != "" {
		out.RendezvousURL = c.testFallback
	}
	if out.TransitRelay == "" {
		out.TransitRelay = FallbackTransitRelay
	}
	return out
}

// isReachabilityError reports whether err means "could not reach the server"
// rather than a protocol, code, or peer problem. Only the former is worth
// retrying elsewhere; retrying a wrong code on another mailbox would just fail
// twice and confuse the user.
//
// The websocket layer wraps the underlying dial error in strings rather than
// preserving it for errors.As in every case, so this checks both the typed
// errors and the text they are reported as.
func isReachabilityError(err error) bool {
	if err == nil {
		return false
	}
	// A context cancellation is the user giving up, not an unreachable server.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, marker := range []string{
		"websocket dial",
		"dial tcp",
		"dial ws",
		"connection refused",
		"connection reset",
		"i/o timeout",
		"no such host",
		"network is unreachable",
		"no route to host",
		"operation timed out",
		"handshake request",
		"tls handshake",
		"eof",
	} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// unreachable renders the "we tried, both were blocked" error in terms a user
// can act on, since the raw dial error names a port and explains nothing.
func unreachable(primary, fallback string, err error) error {
	if fallback == "" {
		return fmt.Errorf("could not reach the transfer server at %s: %w", primary, err)
	}
	return fmt.Errorf("could not reach the transfer server on this network (tried %s, then %s). "+
		"A firewall is likely blocking it; try another network, or run your own server and pass --rendezvous: %w",
		primary, fallback, err)
}

// primaryMailbox names the mailbox a config will actually use, for messages.
func (c Config) primaryMailbox() string {
	switch {
	case c.RendezvousURL != "":
		return c.RendezvousURL
	case c.testPrimary != "":
		return c.testPrimary
	}
	return wormhole.DefaultRendezvousURL
}

// Progress reports bytes moved so far out of the total offered.
type Progress func(done, total int64)

// Send offers name/r over a wormhole. It calls onCode with the generated code as
// soon as it is known, so the caller can show it to the user, then blocks until
// the peer has received everything or ctx is cancelled. r must be seekable
// because the protocol reports the size up front and then streams the bytes.
//
// If the default mailbox cannot be reached - typically a network that blocks its
// non-standard port - Send retries once over TLS/443 and reports the switch
// through cfg.OnFallback. The peer must then use the same mailbox.
func Send(ctx context.Context, cfg Config, name string, r io.ReadSeeker, onCode func(code string), progress Progress) error {
	// A server the user named is used exactly as given: no second-guessing, and no
	// silently routing their data somewhere they did not choose.
	if !cfg.canFallback() {
		return sendOnce(ctx, cfg, name, r, onCode, progress)
	}

	err := attemptSend(ctx, cfg, name, r, onCode, progress)
	if err == nil || !worthRetryingElsewhere(err) {
		return err
	}

	fb := cfg.withFallback()
	// The bundle may have been partially consumed before the failure surfaced, and
	// the protocol streams from the current offset. Without a clean rewind a retry
	// would send a truncated archive, so report the original failure rather than
	// risk that.
	if _, serr := r.Seek(0, io.SeekStart); serr != nil {
		return unreachable(cfg.primaryMailbox(), "", err)
	}
	if cfg.OnFallback != nil {
		cfg.OnFallback(fb.RendezvousURL, fb.TransitRelay, err)
	}
	// The alternate gets whatever time the caller allowed, unbounded by the
	// handshake window: once it answers, the wait is for the peer, not the server.
	if ferr := sendOnce(ctx, fb, name, r, onCode, progress); ferr != nil {
		if worthRetryingElsewhere(ferr) {
			return unreachable(cfg.primaryMailbox(), fb.RendezvousURL, ferr)
		}
		return ferr
	}
	return nil
}

// attemptSend runs one send, giving the server only handshakeTimeout to produce a
// code. Once a code exists the handshake has plainly worked, so the deadline is
// dropped and the send runs for as long as the caller's context allows - the
// remaining wait is for a human to type the code, which can take minutes.
func attemptSend(ctx context.Context, cfg Config, name string, r io.ReadSeeker, onCode func(code string), progress Progress) error {
	attemptCtx, abandon := context.WithCancel(ctx)
	defer abandon()

	coded := make(chan struct{})
	var once sync.Once
	announce := func(code string) {
		once.Do(func() { close(coded) })
		if onCode != nil {
			onCode(code)
		}
	}

	done := make(chan error, 1)
	go func() { done <- sendOnce(attemptCtx, cfg, name, r, announce, progress) }()

	timer := time.NewTimer(handshakeTimeout)
	defer timer.Stop()

	select {
	case err := <-done:
		return err // finished or failed before any code appeared
	case <-coded:
		return <-done // committed to this server; wait as long as the caller allows
	case <-timer.C:
		abandon()
		<-done // let the attempt unwind before reusing the reader
		return errHandshakeStalled
	case <-ctx.Done():
		return ctx.Err()
	}
}

// worthRetryingElsewhere reports whether a failure is about reaching the server,
// as opposed to something a different server would fail at identically.
func worthRetryingElsewhere(err error) bool {
	return errors.Is(err, errHandshakeStalled) || isReachabilityError(err)
}

func sendOnce(ctx context.Context, cfg Config, name string, r io.ReadSeeker, onCode func(code string), progress Progress) error {
	cl := cfg.client()
	var opts []wormhole.SendOption
	if progress != nil {
		opts = append(opts, wormhole.WithProgress(func(done, total int64) { progress(done, total) }))
	}
	code, status, err := cl.SendFile(ctx, name, r, opts...)
	if err != nil {
		return fmt.Errorf("start transfer: %w", err)
	}
	if onCode != nil {
		onCode(code)
	}
	select {
	case res := <-status:
		if res.Error != nil {
			return res.Error
		}
		if !res.OK {
			return errors.New("the transfer did not complete")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Incoming is a bundle arriving over a wormhole: a reader over the decrypted
// bytes plus the name and size the sender offered. Size is a hint the sender
// controls, not a guarantee, so callers should still cap how much they read.
type Incoming struct {
	Name  string
	Bytes int64
	r     io.Reader
}

func (in *Incoming) Read(p []byte) (int, error) { return in.r.Read(p) }

// Receive connects with the given code and returns the incoming bundle. The
// caller must read it to completion for the sender's transfer to finish. A
// transfer that is not a single file is rejected, since a claude-teleport
// bundle is always one archive.
//
// Like Send, an unreachable default mailbox is retried once over TLS/443. Both
// sides fall back on the same trigger, so two people on the same blocked network
// still meet without either of them configuring anything.
func Receive(ctx context.Context, cfg Config, code string) (*Incoming, error) {
	if !cfg.canFallback() {
		return receiveOnce(ctx, cfg, code)
	}

	// Both mailboxes are tried at once, and whichever one the sender is waiting on
	// answers. A timeout cannot be used here the way it can when sending: a
	// receiver legitimately waits minutes for a teammate to run the command, so
	// "slow" and "broken" look identical from this side.
	//
	// Racing also removes the need for the two people to agree on a server. On the
	// mailbox the sender did not use, the code's slot simply never appears, so that
	// attempt waits harmlessly until it is abandoned.
	primary := cfg
	fb := cfg.withFallback()

	type result struct {
		in       *Incoming
		err      error
		fallback bool
	}
	results := make(chan result, 2)

	ctxA, stopA := context.WithCancel(ctx)
	ctxB, stopB := context.WithCancel(ctx)

	// The winner's context must outlive this function: the caller has not read the
	// stream yet, and cancelling it here would tear down the transfer we just
	// found. So the loser is stopped immediately and the winner is released only
	// when the caller's own context ends, which the caller controls.
	releaseWith := func(stop context.CancelFunc) {
		go func() {
			<-ctx.Done()
			stop()
		}()
	}

	go func() {
		in, err := receiveOnce(ctxA, primary, code)
		results <- result{in: in, err: err}
	}()
	go func() {
		in, err := receiveOnce(ctxB, fb, code)
		results <- result{in: in, err: err, fallback: true}
	}()

	var firstErr error
	for i := 0; i < 2; i++ {
		res := <-results
		if res.err == nil {
			if res.fallback {
				stopA()
				releaseWith(stopB)
				if cfg.OnFallback != nil {
					cfg.OnFallback(fb.RendezvousURL, fb.TransitRelay, errHandshakeStalled)
				}
			} else {
				stopB()
				releaseWith(stopA)
			}
			return res.in, nil
		}
		// A cancellation here is this function stopping its own loser, not a failure
		// worth reporting to the user.
		if firstErr == nil && !errors.Is(res.err, context.Canceled) {
			firstErr = res.err
		}
	}
	stopA()
	stopB()
	if firstErr == nil {
		firstErr = errHandshakeStalled
	}
	if worthRetryingElsewhere(firstErr) {
		return nil, unreachable(cfg.primaryMailbox(), fb.RendezvousURL, firstErr)
	}
	return nil, firstErr
}

func receiveOnce(ctx context.Context, cfg Config, code string) (*Incoming, error) {
	cl := cfg.client()
	msg, err := cl.Receive(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if msg.Type != wormhole.TransferFile {
		_ = msg.Reject()
		return nil, fmt.Errorf("expected a single bundle, got a %s transfer", msg.Type)
	}
	return &Incoming{Name: msg.Name, Bytes: msg.TransferBytes64, r: msg}, nil
}
