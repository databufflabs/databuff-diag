#!/usr/bin/env bash
# Pack databuff-oss skill for customer install:
#   curl ... | tar -xC ~/.databuff-diag/skills/
set -euo pipefail

DEPLOY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "${DEPLOY_ROOT}/.." && pwd)"
SKILL_SRC="${DEPLOY_ROOT}/skills/databuff-oss"
OUT="${1:-${DEPLOY_ROOT}/dist/databuff-oss-skill.tar.gz}"

if [[ ! -f "${SKILL_SRC}/SKILL.md" ]]; then
  echo "error: ${SKILL_SRC}/SKILL.md not found" >&2
  exit 1
fi

runbook_count="$(find "${SKILL_SRC}/runbooks" -maxdepth 1 -name '*.yaml' -o -name '*.yml' 2>/dev/null | wc -l | tr -d ' ')"
if [[ "${runbook_count}" -lt 5 ]]; then
  echo "error: expected >= 5 runbooks, found ${runbook_count}" >&2
  exit 1
fi

mkdir -p "$(dirname "${OUT}")"
tar -czf "${OUT}" -C "${DEPLOY_ROOT}/skills" databuff-oss

echo "packed ${runbook_count} runbooks -> ${OUT}"
echo "install: mkdir -p ~/.databuff-diag/skills && tar -xzf ${OUT} -C ~/.databuff-diag/skills/"
