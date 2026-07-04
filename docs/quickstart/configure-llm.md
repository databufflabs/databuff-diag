# 配置大模型

databuff-diag 通过大模型（LLM）驱动对话排障。启动服务后，可用 **Web 设置页**、**本地配置文件** 或 **REST API** 三种方式配置 Provider、API Key、模型与响应解析器。

**前置条件**：已安装并启动服务（默认 `http://127.0.0.1:8787`）。详见 [安装与启动](install.md)。

---

## 内置 Provider 目录

服务内置 13 个 Provider 预设（`internal/providers/catalog.yaml`），包括 OpenAI、Anthropic、DeepSeek、智谱、百炼、千帆、Ollama 本地、OpenRouter 等。可通过 `GET /api/providers` 查看完整列表及默认 `base_url`、`wire_api`、`model`。

| provider_code | 显示名 | 默认 wire_api |
|---------------|--------|---------------|
| `openai` | OpenAI | `openai_compat` |
| `anthropic` | Anthropic | `anthropic` |
| `deepseek` | DeepSeek | `openai_compat` |
| `custom` | 自定义 | `openai_compat` |

完整列表以运行中实例的 `/api/providers` 为准。

---

## 方式一：Web 设置页

### 1. 进入设置

1. 浏览器打开 `http://127.0.0.1:8787`（或你的 `--listen` 地址）。
2. 使用默认账号登录（首次启动：`Admin` / `Databuff@123`，建议首次登录后修改）。
3. 点击左侧边栏底部的 **「设置」**。
4. 确认顶部 Tab 为 **「大模型」**（另一 Tab 为「远程主机」，与 LLM 无关）。

### 2. 选择并配置 Provider

1. 在提供商卡片网格中 **点击目标 Provider**（如 DeepSeek、OpenAI、自定义）。
2. 在弹出的配置窗口中填写：

   | 字段 | 说明 |
   |------|------|
   | **API Key** | 厂商密钥；留空则保留已保存的密钥 |
   | **Base URL** | 接口根地址，通常以 `/v1` 结尾 |
   | **模型** | 模型 ID，如 `deepseek-chat`、`gpt-4o` |
   | **响应解析器** | 见下文「Response Processor」 |

3. 点击 **「测试连接」**：向 `POST /api/llm/test` 发送探测请求，成功时显示延迟、所用解析器与模型回复。
4. 点击 **「保存并启用」**：写入 `~/.databuff-diag/config.yaml`，并将该 Provider 设为 `llm.active`（卡片角标显示「使用中」）。

### 3. 搜索与切换

- 使用 **「搜索提供商…」** 按名称或模型过滤卡片。
- 保存后当前 Provider 即生效；对话排障将使用 `llm.active` 对应的配置。

---

## 方式二：配置文件 `~/.databuff-diag/config.yaml`

配置文件路径：`~/.databuff-diag/config.yaml`（可用 `databuff-diag config path` 查看目录）。

首次 `Load` 时若文件不存在，会自动创建默认配置。保存时文件权限为 **0600**，目录为 **0700**；`api_key` 经本地密钥加密后以 `enc:v1:` 前缀写入，**不以明文落盘**。

### `llm` 节完整示例

以下示例同时展示：公网 OpenAI 兼容 API、Anthropic、以及 **内网 DataBuff Ultra 网关**。

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

    openai:
      enabled: false
      wire_api: openai_compat
      base_url: https://api.openai.com/v1
      api_key: sk-your-openai-key
      model: gpt-4o
      response_processor: openai_compat
      timeout_sec: 120

    anthropic:
      enabled: false
      wire_api: anthropic
      base_url: https://api.anthropic.com
      api_key: sk-ant-your-key
      model: claude-sonnet-4-20250514
      response_processor: anthropic_messages
      timeout_sec: 120

    # 内网 DataBuff Ultra 网关示例（非 OpenAI 标准 JSON 形态）
    ultra:
      enabled: false
      wire_api: openai_compat
      base_url: http://llm-gateway.corp.internal/v1
      api_key: ultra-internal-token
      model: qwen-72b-chat
      response_processor: databuff_ultra_result
      timeout_sec: 180

    ollama:
      enabled: false
      wire_api: openai_compat
      base_url: http://127.0.0.1:11434/v1
      api_key: ""
      model: llama3.2
      response_processor: openai_compat
      timeout_sec: 120
