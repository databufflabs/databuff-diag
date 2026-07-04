#!/usr/bin/env bash
# 批量触发诊断对话，统计「回答不完整」等问题模式。
#
# Usage:
#   ./scripts/chat-stress.sh [rounds] [prompt]
#
# Env:
#   BASE_URL=http://127.0.0.1:8787
#   AUTH_USER=Admin  AUTH_PASS=Databuff@123
#   AUTO_APPROVE=1       自动批准待审命令直至对话结束（默认开启）
#   SET_POLICY=write_approval  压测前临时切换策略（可选）

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

ROUNDS="${1:-5}"
PROMPT="${2:-${PROMPT:-检查 Docker 容器健康状态}}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${LISTEN#:}}"
AUTH_USER="${AUTH_USER:-Admin}"
AUTH_PASS="${AUTH_PASS:-Databuff@123}"
AUTO_APPROVE="${AUTO_APPROVE:-1}"
SET_POLICY="${SET_POLICY:-}"

COOKIE_JAR="$(mktemp)"
trap 'rm -f "$COOKIE_JAR"' EXIT

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
  log "policy set to ${SET_POLICY}"
}

get_session() {
  curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/sessions/$1"
}

drain_approvals() {
  local sid="$1"
  local round=0
  while [ "$round" -lt 12 ]; do
    local pending aid
    pending="$(get_session "$sid" | python3 -c "
import sys, json
s = json.load(sys.stdin)
p = s.get('pending_approvals') or []
print(p[0]['id'] if p else '')
")"
    [ -n "$pending" ] || break
    aid="$pending"
    curl -fsS -b "$COOKIE_JAR" -N -X POST "${BASE_URL}/api/sessions/${sid}/approve" \
      -H 'Content-Type: application/json' \
      -d "{\"approval_id\":\"${aid}\",\"approved\":true}" >/dev/null
    round=$((round + 1))
    sleep 0.3
  done
}

classify_session() {
  local sid="$1"
  local payload
  payload="$(curl -fsS -b "$COOKIE_JAR" "${BASE_URL}/api/sessions/${sid}")"
  SESSION_JSON="$payload" python3 - "$sid" <<'PY'
import json, os, sys, re

sid = sys.argv[1]
data = json.loads(os.environ["SESSION_JSON"])
msgs = data.get("messages") or []
pending = data.get("pending_approvals") or []
policy = data.get("policy_mode", "")

issues = []
if not msgs:
    issues.append("empty_session")

last = msgs[-1] if msgs else {}
last_role = last.get("role", "")
last_content = (last.get("content") or "").strip()

# incomplete ending heuristics (mirror agent/incomplete.go)
if last_role == "assistant" and last_content and last_content != "将执行命令":
    if not (len(last_content) >= 200 and ("## " in last_content and ("结论" in last_content or "诊断" in last_content))):
        tail = last_content[-100:] if len(last_content) > 100 else last_content
        if last_content.rstrip().endswith((":", "：", "...", "…")):
            issues.append("incomplete_colon_end")
        for p in ("接下来", "让我确认", "让我查看", "将执行", "现在先", "进一步检查", "正在检查", "正在查看", "让我先总结", "然后对"):
            if p in tail or p in last_content[:200]:
                issues.append("incomplete_transition")
                break

if last_role == "assistant" and '"tool"' in last_content:
    # malformed tool JSON if no command was attached and no subsequent tool output after this turn
    idx = len(msgs) - 1
    has_followup_tool = any(m.get("role") == "tool" for m in msgs[idx+1:]) if idx+1 < len(msgs) else False
    if not last.get("command") and not has_followup_tool and not last_content.strip().startswith("##"):
        issues.append("malformed_tool_json")

if last_role == "assistant" and not last_content:
    issues.append("empty_assistant")

if pending:
    risks = [p.get("risk", "") for p in pending]
    cmds = [p.get("command", "")[:80] for p in pending]
    issues.append(f"pending_approval:{','.join(risks)}:{';'.join(cmds)}")

# docker readonly misclassified in message history
for m in msgs:
    if m.get("role") != "assistant":
        continue
    cmd = m.get("command") or ""
    risk = m.get("risk") or ""
    if risk == "write" and re.search(r"docker (logs|inspect|ps)\b", cmd):
        issues.append(f"docker_readonly_as_write:{cmd[:60]}")

# docker exec -it
for m in msgs:
    cmd = m.get("command") or ""
    if re.search(r"docker exec\b.*\s-it\b", cmd):
        issues.append(f"docker_exec_it:{cmd[:60]}")

# ended without any command execution
user_turns = sum(1 for m in msgs if m.get("role") == "user")
tool_turns = sum(1 for m in msgs if m.get("role") == "tool")
if user_turns >= 1 and tool_turns == 0 and last_role == "assistant":
    issues.append("no_tool_executed")

# nudge count
nudges = sum(1 for m in msgs if m.get("role") == "system" and "tool JSON" in (m.get("content") or ""))

print(json.dumps({
    "session_id": sid,
    "policy_mode": policy,
    "msg_count": len(msgs),
    "nudges": nudges,
    "last_role": last_role,
    "last_preview": last_content[:120],
    "issues": issues or ["ok"],
}, ensure_ascii=False))
PY
}

run_round() {
  local i="$1"
  local sid err_file
  err_file="$(mktemp)"

  sid="$(curl -fsS -b "$COOKIE_JAR" -N -X POST "${BASE_URL}/api/chat" \
    -H 'Content-Type: application/json' \
    -d "{\"message\":\"${PROMPT}\"}" 2>"$err_file" | python3 -c "
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

  if [ -z "$sid" ]; then
    echo "[round $i] FAIL chat stream: $(cat "$err_file")"
    rm -f "$err_file"
    return 1
  fi
  rm -f "$err_file"

  if [ "$AUTO_APPROVE" = "1" ]; then
    drain_approvals "$sid"
  fi

  sleep 0.5
  classify_session "$sid"
}

main() {
  login
  maybe_set_policy
  log "stress test: ${ROUNDS} rounds, prompt=${PROMPT}, base=${BASE_URL}, auto_approve=${AUTO_APPROVE}"

  local i summary_file
  summary_file="$(mktemp)"
  : >"$summary_file"

  for i in $(seq 1 "$ROUNDS"); do
    log "round $i/$ROUNDS"
    if ! run_round "$i" | tee -a "$summary_file"; then
      echo "{\"round\":$i,\"issues\":[\"chat_failed\"]}" >>"$summary_file"
    fi
  done

  log "summary:"
  python3 - "$summary_file" <<'PY'
import json, sys, collections
path = sys.argv[1]
rows = []
for line in open(path):
    line = line.strip()
    if not line:
        continue
    try:
        rows.append(json.loads(line))
    except json.JSONDecodeError:
        pass

issue_counts = collections.Counter()
for r in rows:
    for iss in r.get("issues", []):
        key = iss.split(":")[0]
        issue_counts[key] += 1

print(f"rounds={len(rows)}")
for k, v in issue_counts.most_common():
    print(f"  {k}: {v}")
bad = [r for r in rows if r.get("issues") != ["ok"]]
print(f"problematic={len(bad)}/{len(rows)}")
for r in bad[:10]:
    print(f"  - {r.get('session_id','?')[:12]}… {r.get('issues')}")
if bad:
    sys.exit(1)
PY
}

main "$@"
