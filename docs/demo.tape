# Records docs/demo.gif - the README hero.
# Requires VHS (brew install vhs), then from the repo root: vhs docs/demo.tape
#
# Runs against seeded demo stores for all three tools (docs/demo-seed.sh), so the
# recording never shows anyone's real sessions.
#
# All three isolations are required and each uses a different mechanism:
# CLAUDE_CONFIG_DIR, CODEX_HOME, and OPENCODE_DB. Miss one and that tool quietly
# falls back to the real store, which is how a first take of this recording ended
# up with 171 genuine sessions and their project paths in it.

Output "docs/demo.gif"

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 700
Set Padding 26
Set Theme "TokyoNight"
Set PlaybackSpeed 0.9

Hide
Type "mkdir -p /tmp/entangle-demo-bin && go build -ldflags '-X github.com/gowtham-sai-yadav/entangle/internal/cli.version=0.6.0' -o /tmp/entangle-demo-bin/entangle . && DEMO_CODEX_HOME=/tmp/entangle-demo-codex bash docs/demo-seed.sh /tmp/entangle-demo-home && clear"
Enter
Sleep 6s
Type "export PATH=/tmp/entangle-demo-bin:$PATH CLAUDE_CONFIG_DIR=/tmp/entangle-demo-home CODEX_HOME=/tmp/entangle-demo-codex OPENCODE_DB=/tmp/entangle-demo-oc.db && clear"
Enter
Sleep 1s
Show

# One list, every coding agent on the machine.
Type "entangle sessions --tool all"
Enter
Sleep 3500ms

Type "clear"
Enter
Sleep 500ms

# Hand one to a teammate: pick it, confirm, read out the code.
Type "entangle send 019f11e3 --tool codex"
Enter
Sleep 2s
Type "y"
Enter
Sleep 6s
Sleep 3s
