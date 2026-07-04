# 配置参考

本文描述 **databuff-diag** 持久化配置与运行时参数。用户配置保存在 **`~/.databuff-diag/config.yaml`**（权限 `0600`），目录权限 `0700`。查看绝对路径：

```bash
databuff-diag config path
```

首次启动若文件不存在，会自动写入默认值（见下文「完整示例」）。敏感字段（LLM `api_key`、SSH `password`）经 `secrets.key`（AES-256-GCM）加密后以 `enc:v1:…` 前缀落盘。

**分场景快速入门**（含 Web 操作步骤与 curl 示例）：

- [安装与启动](quickstart/install.md)
- [配置大模型](quickstart/configure-llm.md)
- [配置远程主机 SSH](quickstart/configure-ssh-hosts.md)

---

## 配置目录结构

```text
~/.databuff-diag/
├── config.yaml      # 本文所述主配置
├── secrets.key      # 加密主密钥（0600）
├── skills/          # 用户安装的 Skill 包（默认扫描目录之一）
├── sessions/        # 对话会话数据
├── reports/         # 导出报告
└── ssh/             # SSH ControlMaster 套接字目录（由 control_path 展开）
```

---

## 顶层字段一览

| YAML 键 | 类型 | 说明 |
|---------|------|------|
| `llm` | object | 大模型 Provider 与当前启用项 |
| `policy` | object | 命令审批默认策略 |
| `ssh` | object | SSH 连接默认值与主机列表 |
| `skills` | object | Skill 扫描目录 |
| `auth` | object | Web UI 登录凭据 |
| `sessions` | object | 会话保留与定时清理 |

