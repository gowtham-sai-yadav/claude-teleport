#!/usr/bin/env bash
# Seed a throwaway Claude config dir with a few tasteful demo sessions, so a
# screenshot or VHS recording of the cockpit shows clean sample data instead of
# your real (possibly private) history.
#
#   bash docs/demo-seed.sh /tmp/ct-demo-home
#   entangle tui --config-dir /tmp/ct-demo-home
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
seed "-Users-dev-entangle"  "/Users/dev/entangle"     "1f9c77a2-bb22-4e11-8d1a-000000000003" "build the interactive TUI"        128   1440
seed "-Users-dev-payments"         "/Users/dev/payments"            "9d0e5b6c-dd44-4c33-6f3c-000000000004" "wire up the Stripe webhook"       63    4320

echo "seeded 4 demo sessions in $DEMO"

# --- Codex demo rollouts -----------------------------------------------------
# Same idea for the Codex provider: a throwaway CODEX_HOME so a screenshot never
# shows anyone's real conversations.
seed_codex() {
  local home="$1" date="$2" id="$3" cwd="$4" title="$5" turns="$6"
  local dir="$home/sessions/$date"
  mkdir -p "$dir"
  local f="$dir/rollout-$(echo "$date" | tr '/' '-')T10-00-00-$id.jsonl"
  {
    printf '{"timestamp":"2026-07-01T10:00:00.000Z","type":"session_meta","payload":{"session_id":"%s","id":"%s","timestamp":"2026-07-01T10:00:00.000Z","cwd":"%s","originator":"codex-tui","cli_version":"0.145.0","source":"cli","model_provider":"openai","base_instructions":{"text":"you are codex"}}}\n' "$id" "$id" "$cwd"
    printf '{"timestamp":"2026-07-01T10:00:00.500Z","type":"turn_context","payload":{"turn_id":"t1","cwd":"%s","model":"gpt-5.5"}}\n' "$cwd"
    printf '{"timestamp":"2026-07-01T10:00:01.000Z","type":"event_msg","payload":{"type":"user_message","message":"%s","images":[]}}\n' "$title"
    local i=0
    while [ "$i" -lt "$turns" ]; do
      printf '{"timestamp":"2026-07-01T10:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"%s"}]}}\n' "$title"
      printf '{"timestamp":"2026-07-01T10:00:03.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"..."}]}}\n'
      i=$((i + 1))
    done
  } > "$f"
}

if [ -n "${DEMO_CODEX_HOME:-}" ]; then
  rm -rf "$DEMO_CODEX_HOME"; mkdir -p "$DEMO_CODEX_HOME/sessions"
  seed_codex "$DEMO_CODEX_HOME" "2026/07/01" "019f11e3-06e6-7d40-bd91-6600ac441e01" "/Users/dev/payments" "trace the failing webhook retry" 6
  seed_codex "$DEMO_CODEX_HOME" "2026/06/28" "019f4d44-1a2b-7c00-9d10-5566aabbccdd" "/Users/dev/infra"    "tighten the deploy IAM policy"   4
  echo "seeded 2 demo Codex sessions in $DEMO_CODEX_HOME"
fi
