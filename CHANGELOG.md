# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Removed

- Root `install.sh` — install from [GitHub Releases](https://github.com/databufflabs/databuff-diag/releases) tarballs instead

## [0.1.0] - 2026-07-04

First public MVP release of **databuff-diag** — a lightweight on-site diagnostic agent for customer environments. Run a single binary in the customer network, configure LLM and SSH via Web UI or `~/.databuff-diag/config.yaml`, and troubleshoot deployments through conversational diagnostics with human-in-the-loop command approval.

### Added

#### Runtime & CLI

- Single-binary CLI: `databuff-diag serve` (default listen `:8787`), `version`, `config path`
- Embedded Web UI (no CDN); static assets served from the binary
- Session store and config persistence under `~/.databuff-diag/`
- Health endpoint for readiness checks

#### Web UI

- Shell layout with navigation (Chat, Settings, Skills preview)
- **Chat panel**: SSE streaming assistant replies, session restore, command proposal cards, tool execution output
- **Approval UI**: pending command queue with approve / reject; policy mode selector (`全部审批` / `写入审批` / `全部开放`)
- **Settings**: LLM provider selection, API key, model, `response_processor`, test connection

#### Agent & policy

- ReAct agent loop with shell tool execution and structured command proposals
- Policy engine with three session modes: `all_approval`, `write_approval` (default), `open`
- Command risk classification: readonly, write, dangerous, blocked — gates auto-run vs human approval

#### Command execution

- Local shell execution with stdout / stderr / exit code capture
- SSH remote execution (MVP): `POST /api/exec/ssh` via system `ssh` binary; same policy gates as local
- SSH host resolution from `ssh.hosts` config and `@host_id` references in chat

#### LLM integration

- Provider catalog (OpenAI, Anthropic, DeepSeek, Moonshot, 智谱, 百炼, 千帆, MiniMax, Ollama, OpenRouter, Groq, Together, custom)
- YAML + Web UI configuration for `llm.active` and per-provider settings
- Pluggable **Response Processor** registry:
  - `openai_compat` — OpenAI-shaped chat completions
  - `anthropic_messages` — Anthropic Messages API
  - `databuff_ultra_result` — DataBuff ultra / enterprise gateway (`result` field)
- Streaming chat via SSE (`POST /api/chat`)

#### Skills

- Skill loader: `SKILL.md` front matter + YAML runbooks from configurable `skills.dirs`
- Built-in skill packs under `deploy/skills/`:
  - **generic-infra** — host resources, Docker health, Kubernetes pod crash triage
  - **databuff-oss** — DataBuff OSS Docker install failure, Doris FE/BE, ingest :4318, web :27403, compose logs

#### Reports & environment capture

- Markdown session report export (`GET/POST /api/report/export`) with timeline and command audit
- Read-only environment bundle (`POST /api/collect/env-bundle`): docker version, disk, uname, compose ps → downloadable `.tar.gz`

#### Release & install

- GoReleaser cross-build: **linux** / **darwin** × **amd64** / **arm64**
- `install.sh` — one-line install from GitHub Releases (`curl | bash`) with OS/arch auto-detect
- GitHub Actions: `ci.yml` (test, size gate under 30 MB, matrix cross-build, snapshot); `release.yml` on `v*` tags
- Archive naming: `databuff-diag_{version}_{os}_{arch}.tar.gz` (includes `LICENSE`, `README.md`)

### Security

- API key storage with restricted file permissions; optional secrets vault helpers
- Dangerous / blocked command patterns rejected or always require explicit approval
- SSH credentials expected at `0600` permissions in config

[0.1.0]: https://github.com/databufflabs/databuff-diag/releases/tag/v0.1.0
