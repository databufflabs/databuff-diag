#!/usr/bin/env bash
# Pi 项目调研对话验证：触发「调研下项目 …/pi，给出总结」并检查消息展示与最终总结。
#
# Usage:
#   ./scripts/pi-research-verify.sh [rounds]
#
# Env: BASE_URL, AUTH_USER, AUTH_PASS, AUTO_APPROVE (同 chat-stress.sh)

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

ROUNDS="${1:-1}"
PROMPT="${PROMPT:-调研下项目 /Users/ligang/important/open/pi，给出总结}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${LISTEN#:}}"
AUTH_USER="${AUTH_USER:-Admin}"
AUTH_PASS="${AUTH_PASS:-Databuff@123}"
AUTO_APPROVE="${AUTO_APPROVE:-1}"

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

login() {
  curl -fsS -c "$COOKIE_JAR" -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${AUTH_USER}\",\"password\":\"${AUTH_PASS}\"}" >/dev/null
}

drain_approvals() {
  local sid="$1"
  local round=0
  while [ "$round" -lt 20 ]; do
    local pending
    pending="$(curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/sessions/${sid}" | python3 -c "
import sys, json
s = json.load(sys.stdin)
p = s.get('pending_approvals') or []
print(p[0]['id'] if p else '')
")"
    [ -n "$pending" ] || break
    curl -fsS -b "$COOKIE_JAR" -N -X POST "${BASE_URL}/api/sessions/${sid}/approve" \
      -H 'Content-Type: application/json' \
      -d "{\"approval_id\":\"${pending}\",\"approved\":true}" >/dev/null
    round=$((round + 1))
    sleep 0.3
  done
}

run_round() {
  local i="$1"
  local sid
  sid="$(curl -fsS -b "$COOKIE_JAR" -N -X POST "${BASE_URL}/api/chat" \
    -H 'Content-Type: application/json' \
    -d "{\"message\":\"${PROMPT}\"}" | python3 -c "
import sys, json
sid = None
for line in sys.stdin:
    line = line.strip()
    if line.startswith('data:'):
        try:
            d = json.loads(line[5:].strip())
            if 'session_id' in d:
                sid = d['session_id']
        except Exception:
            pass
print(sid or '')
")"
  [ -n "$sid" ] || { echo "[round $i] FAIL: no session_id"; return 1; }

  if [ "$AUTO_APPROVE" = "1" ]; then
    drain_approvals "$sid"
  fi

  sleep 0.5
  curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/sessions/${sid}" \
    | python3 "${ROOT}/scripts/pi-research-verify.py"
}

main() {
  login
  log "pi research verify: ${ROUNDS} rounds, prompt=${PROMPT}"

  local i failed=0
  for i in $(seq 1 "$ROUNDS"); do
    log "round $i/$ROUNDS"
    if ! run_round "$i"; then
      failed=$((failed + 1))
    fi
  done

  if [ "$failed" -gt 0 ]; then
    die "pi research verify failed: ${failed}/${ROUNDS} rounds had issues"
  fi
  log "pi research verify passed (${ROUNDS} rounds)"
}

main "$@"
