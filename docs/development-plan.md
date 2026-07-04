# databuff-diag 开发计划

> **本地路径**：`/Users/ligang/important/databuff/databuff-diag`  
> **远程仓库**：`github.com/databufflabs/databuff-diag`  
> **许可证**：AGPL-3.0  
> **任务跟踪**：`~/important/my/daily-task/tasks/20260704-客户环境排查工具/`

---

## 1. 目标与里程碑

| 阶段 | 目标 | 预估 | 可演示 |
|------|------|------|--------|
| **M0** | 仓库脚手架 + CI + 空服务可启动 | 3d | `databuff-diag version` |
| **M1** | LLM 层（含 Response Processor 插件）+ 配置持久化 | 1w | 设置页测试连接成功 |
| **M2** | Agent 循环 + 本机命令 + 策略闸门 | 1.5w | 对话执行只读命令 |
| **M3** | Web UI（对话 / 审批 / 设置） | 1w | 完整本机排障会话 |
| **M4** | Skill 加载器 + `generic-infra` | 1w | 匹配 runbook 排障 |
| **M5** | SSH + 环境采集包 + 报告导出 | 1w | 多机采集 Markdown 报告 |
| **M6** | `databuff-oss` Skill 包 + 安装脚本发布 | 1w | 开源安装失败场景 E2E |

**MVP 截止**：M3 + M1 中 `databuff_ultra_result` 处理器验收通过（见 §4）。

---

## 2. 仓库结构（目标）

```
databuff-diag/
├── cmd/databuff-diag/main.go
├── internal/
│   ├── server/           # HTTP + SSE + embed UI
│   ├── agent/            # ReAct 循环
│   ├── llm/
│   │   ├── client.go     # 统一 Chat 客户端
│   │   ├── provider.go   # Provider 目录与配置
│   │   └── processor/    # ★ 响应/请求格式处理器（可插拔）
│   │       ├── registry.go
│   │       ├── openai_compat.go
│   │       ├── anthropic.go
│   │       └── databuff_ultra_result.go
│   ├── policy/           # shell AST 风险分类
│   ├── exec/             # local + ssh
│   ├── skill/            # SKILL.md + runbook
│   ├── store/            # config / sessions / reports
│   ├── web/              # 静态前端 embed
│   └── providers/        # catalog.yaml embed
├── deploy/
│   ├── local/            # 本地启停脚本
│   ├── skills/           # 内置 Skill 包
│   └── scripts/          # 打包脚本
├── docs/
│   └── development-plan.md
├── LICENSE
└── README.md
```

---

## 3. 分阶段任务拆解

### M0 · 脚手架（第 1 周前半）

- [ ] `go mod init github.com/databufflabs/databuff-diag`
- [ ] AGPL-3.0 LICENSE、README、`.gitignore`
- [ ] `cobra` CLI：`serve` / `version` / `config path`
- [ ] 最小 HTTP：`GET /health` → 200
- [ ] `goreleaser` 或 Makefile 交叉编译四平台
- [ ] GitHub Actions：`go test` + `go build`

### M1 · LLM 与 Response Processor（第 1 周后半～第 2 周）

- [ ] `internal/providers/catalog.yaml`（对齐 DataBuff `/config/ai` Provider 表）
- [ ] `~/.databuff-diag/config.yaml` 读写（权限 0600）
- [ ] `llm.Client`：发 Chat 请求、读响应、流式（后续）
- [ ] **Processor 注册表**（见 §4）
- [ ] 内置处理器：`openai_compat`、`anthropic_messages`
- [ ] 内置处理器：**`databuff_ultra_result`**（北京银行 ultra 网关）
- [ ] API：`POST /api/llm/test` — 测试连接并回显首条回复
- [ ] 单元测试：各处理器对样例 JSON 的解析

### M2 · Agent + 执行 + 策略（第 3～4 周）

