#!/usr/bin/env bash
# Shared helpers for deploy/local.

_LOCAL_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export LOCAL_ROOT="$(cd "${_LOCAL_LIB_DIR}/.." && pwd)"
export REPO_ROOT="$(cd "${LOCAL_ROOT}/../.." && pwd)"

export BINARY="${BINARY:-databuff-diag}"
export DIST_ROOT="${REPO_ROOT}/deploy/dist"
export BINARY_PATH="${BINARY_PATH:-${DIST_ROOT}/${BINARY}}"
export LISTEN="${LISTEN:-:8787}"
export LOCAL_RUN="${LOCAL_ROOT}/run"
export PID_FILE="${PID_FILE:-${LOCAL_RUN}/databuff-diag.pid}"
export LOG_FILE="${LOG_FILE:-${LOCAL_RUN}/databuff-diag.log}"

log() {
  printf '[local] %s\n' "$*"
}

die() {
  printf '[local] %s\n' "$*" >&2
  exit 1
}

ensure_command() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    die "required command not found: ${cmd}"
  fi
}

health_url() {
  local port="${LISTEN#:}"
  printf 'http://127.0.0.1:%s/health' "$port"
}

stop_service() {
  local pattern="databuff-diag serve --listen ${LISTEN}"

  if [ -f "$PID_FILE" ]; then
    local pid
    pid="$(cat "$PID_FILE")"
    if kill -0 "$pid" 2>/dev/null; then
      log "stopping pid ${pid}"
      kill "$pid" 2>/dev/null || true
      for _ in 1 2 3 4 5; do
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.2
      done
      if kill -0 "$pid" 2>/dev/null; then
        kill -9 "$pid" 2>/dev/null || true
      fi
    fi
    rm -f "$PID_FILE"
  fi

  if pgrep -f "$pattern" >/dev/null 2>&1; then
    log "stopping ${BINARY} (${LISTEN})"
    pkill -f "$pattern" 2>/dev/null || true
    sleep 0.5
    pkill -9 -f "$pattern" 2>/dev/null || true
  fi

  local port="${LISTEN#:}"
  if command -v lsof >/dev/null 2>&1 && [ -n "$port" ]; then
    local port_pids
    port_pids="$(lsof -ti "tcp:${port}" -sTCP:LISTEN 2>/dev/null || true)"
    if [ -n "$port_pids" ]; then
      log "freeing port ${port}"
      kill $port_pids 2>/dev/null || true
      sleep 0.3
      kill -9 $port_pids 2>/dev/null || true
    fi
  fi
}

build_binary() {
  ensure_command make
  log "building ${BINARY}"
  (cd "$REPO_ROOT" && make build)
  [ -x "$BINARY_PATH" ] || die "build failed: ${BINARY_PATH} not found"
}

wait_healthy() {
  local url
  url="$(health_url)"

  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.3
  done
  return 1
}

start_service() {
  ensure_command curl
  mkdir -p "$LOCAL_RUN"

  log "starting ${BINARY} on ${LISTEN}"
  if ! "$BINARY_PATH" serve --listen "$LISTEN" --pid-file "$PID_FILE" --log-file "$LOG_FILE"; then
    die "failed to start; see ${LOG_FILE}"
  fi

  log "ready at http://127.0.0.1:${LISTEN#:}"
  log "pid $(cat "$PID_FILE"), log ${LOG_FILE}"
}
