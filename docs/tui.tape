# Capture stills of the interactive cockpit for the README.
# Requires VHS (https://github.com/charmbracelet/vhs):  brew install vhs
# Then, from the repo root:  vhs docs/tui.tape
#
# It runs against a seeded demo history (docs/demo-seed.sh), so no real session
# titles or project paths ever appear. Writes docs/tui-home.png, tui-help.png,
# tui-send.png.
#
# VHS insists on an Output even when only stills are wanted, so the gif goes to
# /tmp. Pointing it inside docs/ leaves a 600k file next to three PNGs that are
# the actual product, waiting to be committed by accident.
#
# All three tools have to be isolated, each by a different mechanism -
# CLAUDE_CONFIG_DIR, CODEX_HOME, OPENCODE_DB - and all three have to be seeded, or
# a still that is supposed to show every agent on the machine quietly shows two.

Output "/tmp/entangle-tui-stills-scratch.gif"

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 850
Set Padding 24
Set Theme "TokyoNight"

# --- hidden setup: build the binary and seed a clean demo history ---
Hide
Type "go build -ldflags '-X github.com/gowtham-sai-yadav/entangle/internal/cli.version=0.6.0' -o /tmp/entangle-demo . && DEMO_CODEX_HOME=/tmp/entangle-demo-codex DEMO_OPENCODE_DB=/tmp/entangle-demo-oc.db bash docs/demo-seed.sh /tmp/entangle-demo-home && clear"
Enter
Sleep 5s
Show

# Every Screenshot is followed by a Sleep. VHS captures the frame buffer as it
# stands and flushes the PNG later, so two Screenshots close together can both
# resolve to the same frame - which happened here, writing the home view into
# tui-help.png as well and leaving two byte-identical files. If a still ever comes
# out as a copy of the one before it, lengthen these, don't shorten them.
#
# The first Sleep is also doing real work: the cockpit draws once at the shell's
# default size and again after the resize lands, and only the second render is
# tall enough for the full banner. Screenshot too early and the README's opening
# image is the cramped version.

# --- home: banner + session list + footer ---
# ENTANGLE_NO_UPDATE_CHECK keeps whatever release is out on the day out of the
# header, so the stills do not date themselves the moment the next one ships.
Type "CODEX_HOME=/tmp/entangle-demo-codex OPENCODE_DB=/tmp/entangle-demo-oc.db ENTANGLE_NO_UPDATE_CHECK=1 /tmp/entangle-demo tui --config-dir /tmp/entangle-demo-home"
Enter
Sleep 4s
Screenshot docs/tui-home.png
Sleep 2s

# --- the ? overlay: what every key does ---
Type "?"
Sleep 2s
Screenshot docs/tui-help.png
Sleep 2s
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
Sleep 2s
Escape
Sleep 500ms

Type "q"
Sleep 800ms
