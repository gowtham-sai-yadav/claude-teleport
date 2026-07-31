# Records docs/demo.gif - the README hero.
# Requires VHS (brew install vhs), then from the repo root: vhs docs/demo.tape
#
# It records the cockpit rather than the flag-driven commands, because typing
# `entangle` with no arguments is what almost everyone actually does, and the
# hero should show the screen they will land in.
#
# Runs against seeded demo stores for all three tools (docs/demo-seed.sh), so the
# recording never shows anyone's real sessions.
#
# All three isolations are required and each uses a different mechanism:
# CLAUDE_CONFIG_DIR, CODEX_HOME, and OPENCODE_DB. Miss one and that tool quietly
# falls back to the real store, which is how a first take of this recording ended
# up with 171 genuine sessions and their project paths in it.
#
# ENTANGLE_NO_UPDATE_CHECK is set for two reasons: the header would otherwise
# carry whatever release happened to be out on the day of recording, dating the
# GIF the moment the next one ships, and the run would make a GitHub request
# nobody asked for.

Output "docs/demo.gif"

Set Shell "bash"
Set FontSize 15
Set Width 1180
Set Height 850
Set Padding 26
Set Theme "TokyoNight"
Set PlaybackSpeed 0.9

Hide
Type "mkdir -p /tmp/entangle-demo-bin && go build -ldflags=-X=github.com/gowtham-sai-yadav/entangle/internal/cli.version=$(git describe --tags --abbrev=0 | sed 's/^v//') -o /tmp/entangle-demo-bin/entangle . && DEMO_CODEX_HOME=/tmp/entangle-demo-codex DEMO_OPENCODE_DB=/tmp/entangle-demo-oc.db bash docs/demo-seed.sh /tmp/entangle-demo-home && clear"
Enter
Sleep 6s
Type "export PATH=/tmp/entangle-demo-bin:$PATH CLAUDE_CONFIG_DIR=/tmp/entangle-demo-home CODEX_HOME=/tmp/entangle-demo-codex OPENCODE_DB=/tmp/entangle-demo-oc.db ENTANGLE_NO_UPDATE_CHECK=1 && clear"
Enter
Sleep 1s
Show

# No flags, no ids to look up: one command, and every coding agent on the
# machine is in the same list.
Type "entangle"
Enter
Sleep 4s

# `t` narrows to one tool at a time and comes back to all of them, so the
# per-tool grouping is visible without leaving the list.
#
# One press per tool the seed installs, plus one to return to all of them - four,
# because docs/demo-seed.sh seeds Claude Code, Codex and opencode. Get the count
# wrong and the recording simply ends on a one-row view, which is how a take of
# this GIF went out showing a single tool under a header that had just promised
# all of them.
Type "t"
Sleep 1800ms
Type "t"
Sleep 1800ms
Type "t"
Sleep 1800ms
Type "t"
Sleep 1500ms

# Hand one to a teammate: what will be sent, then the code to read out. Changing
# the filter puts the cursor back on the first row, so one Down is the second
# session in the merged list - a Codex one. Deliberate: the hand-off is not
# Claude-only, and the preview card names the tool it will land back in.
Down
Sleep 900ms
Enter
Sleep 3s
Enter
Sleep 9s
