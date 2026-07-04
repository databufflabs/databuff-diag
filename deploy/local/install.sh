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
#   PI_VERIFY_ROUNDS=3 ./install.sh   # pi 项目调研对话验证
#   INTEGRATION_VERIFY=1 ./install.sh

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

if [ "${PI_VERIFY_ROUNDS:-0}" -gt 0 ] 2>/dev/null; then
  echo "[install] pi research verify (${PI_VERIFY_ROUNDS} rounds)"
  "${ROOT}/scripts/pi-research-verify.sh" "$PI_VERIFY_ROUNDS"
fi

if [ "${INTEGRATION_VERIFY:-0}" = "1" ]; then
  echo "[install] integration verify (docker local/remote + file rw)"
  chmod +x "${ROOT}/scripts/integration-verify.sh"
  SET_POLICY="${SET_POLICY:-open}" "${ROOT}/scripts/integration-verify.sh"
fi