```

### 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `llm.active` | 是 | 当前启用的 `provider_code` |
| `enabled` | — | 是否启用该 Provider |
| `wire_api` | 推荐 | `openai_compat` 或 `anthropic`；影响默认请求形态 |
| `base_url` | 是 | API 根 URL；客户端会自动追加 `/chat/completions` |
| `api_key` | 视厂商 | 写入 `Authorization: Bearer …`；Ollama 本地可留空 |
| `model` | 是 | 模型名称 |
| `response_processor` | 可选 | 显式指定解析器；留空时按 `wire_api` 推断 |
| `timeout_sec` | 可选 | 超时秒数，默认 120 |

**注意**：直接编辑 YAML 后需 **重启服务** 或 **通过 Web/API 保存** 才会触发加密与权限校验；手动粘贴明文 `api_key` 后，下次 `Save` 会自动加密。

---

## 方式三：REST API

除 `/api/auth/login` 与 `/health` 外，`/api/*` 均需登录会话（Cookie `diag_session`）。

### 登录获取会话

```bash
curl -s -c /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"Admin","password":"Databuff@123"}'
```

后续请求携带 `-b /tmp/diag-cookies.txt`。

### 查询 Provider 目录

```bash
curl -s -b /tmp/diag-cookies.txt http://127.0.0.1:8787/api/providers | jq .
```

### 读取 / 更新配置

```bash
# 读取当前配置
curl -s -b /tmp/diag-cookies.txt http://127.0.0.1:8787/api/config | jq .

# 更新 LLM 配置（PUT 时 api_key 留空字符串可保留已保存密钥）
curl -s -b /tmp/diag-cookies.txt -X PUT http://127.0.0.1:8787/api/config \
  -H 'Content-Type: application/json' \
  -d '{
    "llm": {
      "active": "deepseek",
      "providers": {
        "deepseek": {
          "enabled": true,
          "wire_api": "openai_compat",
          "base_url": "https://api.deepseek.com/v1",
          "api_key": "sk-your-key",
          "model": "deepseek-chat",
          "response_processor": "openai_compat"
        }
      }
    },
    "policy": { "default": "write_approval" },
    "ssh": { "hosts": [] },
    "skills": { "dirs": [] }
  }'
```

### 测试连接：`POST /api/llm/test`

发送一条用户消息 `reply ok`，解析模型回复并返回延迟与所用 Processor。可用于 **保存前验证** 或 **脚本化巡检**。

**请求体**（字段均可选；省略时合并已保存配置与目录默认值）：

| 字段 | 说明 |
|------|------|
| `provider_code` | 目录中的 code；省略则用 `llm.active` |
| `base_url` | 覆盖 Base URL |
| `api_key` | 覆盖 API Key |
| `model` | 覆盖模型 |
| `wire_api` | 覆盖 `openai_compat` / `anthropic` |
| `response_processor` | 覆盖响应解析器 |

**OpenAI 兼容示例**：

```bash
curl -s -b /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/llm/test \
  -H 'Content-Type: application/json' \
  -d '{
    "provider_code": "deepseek",
    "base_url": "https://api.deepseek.com/v1",
    "api_key": "sk-your-key",
    "model": "deepseek-chat",
    "wire_api": "openai_compat",
    "response_processor": "openai_compat"
  }'
```

**成功响应**：

```json
{
  "success": true,
  "content": "ok",
  "latency_ms": 842,
  "processor_used": "openai_compat"
}
```

**失败响应**（HTTP 仍为 200，`success` 为 false）：

```json
{
  "success": false,
  "error": "chat request: dial tcp ...",
  "latency_ms": 12,
  "processor_used": "openai_compat"
}
```

**内网 Ultra 网关示例**：

```bash
curl -s -b /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/llm/test \
  -H 'Content-Type: application/json' \
  -d '{
    "provider_code": "custom",
    "base_url": "http://llm-gateway.corp.internal/v1",
    "api_key": "ultra-internal-token",
    "model": "qwen-72b-chat",
    "wire_api": "openai_compat",
    "response_processor": "databuff_ultra_result"
  }'
```

网关需返回顶层 `result` 字段（见下节）；成功时 `processor_used` 为 `databuff_ultra_result`，`content` 为提取出的文本。

---

## Response Processor（响应解析器）

LLM 网关返回的 JSON 形态不一。`response_processor` 指定如何从 HTTP 响应体中提取助手文本。注册于 `internal/llm/processor/`，Web 下拉框提供三个选项。

| ID | 适用场景 | 提取规则 |
|----|----------|----------|
| `openai_compat` | OpenAI 及多数兼容 API | `choices[0].message.content`（或 `delta.content`） |
| `anthropic_messages` | Anthropic Messages API | `content[0].text` |
| `databuff_ultra_result` | DataBuff 内网 Ultra 网关 | 顶层 JSON 字段 `result`（支持双重转义体） |

### 默认推断规则

未设置 `response_processor` 时：

- `wire_api: anthropic` → `anthropic_messages`
- 其他 → `openai_compat`

### `databuff_ultra_result` 与内网网关

客户现场常见 **非标准 OpenAI JSON** 的 Ultra 网关，响应形如：

```json
{"result":"这是模型回复正文"}
```

部分网关会返回 **双重转义** 字符串（以 `{\"` 开头），解析器会自动还原后再取 `result`。

配置要点：

1. `base_url` 指向内网网关（如 `http://llm-gateway.corp.internal/v1`）。
2. `response_processor` 设为 `databuff_ultra_result`。
3. `wire_api` 通常仍为 `openai_compat`（请求仍走 `/chat/completions` 形态）。
4. 用 **测试连接** 或 `POST /api/llm/test` 确认 `success: true` 且 `processor_used` 正确。

---

## API Key 安全

| 措施 | 说明 |
|------|------|
| **文件权限 0600** | `config.yaml` 保存为仅属主可读写（`internal/store/config.go`） |
| **目录权限 0700** | `~/.databuff-diag/` 仅属主可访问 |
| **静态加密** | `api_key` 经 `secrets.key`（AES-256-GCM）加密存储，磁盘上为 `enc:v1:…`，非明文 |
| **不落访问日志** | HTTP 访问日志（chi `Logger`）仅记录方法、路径、状态码与耗时，**不记录请求体**，API Key 不会写入服务日志 |
| **本地会话** | API Key 通过已登录会话在浏览器内使用；请勿将含密钥的 `curl` 命令写入共享日志或工单 |

**运维建议**：

- 限制 `8787` 端口仅内网或本机可达。
- 首次登录后修改默认 `auth` 密码。
- 轮换密钥时通过 Web 或 API 更新；留空 `api_key` 的 PUT 可保留旧密钥。

---

## 常见问题

**测试连接成功但对话无回复**  
确认已 **保存并启用**，且 `llm.active` 与测试时使用的 Provider 一致。

**Ultra 网关测试失败、`missing result field`**  
检查网关响应是否含顶层 `result`；若格式不同，需联系网关方对齐或扩展 Processor。

**修改 YAML 未生效**  
重启 `databuff-diag serve`，或通过 Web **保存** 触发 `ConfigStore.Save`。

**Ollama 本地无需 Key**  
`api_key` 留空即可；`base_url` 一般为 `http://127.0.0.1:11434/v1`。

---

## 相关文档

- [安装与启动](install.md)
- [配置远程主机 SSH](configure-ssh-hosts.md)
- [开发计划 — LLM Processor 设计](../development-plan.md)
