# entangle for VS Code

Hand a live coding session to a teammate over an encrypted code, or share it to a file — without leaving the editor. [Claude Code](https://claude.com/claude-code), [OpenAI Codex CLI](https://github.com/openai/codex) and [opencode](https://github.com/anomalyco/opencode) sessions are all listed together and shared the same way.

This extension is a thin front-end over the [`entangle`](https://github.com/gowtham-sai-yadav/entangle) CLI. It adds IDE commands and a session picker; the CLI does all the work, so there is nothing to keep in sync and your data never leaves your machine except through the CLI's own end-to-end-encrypted transfer.

## Requirements

The extension drives the `entangle` CLI (v0.5.1+). You do not have to install it separately: the first time you use a command, if the CLI is not already on your PATH, the extension offers to **download it for you** — the prebuilt binary for your OS, fetched from the GitHub release and verified by SHA-256 checksum, cached in the extension's storage.

Prefer to manage it yourself? Install it and it will be used automatically:

```bash
curl -fsSL https://gowthamsai.in/install.sh | sh      # macOS / Linux
brew install gowtham-sai-yadav/tap/entangle    # Homebrew
```

Or point **`entangle.path`** at a specific binary in Settings.

## Commands

Open the Command Palette (`⇧⌘P`) and type "entangle", or click **entangle** in the status bar:

| Command | What it does |
|---|---|
| **entangle: Send a session by code** | Pick a session; a terminal shows a short code and a live progress bar. Read the code to your teammate. |
| **entangle: Receive a session** | Enter a teammate's code; the session is imported into the current workspace folder. |
| **entangle: Share a session to a file** | Pick a session; it is packed into a `.tgz` you can send however you like. |
| **entangle: Browse sessions** | Search your sessions and act on one (send, share, or copy its id). |

Sends and receives run in an integrated terminal, so you see the real CLI output and can cancel with `Ctrl-C`. Secrets are scrubbed before anything leaves your machine, and your login is never included.

## Settings

- **`entangle.path`** — path to the binary (default: `entangle`).
- **`entangle.configDir`** — override the Claude config dir (passed as `--config-dir`).

## License

MIT
