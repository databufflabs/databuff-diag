#!/usr/bin/env bash
# 本地重装：停止服务 → 重新编译 → 启动 →（可选）对话压测验证
#
# Usage:
#   ./install.sh
#
# Optional (透传给 start.sh):
#   LISTEN=:8787
#   SKIP_BUILD=1
#   START_SKIP_READY=1
#
# Optional verify after start:
#   VERIFY_ROUNDS=3 ./install.sh

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

chmod +x "${ROOT}/"*.sh "${ROOT}/scripts/"*.sh "${REPO_ROOT}/deploy/scripts/"*.sh 2>/dev/null || true

echo "[install] stopping local databuff-diag"
"${ROOT}/stop.sh"

echo "[install] starting fresh build"
"${ROOT}/start.sh"

if [ "${VERIFY_ROUNDS:-0}" -gt 0 ] 2>/dev/null; then
  echo "[install] chat stress verify (${VERIFY_ROUNDS} rounds)"
  "${ROOT}/scripts/chat-stress.sh" "$VERIFY_ROUNDS"
fi