- [ ] `policy.Engine`：`mvdan/sh` AST → readonly / write / dangerous / blocked
- [ ] 三档策略：`all_approval` / `write_approval` / `open`
- [ ] `exec.Local`：超时、输出截断、审计日志
- [ ] Agent ReAct：propose → policy → approve → exec → observe
- [ ] 会话 JSON 持久化 `sessions/`

### M3 · Web UI（第 5 周）

- [ ] embed 静态资源
- [ ] 对话页 SSE 流式
- [ ] 策略下拉 + 审批卡片
- [ ] 设置页：Provider 卡片、Processor 选择、测试连接

### M4 · Skill（第 6 周）

- [ ] `SKILL.md` + runbook YAML 加载
- [ ] `deploy/skills/generic-infra`：docker ps/logs、kubectl get、磁盘内存
- [ ] Skill 目录热加载（可选 watch）

### M5 · SSH 与报告（第 7 周）

- [ ] SSH ControlMaster 复用
- [ ] `collect_env_bundle` 一键采集
- [ ] Markdown 报告导出

### M6 · 集成与发布（第 8 周）

- [ ] 独立仓库 `databuff-diag-skills` 或 `databuff/integrations/diag-skills`：`databuff-oss`
- [ ] GitHub Release 四平台预编译包
- [ ] 开源 Docker 安装失败场景 E2E 文档

---

## 4. LLM Response Processor 设计（新增需求）

### 4.1 背景

客户现场 LLM 网关格式不一：

| 来源 | 响应形态 |
|------|----------|
| 标准 OpenAI 兼容 | `choices[0].message.content` |
| Anthropic | Messages API 独立结构 |
| **DataBuff ultra（北京银行等）** | 顶层 **`result`** 字符串；body 可能被 **多转义一层** |

参考实现：

`ultra_2.10.2-databuff/databuff-dao/src/main/java/com/databuff/service/root/service/OpenAIService.java`

（用户口述路径 `ultra_2.10.2-databuff北京银行`，本地目录名为 `ultra_2.10.2-databuff`。）

### 4.2 接口

```go
// internal/llm/processor/processor.go

type Processor interface {
    // ID 配置项 llm.providers[].response_processor 引用
    ID() string

    // Extract 从 HTTP 响应体提取助手文本；reasoning 可选（部分模型）
    Extract(statusCode int, body []byte) (content, reasoning string, err error)
}

// 可选扩展：部分网关请求体也需定制
type RequestAdapter interface {
    AdaptRequest(req *ChatRequest) (payload []byte, headers map[string]string, err error)
}
```

注册表：

```go
func Register(p Processor)
func Get(id string) (Processor, error)
func List() []ProcessorMeta // UI 下拉
```

### 4.3 内置处理器

| ID | 说明 | 提取逻辑 |
|----|------|----------|
| `openai_compat` | 默认 | `choices[0].message.content`；兼容 `delta` 流式 |
| `anthropic_messages` | Claude | `content[0].text` |
| **`databuff_ultra_result`** | ultra OpenAIService 网关 | 见 §4.4 |

配置示例：

```yaml
llm:
  active: boc-gateway
  providers:
    boc-gateway:
      enabled: true
      base_url: https://internal-llm.example/v1/chat/completions
      api_key: "***"
      model: qwen-72b
      wire_api: openai_compat          # 请求仍走 OpenAI 形态
      response_processor: databuff_ultra_result   # ★ 响应解析
```

未指定时默认 `openai_compat`；`wire_api: anthropic` 时默认 `anthropic_messages`。

### 4.4 `databuff_ultra_result` 实现要点

对齐 Java `OpenAIService.parseResponseBodyToMap`：

**请求**（与 Java 一致，标准 OpenAI Chat）：

```json
{
  "model": "...",
  "messages": [{"role":"user","content":"..."}],
  "stream": false
}
```

**响应**（非标准 OpenAI）：

```json
{"result": "助手回复正文"}
```

**多转义 body**：整段 body 以 `{\"` 开头时，先去掉一层转义再 `json.Unmarshal`：

