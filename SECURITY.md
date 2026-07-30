# Security

entangle moves coding sessions between machines, and a session transcript is
often the most sensitive thing on a developer's disk. This page says what the
tool actually protects, what it does not, and how to report a problem.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository: open the
**Security** tab and choose **Report a vulnerability**. That opens a private
thread visible only to the maintainers.

Please do not open a public issue for anything that lets a third party read a
transcript, recover a transfer code, or write files outside the target project.

One person maintains this project, so expect a first reply within a few days
rather than a few hours. If a report is confirmed, the fix ships in a patch
release and the advisory is published with credit unless you ask otherwise.

## Supported versions

Only the most recent release gets fixes. There are no long-term support
branches. If you are running anything older, `entangle update` moves you to
current.

## How a transfer is protected

`entangle send` and `entangle receive` speak the
[magic-wormhole](https://magic-wormhole.readthedocs.io/) protocol through
[wormhole-william](https://github.com/psanford/wormhole-william).

- The short code you read aloud is turned into a shared key by SPAKE2, a
  password-authenticated key exchange. The code itself never crosses the
  network.
- Payload keys are derived from that key with HKDF-SHA256, and the bytes are
  sealed with NaCl secretbox.
- The relay forwards ciphertext. It never holds a key and cannot read the
  session.

### What the code's length actually buys you

Codes are built from a 256-entry word list, so **each word is 8 bits**. The
default of two words is 16 bits of guessing resistance.

That number is smaller than it looks, and it is fine, because an attacker does
not get to guess offline. A wrong code fails the SPAKE2 handshake and the
transfer aborts, which both denies the attacker a second try on that code and
tells the sender something went wrong. The protection is one live guess against
1 in 65,536, not a dictionary attack against a hash.

If you are handing off something you would not want guessed even once, raise it:

```sh
entangle send <id> --code-words 3    # 24 bits instead of 16
```

### Which relay you are using

By default entangle falls back to the public mailbox and transit relay run by
Least Authority, over TLS on port 443, because the library's own defaults use
ports 4000 and 4001 that most office and conference networks block. See
`internal/transfer/transfer.go`.

These are third-party servers. They see ciphertext, plus the metadata any relay
sees: that two IP addresses exchanged a blob of a certain size at a certain
time. If that metadata matters to you, point entangle at your own:

```sh
entangle send <id> --rendezvous wss://your-mailbox/v1 --relay your-relay:4001
# or set ENTANGLE_RENDEZVOUS and ENTANGLE_RELAY
```

## Secret masking is best effort

Before anything leaves the machine, entangle scrubs text that looks like a
credential and reports how many it masked. It currently matches PEM private key
blocks, Anthropic, OpenAI, GitHub, GitLab, Slack, Google, AWS, Hugging Face and
Stripe key formats, JWTs, `Authorization: Bearer` headers, and key/value pairs
named like `password=`, `api_key:` or `client_secret =`. See
`internal/redact/redact.go`.

**This is a filter, not a guarantee.** It matches shapes it recognizes. A secret
with no distinctive format, a credential split across lines, or an internal
token from a scheme not listed above will pass straight through. Anything you
send is a file you can open first, and for a transcript you have any doubt
about, open it.

`--no-redact` turns masking off completely. Only use it when you have read what
you are sending.

## What is never included

Login credentials are not copied. A received session needs the recipient to be
logged in to their own coding tool, which is why the import flow tells them to
log in and resume rather than pretending the session arrived ready to run.

## Importing what someone sends you

An imported bundle writes session files into your project. `entangle inspect
<bundle>` shows what is inside before you commit to it, and `entangle import
--dry-run` shows what would be written without writing it. Both exist so a
bundle from someone you do not fully trust can be read before it lands. Use
them.
