---
name: generic-infra
description: Linux, Docker, and Kubernetes read-only checks for customer environment triage
---

# Generic infrastructure diagnostics

Use this skill when symptoms point to host resources, container health, or Kubernetes pod lifecycle issues. All checks are read-only unless policy allows writes.

## Workflow

1. Match user symptoms to a runbook (`docker-health`, `k8s-pod-crash`, `host-resources`).
2. Run checks in order; capture stdout/stderr verbatim.
3. Correlate findings (restart counts, exit codes, OOM, disk pressure) before suggesting remediation.
4. Do not invent command output; if a tool is missing (`docker`, `kubectl`), report that explicitly.

## Scope

- **Docker**: running containers, restart policy, recent logs, health status.
- **Kubernetes**: pod phase, events, container restarts, last termination reason.
- **Host**: disk (`df`), memory (`free`), load and uptime.

## Safety

- Prefer `risk: readonly` commands.
- Avoid destructive actions (`docker rm`, `kubectl delete`) unless the user explicitly approves write policy.
