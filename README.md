# claude-port

Move your **Claude Code** history between computers — Linux, macOS, Windows, in any direction.

Your Claude Code chat sessions, memory, and settings live in local files on your machine.
When you switch laptops, they get left behind. `claude-port` packs them into a single
portable file on the old machine and unpacks them on the new one, **automatically fixing
the paths** so your old conversations resume in the right place.

```
claude-port export                 # on the OLD machine -> writes a bundle
#  ... move the bundle to the new machine (USB, AirDrop, scp, cloud) ...
claude-port import bundle.tgz      # on the NEW machine -> restores it, fixing paths
```

## Why it's not just "copy a folder"

Claude Code stores each project's sessions in a folder named after the project's **absolute
path**, with the separators turned into dashes (`/home/kali/Desktop` →
`-home-kali-Desktop`). That naming is *lossy* — `/`, `.`, `_` and spaces all become `-`,
so you can never recover the real path from the folder name. On top of that, the session
files have the old machine's paths written *inside* them.

`claude-port` solves this by reading each project's true path from `~/.claude.json` and from
the `cwd` recorded inside the transcripts, storing it in a manifest, and then on import:

1. works out where each project should now live (you can remap freely),
2. renames the session folders to match the new machine and OS,
3. rewrites the old paths baked into the transcripts,
4. merges everything in **without overwriting** what's already there.

## Install

```bash
go build -o claude-port .
```

(Pre-built single-file binaries for each OS are the goal — see the roadmap.)

## Usage

```
claude-port export  [--out FILE] [--config-dir DIR]
claude-port import  <bundle> [--dry-run] [--map OLD=NEW]... [--overwrite] [--deep] [--yes]
claude-port inspect <bundle>
```

### Export (old machine)
```bash
claude-port export
# Exported 23 project(s), 1006 session(s) -> claude-port-backup-20260628-120000.tgz (240.1 MB)
```

### Inspect (anywhere)
```bash
claude-port inspect claude-port-backup-*.tgz
```

### Import (new machine)
Always preview first:
```bash
claude-port import bundle.tgz --dry-run
```
You'll see the path-remapping plan and a table of every project's old → new location.
`claude-port` guesses sensible new paths from your current home directory; override any of
them with `--map`:
```bash
claude-port import bundle.tgz --map /home/kali=/Users/gowtham
```
Then run for real:
```bash
claude-port import bundle.tgz --yes
```

| flag | meaning |
|---|---|
| `--dry-run` | show the plan, write nothing |
| `--map OLD=NEW` | remap a path prefix (repeatable); most specific match wins |
| `--target-home DIR` | override the detected home directory |
| `--overwrite` | replace existing files (backs each one up first) |
| `--deep` | rewrite old paths *everywhere* in transcripts, not just the `cwd` field |
| `--yes` | skip the confirmation prompt |

## What it does and doesn't move

**Moved:** session transcripts, per-session sidecars, project memory, user `settings.json`,
prompt history, plan files, plugin manifests, and the portable parts of `~/.claude.json`
(re-keyed to the new paths).

**Never moved:** your login. Credentials are machine-locked (macOS Keychain, Windows user
profile) and are deliberately left out. After importing, just open Claude Code and log in
once.

**Skipped as junk:** caches, telemetry queues, shell snapshots, lock files, and device
identity fields — all of which rebuild themselves on first run.

## Safety

- Default import **never overwrites** an existing file; it merges and reports what it skipped.
- `--overwrite` backs up each replaced file (and `~/.claude.json`) before touching it.
- The risky operation — rewriting paths inside message text — is **off by default**. The
  safe default only fixes the structural `cwd` field that matters for resuming. Use `--deep`
  to scrub every old path.

## Roadmap

- [x] Milestone 1 — same-OS / new-username export & import (this MVP)
- [ ] Milestone 2 — full cross-OS moves (Windows drive letters, real-machine testing every direction)
- [ ] Milestone 3 — one-button GUI wrapper, a post-import "verify" step, per-project selection

## License

MIT — see [LICENSE](LICENSE).