```go
func unescapeUltraJSON(s string) string {
    const ph = "\x01"
    return strings.ReplaceAll(
        strings.ReplaceAll(
            strings.ReplaceAll(s, `\\`, ph),
        `\"`, `"`),
    ph, `\`)
}

func isEscapedUltraJSON(trimmed string) bool {
    return strings.HasPrefix(trimmed, `{\"`)
}
```

**提取**：

1. `statusCode != 200` → 返回错误（附 body）
2. `trim` body
3. 若 `isEscapedUltraJSON` → `unescapeUltraJSON` 后再解析
4. `json.Unmarshal` → `map[string]any`
5. 取 `result` 字段转 string；缺失则报错（与 Java 行为一致）
6. 可选：若存在 `reasoning` / `reasoning_content` 填入 `reasoning` 返回值

**测试用例**（`processor/databuff_ultra_result_test.go`）：

| case | 输入 body | 期望 content |
|------|-----------|--------------|
| 标准 | `{"result":"ok"}` | `ok` |
| 转义 | `{\"result\":\"ok\"}` | `ok` |
| 缺字段 | `{"code":0}` | error |
| 非 200 | status 500 | error |

### 4.5 自定义处理器扩展（后续）

- **Phase 1**：内置 ID 硬编码在 `processor/` 目录
- **Phase 2**（可选）：`~/.databuff-diag/processors/*.yaml` 声明 JSONPath 提取规则
- **Phase 3**（可选）：WASM / Starlark 脚本处理器（客户内网特殊网关）

M1 只需完成内置注册表 + `databuff_ultra_result` + 配置项。

---

## 5. 依赖与选型

| 组件 | 库 |
|------|-----|
| CLI | `spf13/cobra` |
| HTTP | `chi` 或 stdlib |
| Shell AST | `mvdan.cc/sh/v3` |
| SSH | `golang.org/x/crypto/ssh` |
| 配置 | `gopkg.in/yaml.v3` |
| 测试 | `stretchr/testify` |

---

## 6. 验收清单（MVP）

| # | 项 | 通过标准 |
|---|-----|----------|
| 1 | 启动 | `databuff-diag serve` 本机 8787 可访问 |
| 2 | LLM 标准 | DeepSeek / OpenAI 兼容 API 测试连接成功 |
| 3 | **ultra 处理器** | 配置 `response_processor: databuff_ultra_result` 后，对 `{"result":"..."}` 及转义 body 均能提取正文 |
| 4 | 策略 | `write_approval` 下写命令需审批 |
| 5 | 本机排障 | 对话触发 `docker ps` 类只读命令并展示输出 |
| 6 | 无 DB | 重启后会话与配置可恢复 |
| 7 | 体积 | 发布包 < 30MB |

---

## 7. 风险

| 风险 | 缓解 |
|------|------|
| ultra 网关实际 body 与 Java 样本不一致 | M1 用真实北京银行环境抓一条响应固化 golden test |
| 流式 `stream:true` ultra 未定义 | M1 仅 `stream:false`；流式后续单独处理器 |
| 前端工期 | M2 可用 `curl`/API 先验 Agent，M3 再补 UI |

---

## 8. 建议执行顺序（本周）

1. **M0**：初始化仓库、`go mod`、health 端点  
2. **M1 优先**：`processor` 包 + `databuff_ultra_result` 单测（不依赖 UI）  
3. 用客户/内网 ultra 网关 URL 做一次真实 `POST /api/llm/test` 联调  
4. 并行写 `internal/providers/catalog.yaml` 与 config store  

---

## 9. 相关文档

- 产品设计：`~/important/my/daily-task/tasks/20260704-客户环境排查工具/design.md`
- 需求：`~/important/my/daily-task/tasks/20260704-客户环境排查工具/requirement.md`
- ultra 参考代码：`ultra_2.10.2-databuff/.../OpenAIService.java`
