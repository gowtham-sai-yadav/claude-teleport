# Design notes

This document records why claude-teleport is built the way it is: the problems
that are not obvious from the outside, the decisions taken, and the tradeoffs
accepted. It is written for someone deciding whether to trust, extend, or
review the code.

## The problem

Claude Code keeps everything on your machine. Under `~/.claude/` (and
`~/.claude.json`) it stores your session transcripts, project memory, settings,
and prompt history. There is no cloud copy. So the moment you change machines,
that history is stranded, and you cannot simply copy the folder across. Two
things break:

1. **Session folders are named after the project's absolute path, with a lossy
   transform.** `/home/kai/app` becomes `-home-kai-app`. Slashes, dots,
   underscores, and spaces all collapse to a single dash, so the original path
   cannot be reconstructed from the folder name, and the name is wrong on a
   machine with a different username or operating system anyway.
2. **The transcripts have the old machine's absolute paths written inside
   them.** Even if you fixed the folder names, the recorded working directory
   would still point at the old machine, and the session would not resume.

Everything below follows from those two facts.

## Architecture

The tool is a single Go binary with small, single-purpose packages:

```
internal/
  claudedir/   locate ~/.claude, discover projects, recover true paths, list sessions
  manifest/    the description carried inside every bundle
  bundle/      read/write the .tgz archive (to a file, memory, or a network stream)
  exporter/    build a bundle: full backup, or a single scrubbed session
  redact/      best-effort secret scrubbing before anything leaves the machine
  importer/    place a bundle on the new machine: re-encode folders, rewrite paths, verify
  transfer/    move a bundle between machines over an encrypted wormhole
  webui/       a local browser wizard over the same import code path
  cli/         wire the subcommands together
```

Two design rules keep this honest:

- **One place decides "what leaves your machine."** The exporter builds a bundle
  in memory once (`PrepareShare`), and the caller chooses the destination: a
  file (`share`) or a wormhole (`send`). The redaction and the "here is what is
  in it" preview live in that single path, so a file share and a network send
  can never disagree about what they include.
- **Import is one code path.** The CLI, the GUI, and a wormhole receive all end
  up calling the same importer, so the safety checks and the resume-ready
  verification apply everywhere.

## Recovering the true path

The manifest is the fix for the lossy folder name. Before packing, the exporter
records each project's real absolute path, read from two sources and preferring
the first that is available:

1. the raw keys in `~/.claude.json`, which keep the path intact, and
2. the `cwd` field recorded inside the transcripts, as a fallback.

On import the tool re-encodes the folder name for the destination, and rewrites
the paths stored inside the transcripts, so the session resumes where the user
now keeps the project.

## Cross-operating-system rewriting

Import runs on the destination, so it knows the target operating system and can
render paths in the right style, including Windows drive letters and
backslashes. The subtle part is that transcripts are JSON, so a Windows path is
stored escaped as `C:\\Users\\you`. A naive text replace of `C:\Users\you`
misses it. The rewriter matches and replaces paths in their JSON-escaped form,
so nothing is left pointing at the old machine.

By default only the structural `cwd` field is rewritten, which is all that is
needed to resume. Rewriting paths inside message text is opt-in (`--deep`),
because that is the riskier edit and most users do not need it.

## Security model

The tool assumes a bundle can be hostile, because with sharing a bundle now
arrives from someone else.

- **Path traversal (zip-slip).** Every path drawn from a bundle is joined to the
  target directory and then checked to still be contained within it. A crafted
  entry with `../` is refused and counted, never written. Legitimate bundles
  never contain `..`, so this only ever rejects an attack.
- **Secrets never leave by accident.** Export uses an allowlist: only known-safe
  fields of `~/.claude.json` are carried, so credentials, API keys, and any
  future secret field are dropped by default rather than requiring a blocklist
  to be kept up to date. Credentials are machine-locked and are never moved at
  all; you log in once on the new machine.
- **Shared sessions are scrubbed.** A transcript can contain secrets a user
  pasted into chat. Before a session is written to a share file or sent over the
  wire, `redact` masks likely secrets (API keys, tokens, JWTs, private-key
  blocks, `password=` style assignments), including secrets stored with the
  JSON-escaped quotes a transcript uses. This is best effort and is documented
  as such: it reduces exposure, it does not guarantee a clean transcript.
- **A received transfer is treated as untrusted input.** The receiver caps how
  many bytes it will accept (the offered size is set by the peer and cannot be
  trusted), streams to a temporary file, and then runs the same importer, so the
  zip-slip containment and the no-overwrite default protect the receiver exactly
  as they protect a file import.

## The transfer feature: sending a session by code

Sharing a file works, but copying a file around is friction. `send`/`receive`
removes it: the sender reads out a short code like `7-crossover-clockwork`, the
receiver types it, and the session moves directly. This is the magic-wormhole
model, and the reasoning behind the choices is:

- **Why a password-authenticated key exchange (PAKE).** A short code is
  guessable, so it cannot be used as an encryption key directly. PAKE (SPAKE2)
  turns the weak code into a strong shared key such that the rendezvous server
  which introduces the two peers never learns the code, and an attacker gets
  only a single online guess. This is what makes a three-word code safe.
- **Why a rendezvous server and a relay.** The rendezvous server only helps two
  peers find each other and passes a few short messages; it never sees plaintext.
  The bulk transfer prefers a direct peer-to-peer connection and falls back to a
  transit relay when a firewall or NAT blocks that. Both paths are end-to-end
  encrypted, so the relay only ever sees ciphertext.
- **Why wormhole-william.** Reimplementing SPAKE2 and the transit protocol
  correctly is a large, security-sensitive effort, so it was rejected. croc was
  considered, but it is built as a CLI rather than a clean library. wormhole-
  william is a mature, MIT-licensed Go implementation designed to be embedded,
  and it interoperates with the reference client. It is wrapped behind a tiny
  `transfer` package that exposes only send and receive, so the rest of the tool
  is insulated from it.
- **A claude-teleport-specific AppID.** Two wormhole clients can only meet if
  their AppID matches. Using our own namespace means a claude-teleport code only
  ever pairs with another claude-teleport client, never a stranger on the public
  network who happened to be handed the same words.
- **Self-hostable.** The rendezvous and relay addresses are overridable by flag
  or environment variable, so an organization can run its own servers and depend
  on no one else's uptime, with its ciphertext transiting only its own relay.

### Tradeoffs consciously accepted

Adding this feature changed two of the tool's original properties, and that was
a deliberate choice, not an accident:

- The tool is no longer strictly dependency-free. It now has one focused
  dependency (and a handful of small transitive ones) for the transfer path. It
  is still a single static binary.
- The tool is no longer strictly offline. `send`/`receive` is a networked mode.
  The file-based `share`/`import` path remains fully offline and is unchanged, so
  the offline guarantee still holds for anyone who wants it. The networked mode
  is opt-in, end-to-end encrypted, and can be pointed at self-hosted servers.

## Testing

- **Unit tests** cover the parts that must be correct regardless of environment:
  path encoding and remapping, the zip-slip containment check, the secret
  scrubber (including the JSON-escaped-quote case), the export allowlist, and the
  in-memory bundle build.
- **A gated integration test** (`-tags integration`) stands up an in-process
  rendezvous server and moves a payload end to end through the real send and
  receive code. It is gated because it needs loopback networking that some
  sandboxes block, and a stuck peer connection would otherwise hang the default
  test run. Continuous integration runs the default suite on Linux, macOS, and
  Windows.
