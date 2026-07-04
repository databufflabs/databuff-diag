---
name: databuff-oss
description: DataBuff open-source Docker install and component troubleshooting (Doris, ingest, web)
edition: optional
requires_bins: [docker, curl]
---

# DataBuff OSS diagnostics

Use this skill when the customer installed DataBuff via `ai-apm-install.sh` or runs the Docker Compose stack under the default install directory. All checks are read-only unless the user approves write policy.

## Applicable symptoms

- `ai-apm-install.sh` exits with error (root, Docker, curl, compose, download, or start failure)
- Doris FE or BE container unhealthy, restarting, or init timeout
- Ingest OTLP HTTP port **4318** unreachable or `/health` fails
- Web UI port **27403** unreachable or `/health` fails
- Need to triage startup order via `docker compose ps` and service logs

## Default paths

| Item | Default |
|------|---------|
| Install directory | `/opt/databuff-ai-apm` |
| Compose file | `/opt/databuff-ai-apm/docker-compose.yml` |
| Start / stop | `./start.sh` / `./stop.sh` in install dir |

If the user gave a different `APM_INSTALL_DIR`, substitute that path in runbook commands.

## Workflow

1. Confirm install directory exists and contains `docker-compose.yml`.
2. Match symptoms to a runbook (install failure, Doris FE/BE, ingest 4318, web 27403, compose logs).
3. Run checks in order; capture stdout/stderr verbatim.
4. Check host memory and disk when Doris BE or ingest is slow to become healthy (BE defaults to ~6g limit).
5. Summarize root cause from command output only; write actions (restart, reset, uninstall) require user approval.

## Runbooks

| ID | When to use |
|----|-------------|
| `ai-apm-install-failure` | Install script failed before or during deploy |
| `doris-fe-unhealthy` | FE not healthy, ports 8030/9030, bootstrap timeout |
| `doris-be-unhealthy` | BE not healthy, port 8040, OOM or FE dependency |
| `ingest-4318-unreachable` | OTLP HTTP or ingest health check fails |
| `web-27403-unreachable` | Web UI or `/health` on 27403 fails |
| `compose-logs-triage` | Cross-service log correlation after `compose ps` shows issues |

## Safety

- Prefer `risk: readonly` commands (`docker compose ps`, `logs`, `curl` health probes).
- Do not run `docker compose down`, `rm -rf`, or `./reset-table.sh` without explicit user approval.
