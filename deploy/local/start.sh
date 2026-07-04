#!/usr/bin/env bash
# 本地启动：编译 databuff-diag → 后台运行 HTTP 服务
#
# Usage:
#   ./start.sh
#
# Optional:
#   SKIP_BUILD=1          复用已有编译产物
#   START_SKIP_READY=1    不等待 /health

set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

chmod +x "${REPO_ROOT}/deploy/scripts/"*.sh 2>/dev/null || true

if [ "${SKIP_BUILD:-0}" != "1" ]; then
  build_binary
fi

start_service
