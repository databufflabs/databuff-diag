#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
cd "$ROOT"

# shellcheck source=scripts/lib.sh
. "${ROOT}/scripts/lib.sh"

chmod +x "${REPO_ROOT}/deploy/scripts/"*.sh 2>/dev/null || true

stop_service
log "stopped"
