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

	"github.com/psanford/wormhole-william/wormhole"
)

// AppID namespaces our transfers on the rendezvous server. Because two clients
// can only meet if their AppID matches, using our own means a claude-teleport
// code only ever pairs with another claude-teleport client, never a stranger on
// the public wormhole network who happened to be handed the same words.
const AppID = "github.com/gowtham-sai-yadav/claude-teleport"

// Config selects the infrastructure a transfer uses. Zero values fall back to
// the public magic-wormhole servers; set these to point at servers you host so
// you depend on no one else's uptime and your ciphertext transits only your own
// relay.
type Config struct {
	RendezvousURL string // mailbox server URL; "" = public default
	TransitRelay  string // transit relay host:port; "" = public default
	CodeWords     int    // words in the generated code (minimum 2)
}

func (c Config) client() *wormhole.Client {
	cl := &wormhole.Client{AppID: AppID}
	if c.RendezvousURL != "" {
		cl.RendezvousURL = c.RendezvousURL
	}
	if c.TransitRelay != "" {
		cl.TransitRelayAddress = c.TransitRelay
	}
	if c.CodeWords >= 2 {
		cl.PassPhraseComponentLength = c.CodeWords
	}
	return cl
}

// Progress reports bytes moved so far out of the total offered.
type Progress func(done, total int64)

// Send offers name/r over a wormhole. It calls onCode with the generated code as
// soon as it is known, so the caller can show it to the user, then blocks until
// the peer has received everything or ctx is cancelled. r must be seekable
// because the protocol reports the size up front and then streams the bytes.
func Send(ctx context.Context, cfg Config, name string, r io.ReadSeeker, onCode func(code string), progress Progress) error {
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
func Receive(ctx context.Context, cfg Config, code string) (*Incoming, error) {
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
