<div align="center">

# claude-teleport

**Move your [Claude Code](https://claude.com/claude-code) history to a new computer - sessions, memory, and settings - with every path fixed automatically.**

Linux · macOS · Windows, in any direction.

[![CI](https://github.com/gowtham-sai-yadav/claude-teleport/actions/workflows/ci.yml/badge.svg)](https://github.com/gowtham-sai-yadav/claude-teleport/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gowtham-sai-yadav/claude-teleport?include_prereleases&sort=semver)](https://github.com/gowtham-sai-yadav/claude-teleport/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8.svg)](go.mod)
[![Go Report Card](https://goreportcard.com/badge/github.com/gowtham-sai-yadav/claude-teleport)](https://goreportcard.com/report/github.com/gowtham-sai-yadav/claude-teleport)

<img src="docs/demo.gif" alt="claude-teleport demo: migrate your Claude Code history to a new machine" width="820">

</div>

---

If you live in Claude Code, losing your sessions feels like losing part of your workspace.

When I moved from my Linux laptop to a new Mac, I wanted to bring all of it with me: my
sessions, conversation history, memory, and project context. There was no clean way to do it.
Copying the files by hand is tedious, and because project paths change between machines,
nothing just works when you get there.

So I built claude-teleport. It packs your whole Claude Code setup into one file on the old
machine and restores it on the new one, rewriting every path so your conversations resume
exactly where you left off.

```
┌─────────────┐    export     ┌───────────────┐   import (paths fixed)   ┌─────────────┐
│ OLD machine │ ────────────▶ │ one .tgz file │ ───────────────────────▶ │ NEW machine │
└─────────────┘               └───────────────┘                          └─────────────┘
```

## Contents

- [Why you need it](#why-you-need-it)
- [Install](#install)
- [Updating](#updating)
- [Quick start (5 minutes)](#quick-start-5-minutes)
- [Share one session with a teammate](#share-one-session-with-a-teammate)
- [Prefer clicking? Use the GUI](#prefer-clicking-use-the-gui)
- [Moving between different operating systems](#moving-between-different-operating-systems)
- [Command reference](#command-reference)
- [What gets moved (and what doesn't)](#what-gets-moved-and-what-doesnt)
- [Is it safe?](#is-it-safe)
- [How it works](#how-it-works)
- [FAQ](#faq)
- [Contributing](#contributing)

## Why you need it

Copying the `~/.claude` folder by hand **does not work**, for two reasons:

1. Sessions live in folders named after each project's full path, with the slashes turned into
   dashes (`/home/you/app` → `-home-you-app`). That naming is *lossy* - slashes, dots,
   underscores and spaces all become `-` - so you can't reconstruct the real path, and the
   name is wrong on a machine with a different username or OS anyway.
2. The session files have the **old machine's paths written inside them**.

claude-teleport handles both: it records each project's true path, then rebuilds the folder
names and rewrites the in-file paths for the new machine.

## Install

Pick whichever you like.

### Option A - Download a ready-made binary (no tools needed)

1. Go to the [**latest release**](https://github.com/gowtham-sai-yadav/claude-teleport/releases/latest).
2. Download the file for your computer:
   | Your machine | File |
   |---|---|
   | macOS (Apple Silicon) | `claude-teleport-darwin-arm64` |
   | macOS (Intel) | `claude-teleport-darwin-amd64` |
   | Linux | `claude-teleport-linux-amd64` |
   | Windows | `claude-teleport-windows-amd64.exe` |
3. On macOS/Linux, make it runnable and put it on your PATH:
   ```bash
   chmod +x claude-teleport-*            # allow it to run
   sudo mv claude-teleport-* /usr/local/bin/claude-teleport
   ```
   On Windows, rename it to `claude-teleport.exe` and keep it in a folder you can find.

### Option B - With Go installed

```bash
go install github.com/gowtham-sai-yadav/claude-teleport@latest
```

This drops the binary in Go's bin directory (`$(go env GOPATH)/bin`, usually `~/go/bin`).
If `claude-teleport: command not found`, that directory is not on your PATH yet. Add it:

```bash
# macOS / Linux (zsh)
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
```

On Windows, add `%USERPROFILE%\go\bin` to your PATH via System Settings.

### Option C - From source

```bash
git clone https://github.com/gowtham-sai-yadav/claude-teleport
cd claude-teleport
go build -o claude-teleport .
```

Check it works: `claude-teleport version`

## Updating

claude-teleport does not update itself. Nothing you install with `go install` or
download by hand refreshes on its own, so you stay on the version you installed
until you upgrade it yourself.

If you installed with Go, run the same command again to rebuild at the newest
release:

```bash
go install github.com/gowtham-sai-yadav/claude-teleport@latest
```

Just after a new release goes out, Go's module cache can take a few minutes to
notice the tag. If you want a specific version right away, ask for it directly,
for example `@v0.2.0`.

If you use a downloaded binary, grab the newest one from the
[latest release](https://github.com/gowtham-sai-yadav/claude-teleport/releases/latest)
and replace the old file.

Run `claude-teleport version` to see what you are on.

## Quick start (5 minutes)

You'll run two commands total - one on each computer.

### On the OLD computer

1. Open a terminal and run:
   ```bash
   claude-teleport export
   ```
2. It prints the name of a file it created, e.g.
   `claude-teleport-backup-20260628-120000.tgz`.
3. Copy that one file to the new computer - AirDrop, a USB stick, `scp`, Google Drive,
   whatever is easiest.

### On the NEW computer

4. **Install Claude Code and sign in once** (so it sets up your account), then close it.
5. Preview what will happen - this writes nothing:
   ```bash
   claude-teleport import claude-teleport-backup-20260628-120000.tgz --dry-run
   ```
   You'll see a table of every project and where it will move to. claude-teleport guesses the
   new location from your current home folder. If a guess is wrong, fix it with `--map`:
   ```bash
   claude-teleport import bundle.tgz --map /home/oldname=/Users/newname --dry-run
   ```
6. Happy with the preview? Run it for real:
   ```bash
   claude-teleport import claude-teleport-backup-20260628-120000.tgz
   ```
   It asks for confirmation, copies everything into place, fixes the paths, and finishes with
   a check that your sessions are resume-ready.
7. Put your actual project folders where the table said they'd go (e.g. clone your repos into
   `~/Desktop/...`). Then open one and resume:
   ```bash
   cd ~/Desktop/my-project
   claude --resume
   ```

That's it. Your old conversations are there.

> **One thing that does not transfer: your login.** That's on purpose - credentials are locked
> to each machine and should never travel in a file. Just sign in to Claude Code once on the
> new computer.

## Share one session with a teammate

Moving your whole setup is one thing. Sometimes you just want to hand a single
conversation to a teammate so they can pick up exactly where you left off, with
all your context intact.

First, find the session you want:

```bash
claude-teleport sessions
```

You get a list with a short ID, when it was last active, how many messages it
has, the project, and the first thing you asked, so it is easy to spot the right
one. Then pack it into a file:

```bash
claude-teleport share 8d84f55b
```

Before it writes anything, it shows you exactly what is about to leave your
machine and asks you to confirm. It **scans for and masks likely secrets**
(API keys, tokens, private keys, `password=...`) first. This is best effort, not
a guarantee, so glance at what you are sending if it matters.

Your teammate drops into their own copy of the project and imports it:

```bash
cd ~/their/copy/of/the/project
claude-teleport import claude-teleport-session-8d84f55b.tgz
```

The session attaches to whatever directory they are standing in, with every path
rewritten for their machine, so `claude --resume` picks it up right away.

Notes:

- Only the conversation is shared by default. Add `--with-context` to include the
  project's memory files too.
- `--last` shares your most recent session without needing the ID.
- `--no-redact` sends the raw transcript unscrubbed (not recommended).
- Your login is never included, here or anywhere.

## Prefer clicking? Use the GUI

Not a terminal person? Run:

```bash
claude-teleport gui
```

Your browser opens a small wizard where you pick the bundle file, confirm where each project
should go, tick the projects you want, and click **Import**. It shows a live result and the
same resume-ready check at the end. Everything runs locally on your own machine - nothing is
uploaded anywhere.

## Moving between different operating systems

Import runs **on the destination machine**, so it detects the right OS automatically and
translates everything for you, including Windows drive letters and backslashes:

| From → To | Example folder rename |
|---|---|
| Linux/macOS → Windows | `-home-you-app` → `-C-Users-you-app` |
| Windows → Linux/macOS | `-C-Users-you-app` → `-home-you-app` |

Paths stored *inside* the transcripts are rewritten in the correct style too. Because
transcripts are JSON, Windows paths are matched and re-written in their escaped form
(`C:\\Users\\you`), so nothing is missed.

## Command reference

```
claude-teleport export   [--out FILE] [--config-dir DIR]
claude-teleport import   <bundle> [flags]
claude-teleport inspect  <bundle>
claude-teleport verify   [--config-dir DIR]
claude-teleport sessions [--project P] [--config-dir DIR]
claude-teleport share    <session-id-prefix | --last> [--project P] [--out FILE] [--with-context] [--no-redact] [--yes]
claude-teleport gui      [bundle] [--port N]
```

**`import` flags**

| Flag | What it does |
|---|---|
| `--dry-run` | Show the plan and write nothing. Always start here. |
| `--map OLD=NEW` | Remap a path prefix (repeatable). The most specific match wins. |
| `--project P` | Import only this project, by its path or folder (repeatable; default: all). |
| `--target-home DIR` | Override the detected home directory. |
| `--target-os OS` | Render paths for `linux`/`darwin`/`windows` (default: this machine). |
| `--overwrite` | Replace files that already exist (each one is backed up first). |
| `--deep` | Rewrite old paths *everywhere* in transcripts, not just the `cwd` field. |
| `--yes` | Skip the confirmation prompt. |

- **`inspect`** prints what's inside a bundle without importing.
- **`verify`** checks that the sessions already on this machine are resume-ready.
- **`sessions`** lists your conversations so you can pick one to hand off.
- **`share`** packs a single session into a file for a teammate, masking likely
  secrets first. They import it from inside their own copy of the project.

## What gets moved (and what doesn't)

**Moved:** session transcripts, per-session sidecars, project memory, your user
`settings.json`, prompt history, plan files, plugin manifests, and the portable parts of
`~/.claude.json` (re-keyed to the new paths).

**Never moved - your login.** Credentials are machine-locked (macOS Keychain, Windows user
profile) and are deliberately left out. Sign in once after importing.

**Skipped as junk:** caches, telemetry, shell snapshots, lock files, and device-identity
fields - all of which rebuild themselves on first run.

## Is it safe?

- It **never overwrites** an existing file by default - it merges and tells you what it
  skipped. `--overwrite` makes a timestamped backup of each file it replaces.
- `--dry-run` shows exactly what will happen before anything is written.
- The riskiest step - rewriting paths inside message text - is **off by default**. The safe
  default only fixes the structural `cwd` field needed to resume. `--deep` opts into a full
  rewrite.
- It runs entirely offline. Your bundle and history never leave your computers.

## How it works

A bundle is a `.tgz` archive with a `manifest.json` written first. The manifest records each
project's **true absolute path** - the piece the lossy folder name throws away - read from
`~/.claude.json` and from the `cwd` stored inside each transcript. On import, claude-teleport:

1. works out a new path for each project (auto-detected, or via `--map`),
2. re-encodes the folder names for the target machine and OS,
3. rewrites the old paths inside transcripts (JSON-escaped, so Windows paths match),
4. merges everything in without overwriting, and
5. verifies each migrated project's `cwd` matches its new folder.

## FAQ

**Will this delete anything on my old machine?** No. `export` only reads.

**Do I need Claude Code installed first on the new machine?** Yes - install it and sign in
once so your account is set up, then import.

**My new username/OS is different.** That's the whole point - it's handled. Preview with
`--dry-run` and adjust with `--map` if a guess is off.

**I only want a couple of projects.** Use `--project <path-or-folder>` (repeatable), or tick
just those in the GUI.

**Where is all this stored?** Under `~/.claude/` and `~/.claude.json` (or `%USERPROFILE%` on
Windows). Set `CLAUDE_CONFIG_DIR` to relocate it; claude-teleport respects that variable.

## Contributing

Issues and pull requests are welcome. To develop:

```bash
go test ./...     # run the tests
go vet ./...      # static checks
gofmt -l .        # should print nothing
go run . gui      # try the wizard locally
```

CI runs lint and tests on Linux, macOS, and Windows. Tagging `vX.Y.Z` builds and publishes
release binaries automatically.

## Star history

<a href="https://star-history.com/#gowtham-sai-yadav/claude-teleport&Date">
  <img src="https://api.star-history.com/svg?repos=gowtham-sai-yadav/claude-teleport&type=Date" alt="Star history chart" width="600">
</a>

## License

[MIT](LICENSE) © Gowtham Sai Yadav
