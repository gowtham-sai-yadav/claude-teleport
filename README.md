<div align="center">

# claude-teleport

**Hand a [Claude Code](https://claude.com/claude-code) session to a teammate, or move your whole setup to a new machine. Private, no account, one command.**

Sessions, memory, and settings, across Linux, macOS, and Windows.

[![CI](https://github.com/gowtham-sai-yadav/claude-teleport/actions/workflows/ci.yml/badge.svg)](https://github.com/gowtham-sai-yadav/claude-teleport/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gowtham-sai-yadav/claude-teleport?sort=semver)](https://github.com/gowtham-sai-yadav/claude-teleport/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/gowtham-sai-yadav/claude-teleport)](https://goreportcard.com/report/github.com/gowtham-sai-yadav/claude-teleport)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

<img src="docs/demo.gif" alt="claude-teleport demo" width="820">

</div>

---

Your Claude Code sessions, memory, and project context live only on your machine. So the day you want to hand a conversation to a teammate, or move to a new laptop, you are stuck: copying the `.claude` folder by hand does not work, because every session is tied to the exact path your project lived at, and that path changes on another machine.

claude-teleport makes it one command. Send a live session straight to a teammate with a short code they type in, or pack your whole setup and carry it to a new machine, with every path rewritten so the conversation resumes right where it left off.

It is private by default. Nothing is uploaded to a server or an account; a direct transfer is end-to-end encrypted, likely secrets are scrubbed before anything leaves your machine, and your login never travels.

## Install

macOS and Linux:

```bash
curl -fsSL https://gowthamsai.in/install.sh | sh
```

With Homebrew:

```bash
brew install gowtham-sai-yadav/tap/claude-teleport
```

Windows (PowerShell):

```powershell
irm https://gowthamsai.in/install.ps1 | iex
```

Each one fetches the right prebuilt binary, verifies its checksum, and puts it on your PATH. Confirm with `claude-teleport version`.

<details>
<summary>Other ways: Go, direct download, from source</summary>

**With Go:**

```bash
go install github.com/gowtham-sai-yadav/claude-teleport@latest
```

This installs into `$(go env GOPATH)/bin` (usually `~/go/bin`). If the command is not found afterward, that folder is not on your PATH yet: `echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc`.

**Direct download:** grab the file for your machine from the [latest release](https://github.com/gowtham-sai-yadav/claude-teleport/releases/latest) (`...-darwin-arm64` for Apple Silicon, `-darwin-amd64` for Intel Macs, `-linux-amd64`, or `-windows-amd64.exe`), then on macOS/Linux `chmod +x` it and move it onto your PATH.

**From source:**

```bash
git clone https://github.com/gowtham-sai-yadav/claude-teleport
cd claude-teleport && go build -o claude-teleport .
```

</details>

## Share a session with a teammate

Hand one conversation to someone else, with all its context intact, so they can carry it forward. Find the session first:

```bash
claude-teleport sessions
```

Then send it one of two ways.

**Straight across, by code** (no file to move, nothing uploaded anywhere):

```bash
claude-teleport send <id>
```

You read out the short code it prints; they run `claude-teleport receive <code>` from their copy of the project. The transfer is end-to-end encrypted, so no server can read it.

**As a file**, if they are not around right now:

```bash
claude-teleport share <id>
```

They import it later with `claude-teleport import <file>`.

Either way, likely secrets (keys, tokens, passwords) are scrubbed before anything leaves your machine, and your login is never included. `--last` picks your most recent session, and `--with-context` also includes the project's memory files.

## Move to a new machine

Moving all of your work to a new computer is two commands, one on each.

**On the old machine**, pack everything into a file:

```bash
claude-teleport export
```

Copy the file it creates to the new machine any way you like: AirDrop, a USB stick, `scp`.

**On the new machine**, install Claude Code and sign in once, then restore:

```bash
claude-teleport import claude-teleport-backup-*.tgz
```

It shows you every project and where it will land, asks you to confirm, fixes the paths, and checks your sessions are resume-ready. Add `--dry-run` to preview without writing anything, and `--map /old/path=/new/path` if it guesses a location wrong.

Then open a project and carry on:

```bash
cd ~/path/to/your/project
claude --resume
```

> Your login does not transfer, on purpose. Credentials are locked to each machine, so just sign in once on the new one.

## Prefer clicking?

```bash
claude-teleport gui
```

opens a small wizard in your browser to pick a bundle and import it. Everything stays on your machine.

## Updating

```bash
claude-teleport update
```

checks for a newer release and swaps the binary in place. (`brew upgrade claude-teleport` works too, or re-run whichever installer you used.)

## What moves, and what doesn't

**Moves:** your sessions, project memory, settings, prompt history, and the portable parts of `~/.claude.json`, all re-pathed for the new machine.

**Never moves:** your login. Credentials are machine-locked and deliberately left out.

**Skipped:** caches, telemetry, and other throwaway files that rebuild themselves.

## Different operating systems

Import runs on the destination, so it detects the OS and translates everything, including Windows drive letters and backslashes. Linux and macOS paths look like `/home/you` or `/Users/you`; Windows uses `C:\Users\you`. You do not have to do anything special, it just works in any direction.

<details>
<summary>Is it safe, and how does it work?</summary>

**Safe by default:**

- It never overwrites an existing file. It merges and tells you what it skipped; `--overwrite` backs up each replaced file first.
- `--dry-run` shows exactly what will happen before anything is written.
- The file-based flow is fully offline. Sharing scrubs likely secrets (best effort, so glance at what you send) and never includes your login.

**How it works:**

A bundle is a `.tgz` with a manifest that records each project's true absolute path, the piece the folder name throws away, read from `~/.claude.json` and the `cwd` stored inside each transcript. On import it re-encodes the folder names for the target OS, rewrites the in-file paths (matching Windows paths in their JSON-escaped form so none are missed), merges without overwriting, and verifies every session is resume-ready.

The full design and reasoning is in [DESIGN.md](DESIGN.md).

</details>

<details>
<summary>All commands and flags</summary>

```
claude-teleport export   [--out FILE] [--config-dir DIR]
claude-teleport import   <bundle> [flags]
claude-teleport inspect  <bundle>
claude-teleport verify   [--config-dir DIR]
claude-teleport sessions [--project P] [--config-dir DIR]
claude-teleport share    <session-id | --last> [--project P] [--out FILE] [--with-context] [--no-redact] [--yes]
claude-teleport send     <session-id | --last> [--project P] [--with-context] [--no-redact] [--rendezvous URL] [--relay HOST:PORT] [--yes]
claude-teleport receive  <code> [--config-dir DIR] [--map OLD=NEW]... [--rendezvous URL] [--relay HOST:PORT] [--yes]
claude-teleport update   [--check] [--yes]
claude-teleport gui      [bundle] [--port N]
```

`import` flags:

| Flag | What it does |
|---|---|
| `--dry-run` | Show the plan and write nothing. A good first run. |
| `--map OLD=NEW` | Remap a path prefix (repeatable). The most specific match wins. |
| `--project P` | Import only this project, by path or folder (repeatable). |
| `--target-home DIR` | Override the detected home directory. |
| `--target-os OS` | Render paths for `linux`, `darwin`, or `windows`. |
| `--overwrite` | Replace existing files (each is backed up first). |
| `--deep` | Rewrite old paths everywhere in transcripts, not just the `cwd` field. |
| `--yes` | Skip the confirmation prompt. |

`inspect` shows what is inside a bundle. `verify` checks the sessions already on this machine are resume-ready. `send`/`receive` use the public magic-wormhole servers by default; point them at your own with `--rendezvous`/`--relay` or the `CLAUDE_TELEPORT_RENDEZVOUS`/`CLAUDE_TELEPORT_RELAY` environment variables.

</details>

<details>
<summary>FAQ</summary>

**Will this delete anything on my old machine?** No. `export` only reads.

**Do I need Claude Code on the new machine first?** Yes. Install it and sign in once so your account is set up, then import.

**My new username or OS is different.** That is handled. Preview with `--dry-run` and adjust with `--map` if a guess is off.

**I only want a few projects.** Use `--project <path-or-folder>` (repeatable), or tick just those in the GUI.

**Where does Claude Code keep all this?** Under `~/.claude/` and `~/.claude.json` (`%USERPROFILE%` on Windows). Set `CLAUDE_CONFIG_DIR` to relocate it; claude-teleport respects that variable.

</details>

## Contributing

Issues and pull requests are welcome. The architecture and design decisions are written up in [DESIGN.md](DESIGN.md). To develop:

```bash
go test ./...                                    # unit tests
go test -tags integration ./internal/transfer/   # a real send/receive over a local server
go vet ./... && gofmt -l .                        # checks (gofmt should print nothing)
```

CI runs on Linux, macOS, and Windows. Tagging `vX.Y.Z` builds and publishes the release automatically.

## License

[MIT](LICENSE) © Gowtham Sai Yadav
