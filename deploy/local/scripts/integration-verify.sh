#!/usr/bin/env bash
# 集成验证：本地 Docker / 远程 140 Docker / 工作区读写
#
# Usage:
#   ./scripts/integration-verify.sh
#
# Env:
#   BASE_URL, AUTH_USER, AUTH_PASS  同 chat-stress.sh
#   REMOTE_HOST_ID=host-mr63i8s5-ozgbavlj
#   SET_POLICY=open                可选，压测时减少审批打断

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

BASE_URL="${BASE_URL:-http://127.0.0.1:${LISTEN#:}}"
AUTH_USER="${AUTH_USER:-Admin}"
AUTH_PASS="${AUTH_PASS:-Databuff@123}"
AUTO_APPROVE="${AUTO_APPROVE:-1}"
REMOTE_HOST_ID="${REMOTE_HOST_ID:-host-mr63i8s5-ozgbavlj}"
SET_POLICY="${SET_POLICY:-}"

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

vlog() {
  printf '[verify] %s\n' "$*" >&2
}

login() {
  curl -fsS -c "$COOKIE_JAR" -X POST "${BASE_URL}/api/auth/login" \
    -H 'Content-Type: application/json' \
    -d "{\"username\":\"${AUTH_USER}\",\"password\":\"${AUTH_PASS}\"}" >/dev/null
}

maybe_set_policy() {
  [ -n "$SET_POLICY" ] || return 0
  local cfg body
  cfg="$(curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/config")"
  body="$(CFG_JSON="$cfg" SET_POLICY="$SET_POLICY" python3 -c "
import json, os
cfg = json.loads(os.environ['CFG_JSON'])
cfg.setdefault('policy', {})['default'] = os.environ['SET_POLICY']
print(json.dumps(cfg))
")"
  curl -fsS -b "$COOKIE_JAR" -X PUT "${BASE_URL}/api/config" \
    -H 'Content-Type: application/json' \
    -d "$body" >/dev/null
  vlog "policy set to ${SET_POLICY}"
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
    sleep 0.4
  done
}

chat_prompt() {
  local prompt="$1"
  local sid
  sid="$(curl -fsS -b "$COOKIE_JAR" -N -X POST "${BASE_URL}/api/chat" \
    -H 'Content-Type: application/json' \
    -d "{\"message\":$(python3 -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$prompt")}" | python3 -c "
import sys, json
sid = None
for line in sys.stdin:
    line = line.strip()
    if line.startswith('data:'):
        try:
            d = json.loads(line[5:].strip())
            if 'session_id' in d:
                sid = d['session_id']
            if d.get('error'):
                print('ERROR:' + d['error'], file=sys.stderr)
                raise SystemExit(2)
        except SystemExit:
            raise
        except Exception:
            pass
if not sid:
    raise SystemExit(1)
print(sid)
")"
  if [ "$AUTO_APPROVE" = "1" ]; then
    drain_approvals "$sid"
  fi
  echo "$sid"
}

assert_session() {
  local sid="$1"
  local expect_tool="$2"
  local expect_kind="${3:-}"
  SESSION_JSON="$(curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/sessions/${sid}")" \
    EXPECT_TOOL="$expect_tool" EXPECT_KIND="$expect_kind" python3 - <<'PY'
import json, os, sys, re

data = json.loads(os.environ["SESSION_JSON"])
expect_tool = os.environ.get("EXPECT_TOOL", "")
expect_kind = os.environ.get("EXPECT_KIND", "")
msgs = data.get("messages") or []
issues = []

tool_msgs = [m for m in msgs if m.get("role") == "tool"]
if expect_tool and not tool_msgs:
    issues.append(f"no_tool_result:expected_{expect_tool}")

matched = False
for m in tool_msgs:
    cmd = m.get("command") or ""
    content = m.get("content") or ""
    if expect_tool == "bash" and re.search(r"docker\s+ps", cmd + content, re.I):
        matched = True
    if expect_tool == "ssh" and ("ssh" in cmd.lower() or "192.168.50.140" in cmd or "Exit code" in content):
        matched = True
    if expect_tool == "write" and ("write" in cmd or "Successfully wrote" in content):
        matched = True
    if expect_tool == "read" and ("read" in cmd or "File:" in content):
        matched = True

if expect_tool and tool_msgs and not matched:
    issues.append(f"tool_mismatch:expected_{expect_tool}")

if expect_kind == "final_answer":
    last = msgs[-1] if msgs else {}
    if last.get("role") != "assistant" or not (last.get("content") or "").strip():
        issues.append("missing_final_answer")

pending = data.get("pending_approvals") or []
if pending:
    issues.append("pending_approval:" + pending[0].get("command", "")[:60])

if issues:
    print(json.dumps({"session_id": data.get("id"), "issues": issues, "tool_count": len(tool_msgs)}, ensure_ascii=False), file=sys.stderr)
    sys.exit(1)
# success: quiet
PY
}

verify_workspace_file() {
  local sid="$1"
  local path="$2"
  local expect="$3"
  local content
  local enc_path
  enc_path="$(python3 -c 'import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))' "$path")"
  content="$(curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/workspace/file?session_id=${sid}&path=${enc_path}" | python3 -c "
import sys, json
print(json.load(sys.stdin).get('content',''))
")"
  if ! printf '%s' "$content" | grep -q "$expect"; then
    die "workspace file ${path} missing expected content: ${expect}"
  fi
  vlog "workspace file ${path} ok"
}

run_case() {
  local name="$1"
  local prompt="$2"
  local expect_tool="$3"
  local expect_kind="${4:-}"
  vlog "case: ${name}"
  local sid
  sid="$(chat_prompt "$prompt")" || die "chat failed: ${name}"
  assert_session "$sid" "$expect_tool" "$expect_kind"
  vlog "case passed: ${name} (session ${sid:0:12}…)"
  echo "$sid"
}

main() {
  login
  maybe_set_policy
  vlog "integration verify on ${BASE_URL}"

  local sid1 sid2 sid3
  sid1="$(run_case "local-docker" "在本机执行 docker ps -a，用一句话总结有多少容器在运行" "bash" "final_answer")"
  sid2="$(run_case "remote-docker" "通过 SSH 在 192.168.50.140 上执行 docker ps -a，说明容器运行情况（host_id=${REMOTE_HOST_ID}）" "ssh" "final_answer")"
  sid3="$(run_case "file-rw" "请在工作区用 write 工具创建 notes/verify.md，内容为两行：第一行 VERIFY-LINE-1，第二行 VERIFY-LINE-2；然后用 read 工具读取该文件并确认内容正确" "write" "final_answer")"

  verify_workspace_file "$sid3" "notes/verify.md" "VERIFY-LINE-1"
  verify_workspace_file "$sid3" "notes/verify.md" "VERIFY-LINE-2"

  vlog "all integration cases passed"
}

main "$@"
