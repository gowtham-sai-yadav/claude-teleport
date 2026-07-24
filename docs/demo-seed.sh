#!/usr/bin/env bash
# Seed a throwaway Claude config dir with a few tasteful demo sessions, so a
# screenshot or VHS recording of the cockpit shows clean sample data instead of
# your real (possibly private) history.
#
#   bash docs/demo-seed.sh /tmp/ct-demo-home
#   claude-teleport tui --config-dir /tmp/ct-demo-home
set -euo pipefail

DEMO="${1:?usage: demo-seed.sh <config-dir>}"
rm -rf "$DEMO"
mkdir -p "$DEMO/projects"

# minutes-ago -> touch timestamp, portable across macOS (BSD date) and Linux.
ago() {
  if date -v-1M >/dev/null 2>&1; then date -v-"${1}"M +%Y%m%d%H%M; else date -d "${1} minutes ago" +%Y%m%d%H%M; fi
}

seed() { # folder  cwd  id  title  msgs  minutes-ago
  local folder="$1" cwd="$2" id="$3" title="$4" msgs="$5" mins="$6"
  local dir="$DEMO/projects/$folder"
  mkdir -p "$dir"
  local f="$dir/$id.jsonl"
  : >"$f"
  # First user line becomes the session title in the list.
  printf '{"type":"user","cwd":"%s","sessionId":"%s","message":{"role":"user","content":"%s"}}\n' "$cwd" "$id" "$title" >>"$f"
  local i=1
  while [ "$i" -lt "$msgs" ]; do
    printf '{"type":"assistant","cwd":"%s","message":{"role":"assistant","content":"..."}}\n' "$cwd" >>"$f"
    i=$((i + 1))
  done
  touch -t "$(ago "$mins")" "$f"
}

#     folder                       cwd                              id                                      title                              msgs  mins-ago
seed "-Users-dev-dashboard"        "/Users/dev/dashboard"           "44ab90ee-cc33-4d22-7e2b-000000000001" "fix the flaky pagination test"    9     9
seed "-Users-dev-acme-api"         "/Users/dev/acme-api"            "8e3d21d1-aa11-4f00-9c0f-000000000002" "refactor the auth layer"          42    120
seed "-Users-dev-claude-teleport"  "/Users/dev/claude-teleport"     "1f9c77a2-bb22-4e11-8d1a-000000000003" "build the interactive TUI"        128   1440
seed "-Users-dev-payments"         "/Users/dev/payments"            "9d0e5b6c-dd44-4c33-6f3c-000000000004" "wire up the Stripe webhook"       63    4320

echo "seeded 4 demo sessions in $DEMO"
