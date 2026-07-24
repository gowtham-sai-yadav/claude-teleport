# claude-teleport for VS Code

Hand a [Claude Code](https://claude.com/claude-code) session to a teammate over an encrypted code, or share it to a file — without leaving the editor.

This extension is a thin front-end over the [`claude-teleport`](https://github.com/gowtham-sai-yadav/claude-teleport) CLI. It adds IDE commands and a session picker; the CLI does all the work, so there is nothing to keep in sync and your data never leaves your machine except through the CLI's own end-to-end-encrypted transfer.

## Requirements

Install the `claude-teleport` binary first (v0.5.1 or newer):

```bash
curl -fsSL https://gowthamsai.in/install.sh | sh      # macOS / Linux
brew install gowtham-sai-yadav/tap/claude-teleport    # Homebrew
```

If it is not on your PATH, set **`claude-teleport.path`** in Settings.

## Commands

Open the Command Palette (`⇧⌘P`) and type "Teleport", or click **◈ teleport** in the status bar:

| Command | What it does |
|---|---|
| **Teleport: Send a session by code** | Pick a session; a terminal shows a short code and a live progress bar. Read the code to your teammate. |
| **Teleport: Receive a session** | Enter a teammate's code; the session is imported into the current workspace folder. |
| **Teleport: Share a session to a file** | Pick a session; it is packed into a `.tgz` you can send however you like. |
| **Teleport: Browse sessions** | Search your sessions and act on one (send, share, or copy its id). |

Sends and receives run in an integrated terminal, so you see the real CLI output and can cancel with `Ctrl-C`. Secrets are scrubbed before anything leaves your machine, and your login is never included.

## Settings

- **`claude-teleport.path`** — path to the binary (default: `claude-teleport`).
- **`claude-teleport.configDir`** — override the Claude config dir (passed as `--config-dir`).

## License

MIT
