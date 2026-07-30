# Contributing to entangle

Thanks for looking. One maintainer, so the most useful thing you can do is tell me
what broke before you write code for it.

## Start here

- **Typo, docs, a clear one-line bug.** Send a PR straight away, no issue needed.
- **New behaviour, a new flag, or anything that changes what lands on disk.** Open an
  issue first. entangle writes into people's real `~/.claude`, and a design decided
  after the code exists usually means throwing the code away.
- **Issues labelled [`help wanted`](https://github.com/gowtham-sai-yadav/entangle/issues?q=is%3Aissue+is%3Aopen+label%3A%22help+wanted%22)** are free to take. Leave a comment so two people
  don't start the same thing.
- **Issues assigned to me** are already in progress. Comment if you want to argue about
  the design, but please don't build them. You would be racing code you cannot see yet.
- **A question rather than a bug?** Use
  [Discussions](https://github.com/gowtham-sai-yadav/entangle/discussions).

Found a security problem? Don't open an issue. Email the address on
[my profile](https://github.com/gowtham-sai-yadav).

## Setup

Go 1.26 or newer. Nothing else: no code generation, no external services.

```bash
git clone https://github.com/gowtham-sai-yadav/entangle
cd entangle
go build -o /tmp/entangle .    # the main package is the repo root, not ./cmd/...
/tmp/entangle sessions
```

Point it at a throwaway config directory while you work, so you are never testing
against your own history:

```bash
/tmp/entangle sessions --config-dir /tmp/fake-claude
```

## What has to pass

CI runs exactly this. Run it before you push and there are no surprises:

```bash
go vet ./...
gofmt -l .        # must print nothing
go test ./...
go build ./...
```

The test job runs on Linux, macOS **and Windows**. Path handling is the heart of this
project, so a change that passes on your machine and fails on Windows is the normal
failure. Check the matrix, not just your terminal.

One thing CI does *not* run, because it needs a live local relay:

```bash
go test -tags integration ./internal/transfer/
```

Run that yourself if you touch `internal/transfer`.

## Where things live

| Package | What it does |
|---|---|
| `main.go`, `internal/cli` | Subcommand wiring, flags, and the text users actually see |
| `internal/tui` | The interactive cockpit (`entangle` with no arguments) |
| `internal/webui` | The browser import wizard (`entangle gui`) |
| `internal/agent` | The seam between entangle and each coding tool. A tool implements `Provider`: where it keeps data, and what sessions are there |
| `internal/agentshare` | Packs and unpacks a single session from a tool other than Claude Code |
| `internal/exporter`, `internal/importer` | Whole-machine pack and restore, plus the resume-readiness check |
| `internal/bundle`, `internal/manifest` | The `.tgz` layout, and the record of each project's true absolute path |
| `internal/paths`, `internal/claudedir` | Rewriting one machine's paths for another, and locating `~/.claude` |
| `internal/transfer` | The end-to-end encrypted send and receive |
| `internal/handoff` | The invite text a sender passes to the receiver |
| `internal/redact` | Scrubbing likely secrets before anything leaves the machine |
| `internal/updater` | The once-a-day release check |

The reasoning behind the design is in [DESIGN.md](DESIGN.md). Read it before proposing
anything structural. Most "why not just" questions are answered there.

## Rules the code has to keep

These are promises already made to users in the README and in the CLI's own output.
A PR that breaks one needs to argue for it explicitly, not slip it through.

- **Never overwrite a user's file.** Import merges and reports what it skipped.
  `--overwrite` exists, and it backs up each replaced file first.
- **`--dry-run` must stay honest.** If it says nothing will be written, nothing is.
- **Secrets are scrubbed before anything leaves the machine**, and credentials are
  never included at all. Best effort is the stated bar; do not lower it.
- **Nothing is uploaded and there is no account.** The only network call entangle makes
  on its own is the anonymous release check, and `ENTANGLE_NO_UPDATE_CHECK=1` turns off
  both the message and the request.
- **`--json` output stays machine-readable.** Notices go to stderr, and only when a
  terminal is attached. Scripts and CI must never see them.
- **The GUI binds `127.0.0.1` only.**
- **Say what you left out.** If a command covers less than a user would assume, it
  prints so. `export` naming the tools it skipped is the pattern to copy.

## Commits and PRs

Branch off `main` as `type/short-slug`, e.g. `fix/windows-drive-letters`.

Commit subjects are lowercase `type(scope): what changed`, describing the effect
rather than the mechanism:

```
fix(release): tell the Homebrew formula which archive to install
feat: cover every coding agent by default
docs: say which tools whole-machine migration covers
```

Use the body to say **why**, especially for anything non-obvious. Someone reading
`git log` in a year is the audience.

Add a test when you fix a bug. 28 test files exist across `internal/`, so find the
neighbouring one and follow it.

`main` is protected, so everything lands through a PR with CI green.

## Releases

Maintainer only: pushing a `vX.Y.Z` tag builds and publishes everything through
GoReleaser: archives, checksums, the Homebrew tap, `.deb` and `.rpm`. There is no
version constant to bump; the version comes from the tag.
