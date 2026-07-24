# Capture stills of the interactive cockpit for the README.
# Requires VHS (https://github.com/charmbracelet/vhs):  brew install vhs
# Then, from the repo root:  vhs docs/tui.tape
#
# It runs against a seeded demo history (docs/demo-seed.sh), so no real session
# titles or project paths ever appear. Writes docs/tui-home.png, tui-help.png,
# tui-send.png. The .gif Output is a VHS requirement and is not used.

Output docs/.vhs-scratch.gif

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 720
Set Padding 24
Set Theme "TokyoNight"

# --- hidden setup: build the binary and seed a clean demo history ---
Hide
Type "go build -ldflags '-X github.com/gowtham-sai-yadav/claude-teleport/internal/cli.version=0.5.0' -o /tmp/ct-demo . && bash docs/demo-seed.sh /tmp/ct-demo-home && clear"
Enter
Sleep 5s
Show

# --- home: banner + session list + footer ---
Type "/tmp/ct-demo tui --config-dir /tmp/ct-demo-home"
Enter
Sleep 3s
Screenshot docs/tui-home.png

# --- the ? overlay: what every key does ---
Type "?"
Sleep 2s
Screenshot docs/tui-help.png
Escape
Sleep 700ms

# --- preview a hand-off (encrypted transport, secrets scrubbed) ---
Down
Sleep 400ms
Down
Sleep 700ms
Enter
Sleep 2500ms
Screenshot docs/tui-send.png
Escape
Sleep 500ms

Type "q"
Sleep 800ms
