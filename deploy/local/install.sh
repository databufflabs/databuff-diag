#!/usr/bin/env bash
# 本地重装：停止服务 → 重新编译 → 启动
#
# Usage:
#   ./install.sh
#
# Optional (透传给 start.sh):
#   LISTEN=:8787
#   SKIP_BUILD=1
#   START_SKIP_READY=1

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

chmod +x "${ROOT}/"*.sh "${REPO_ROOT}/deploy/scripts/"*.sh 2>/dev/null || true

echo "[install] stopping local databuff-diag"
"${ROOT}/stop.sh"

echo "[install] starting fresh build"
exec "${ROOT}/start.sh"