> **服务监听地址**不在 `config.yaml` 中，由启动命令或环境变量指定，见下文 [服务（server）](#服务server)。

---

## `llm` — 大模型

驱动对话排障的 LLM 配置。内置 13 个 Provider 预设见 `internal/providers/catalog.yaml`，运行中可通过 `GET /api/providers` 查看。

### 结构

```yaml
llm:
  active: deepseek          # 当前启用的 provider_code
  providers:
    <provider_code>:
      enabled: true
      wire_api: openai_compat
      base_url: https://api.deepseek.com/v1
      api_key: sk-...
      model: deepseek-chat
      response_processor: openai_compat
      timeout_sec: 120
      supports_vision: false   # 可选，是否支持视觉输入
```

### 字段说明

| 字段 | 必填 | 默认 / 说明 |
|------|------|-------------|
| `active` | 是 | 当前对话使用的 `provider_code`，须存在于 `providers` |
| `providers` | — | 键为 provider 代码（如 `deepseek`、`openai`、`custom`） |
| `providers.*.enabled` | — | 是否启用；`false` 时不可选为 active |
| `providers.*.wire_api` | 推荐 | `openai_compat` 或 `anthropic`；决定 HTTP 请求形态 |
| `providers.*.base_url` | 是 | API 根 URL；OpenAI 兼容接口通常以 `/v1` 结尾 |
| `providers.*.api_key` | 视厂商 | Bearer Token；Ollama 本地可留空；保存后加密 |
| `providers.*.model` | 是 | 模型 ID |
| `providers.*.response_processor` | 可选 | 响应解析器 ID；留空时按 `wire_api` 推断 |
| `providers.*.timeout_sec` | 可选 | 请求超时秒数，默认 **120** |
| `providers.*.supports_vision` | 可选 | 是否声明支持图像输入 |

### Response Processor

| ID | 适用场景 |
|----|----------|
| `openai_compat` | OpenAI 及多数兼容 API（`choices[0].message.content`） |
| `anthropic_messages` | Anthropic Messages API（`content[0].text`） |
| `databuff_ultra_result` | DataBuff 内网 Ultra 网关（顶层 JSON 字段 `result`） |

未设置时：`wire_api: anthropic` → `anthropic_messages`；否则 → `openai_compat`。

### 默认值（首次创建）

```yaml
llm:
  active: deepseek
  providers:
    deepseek:
      enabled: false
      wire_api: openai_compat
      base_url: https://api.deepseek.com/v1
      model: deepseek-chat
      timeout_sec: 120
```

更多示例与 `POST /api/llm/test` 见 [配置大模型](quickstart/configure-llm.md)。

---

## `ssh` — 远程主机

通过系统 `ssh` 连接客户内网主机。密码登录需安装 `sshpass`；密钥登录依赖本机 `~/.ssh` 或 `ssh-agent`，配置中不保存私钥路径。

### 结构

```yaml
ssh:
  control_path: ~/.databuff-diag/ssh/%r@%h-%p
  control_persist: 10m
  hosts:
    - id: host-a1b2c3d4e5f67890
      name: prod-app-01
      host: 10.0.1.11
      port: 22
      user: deploy
      password: enc:v1:...    # 可选；保存后加密
```

### 顶层字段

| 字段 | 默认 | 说明 |
|------|------|------|
| `control_path` | `~/.databuff-diag/ssh/%r@%h-%p` | SSH ControlMaster 套接字路径模板 |
| `control_persist` | `10m` | ControlMaster 空闲保持时间 |
| `hosts` | `[]` | 已保存主机列表 |

### `hosts[]` 条目

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 推荐 | 稳定标识；对话可用 `@host_id` 引用；省略时加载时自动生成 |
| `name` | 否 | 显示名，便于自然语言指代 |
| `host` | 是 | IP 或主机名 |
| `port` | 否 | SSH 端口，默认 **22** |
| `user` | 是 | 登录用户名 |
| `password` | 否 | SSH 密码；保存后加密；留空表示密钥登录 |

### 兼容旧格式

仍支持仅写主机地址的字符串列表，加载时自动分配 `id`：

```yaml
ssh:
  hosts:
    - 10.0.0.100
```

更多 Web / API 操作见 [配置远程主机 SSH](quickstart/configure-ssh-hosts.md)。

---

## `skills` — Skill 目录

Agent 从配置的目录扫描 `SKILL.md` 与 `runbooks/*.yaml`，按场景匹配 runbook。

### 结构

```yaml
skills:
  dirs:
    - ~/.databuff-diag/skills
    - ./deploy/skills
```

### 字段说明

| 字段 | 说明 |
|------|------|
| `dirs` | Skill 根目录列表；按顺序扫描，**同名 Skill 以先发现者为准** |

### 默认值

```yaml
skills:
  dirs:
    - ~/.databuff-diag/skills
    - ./deploy/skills
```

内置 Skill 包（随仓库 `deploy/skills/`）包括 `generic-infra`、`databuff-oss`。用户可将自定义 Skill 目录解压到 `~/.databuff-diag/skills/<skill-name>/`。

运行中列表：`GET /api/skills`（需登录）。

---

## `policy` — 命令审批策略

全局默认审批档位；新建会话时可覆盖。本地与 SSH 远程命令共用同一套风险分类（`internal/policy`）。

### 结构

```yaml
policy:
  default: write_approval
```

### `default` 取值

| 值 | 说明 |
|----|------|
| `all_approval` | **全部审批** — 每条命令均需人工批准（最保守） |
| `write_approval` | **写入审批**（默认）— `readonly` 自动执行；`write` / `dangerous` 需批准 |
| `open` | **全部开放** — `readonly` 与 `write` 自动执行；仅 `dangerous` 需批准 |

风险等级：`readonly`（如 `df`、`kubectl get`）、`write`（如 `systemctl restart`）、`dangerous`、`blocked`（如 `rm -rf /`，无法执行）。

---

## `auth` — Web 登录

Web UI 与 `/api/*`（除 `/health`、`/api/auth/login`）的 HTTP Basic 会话认证凭据。

### 结构

```yaml
auth:
  username: Admin
  password: Databuff@123
```

### 字段说明

| 字段 | 默认 | 说明 |
|------|------|------|
| `username` | `Admin` | 登录用户名 |
| `password` | `Databuff@123` | 登录密码；**首次部署后请立即修改** |

`GET /api/config` 响应中 `password` 不回传。通过 `PUT /api/config` 更新时省略 `password` 可保留原值。

---

## `sessions` — 会话保留

控制历史会话的自动清理。

### 结构

```yaml
sessions:
  retention_days: 30
  cleanup_hour: 1
```

### 字段说明

| 字段 | 默认 | 说明 |
|------|------|------|
| `retention_days` | **30** | 保留天数；设为 **0** 禁用自动清理 |
| `cleanup_hour` | **1** | 每日清理执行的本地小时（0–23） |

---

## 服务（server）

HTTP 服务监听地址**不写入** `config.yaml`，在启动时指定。

### CLI

```bash
# 默认监听所有网卡 :8787
databuff-diag serve

# 自定义端口或仅本机
databuff-diag serve --listen :9000
databuff-diag serve --listen 127.0.0.1:8787
```

| 参数 | 默认 | 说明 |
|------|------|------|
| `--listen` | `:8787` | 监听地址（host:port 或 `:port`） |

### 本地开发脚本

`deploy/local/start.sh` 与 `deploy/local/install.sh` 支持环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `LISTEN` | `:8787` | 同 `--listen` |
| `SKIP_BUILD` | `0` | `1` 时跳过 `make build` |
| `START_SKIP_READY` | `0` | `1` 时不等待 `/health` 就绪 |

### 健康检查

```bash
curl http://127.0.0.1:8787/health
# {"status":"ok"}
```

防火墙与内网部署注意入站 `8787`（或自定义端口）与出站 LLM API 可达性，见 [安装与启动 — 防火墙](quickstart/install.md#防火墙与内网部署提示)。

---

## 完整示例

以下为常见生产场景的合并示例（敏感值请替换）：

```yaml
llm:
  active: deepseek
  providers:
    deepseek:
      enabled: true
      wire_api: openai_compat
      base_url: https://api.deepseek.com/v1
      api_key: sk-your-deepseek-key
      model: deepseek-chat
      response_processor: openai_compat
      timeout_sec: 120

policy:
  default: write_approval

ssh:
  control_path: ~/.databuff-diag/ssh/%r@%h-%p
  control_persist: 10m
  hosts:
    - id: host-prod-01
      name: prod-vm
      host: 10.0.1.11
      port: 22
      user: root
      password: your-ssh-password

skills:
  dirs:
    - ~/.databuff-diag/skills
    - ./deploy/skills

auth:
  username: Admin
  password: "ChangeMe-Strong-Pass"

sessions:
  retention_days: 30
  cleanup_hour: 1
```

---

## REST API 读写配置

除 `/health` 与 `POST /api/auth/login` 外，`/api/*` 需登录会话（Cookie `diag_session`）。

```bash
# 登录
curl -s -c /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"Admin","password":"Databuff@123"}'

# 读取（api_key / password 已脱敏）
curl -s -b /tmp/diag-cookies.txt http://127.0.0.1:8787/api/config | jq .

# 全量更新（PUT 为合并替换；省略 api_key / password 保留原值）
curl -s -b /tmp/diag-cookies.txt -X PUT http://127.0.0.1:8787/api/config \
  -H 'Content-Type: application/json' \
  -d @config-patch.json
```

直接编辑 YAML 后需**重启服务**或通过 Web / API **保存**以触发加密与权限校验。

---

## 安全与权限

| 措施 | 说明 |
|------|------|
| `config.yaml` **0600** | 仅属主可读写 |
| `~/.databuff-diag/` **0700** | 配置目录仅属主可访问 |
| `secrets.key` | AES-256-GCM 主密钥；丢失后已加密字段无法解密 |
| 静态加密 | `api_key`、`password` 磁盘形态为 `enc:v1:…` |
| 访问日志 | HTTP 日志不记录请求体，密钥不会写入服务日志 |

运维建议：限制 Web 端口仅内网可达；首次登录后修改 `auth` 密码；定期备份 `config.yaml` 与 `secrets.key`。

---

## 相关文档

- [文档导航](README.md)
- [安装与启动](quickstart/install.md)
- [配置大模型](quickstart/configure-llm.md)
- [配置远程主机 SSH](quickstart/configure-ssh-hosts.md)
- [开发计划](development-plan.md)
