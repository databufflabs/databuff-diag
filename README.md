# databuff-diag

[![Release](https://img.shields.io/github/v/release/databufflabs/databuff-diag?label=release)](https://github.com/databufflabs/databuff-diag/releases)

**DataBuff 客户环境排查助手** — 在客户内网运行的轻量诊断 Agent。通过 Web 对话完成部署与环境排障，支持本机与 SSH 远程命令执行，无需云端中控。

---

## 功能

| 能力 | 说明 |
|------|------|
| **对话排障** | Web UI 多轮对话，Agent 自动规划、执行命令并汇总结论 |
| **三档审批** | 全部审批 / 写入审批（默认）/ 全部开放，按命令风险分级闸门 |
| **LLM Processor** | 可插拔 Response Processor：`openai_compat`、`anthropic_messages`、`databuff_ultra_result` 等 |
| **Skill** | `SKILL.md` + runbook 插件，内置 `generic-infra`、`databuff-oss` |
| **SSH** | 多主机远程排障，ControlMaster 连接复用 |
| **报告** | 会话导出 Markdown 排障报告，环境采集包下载 |

---

## 安装

从 [GitHub Releases](https://github.com/databufflabs/databuff-diag/releases) 下载对应平台的预编译包（linux / darwin，amd64 / arm64），解压后安装到 `PATH`：

```bash
# 示例：Linux amd64，v0.1.0
curl -fsSL -O https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_linux_amd64.tar.gz
tar -xzf databuff-diag_0.1.0_linux_amd64.tar.gz
install -m 755 databuff-diag /usr/local/bin/databuff-diag
# 无写权限时：mv databuff-diag ~/.local/bin/ && export PATH="$HOME/.local/bin:$PATH"

databuff-diag version
```

资产命名：`databuff-diag_{版本号}_{os}_{arch}.tar.gz`（版本号不含 `v` 前缀）。更多平台与内网部署说明见 [安装与启动](docs/quickstart/install.md)。

---

## 快速开始

```bash
# 启动 HTTP 服务（默认监听 :8787）
databuff-diag serve

# 浏览器打开
open http://127.0.0.1:8787    # macOS
# 或访问 http://127.0.0.1:8787
```

首次启动会在 **`~/.databuff-diag/`** 创建本地配置与会话目录：

```
~/.databuff-diag/
├── config.yaml    # LLM、SSH、策略、Skill 路径
├── skills/        # 用户安装的 Skill 包
├── sessions/      # 对话会话
└── reports/       # 导出报告
```

查看配置目录路径：`databuff-diag config path`

健康检查：`curl http://127.0.0.1:8787/health`

---

## 文档

| 文档 | 说明 |
|------|------|
| [安装与启动](docs/quickstart/install.md) | Release 下载、本地开发模式 |
| [配置大模型](docs/quickstart/configure-llm.md) | Provider、API Key、Response Processor、测试连接 |
| [配置远程主机 SSH](docs/quickstart/configure-ssh-hosts.md) | 添加主机、凭证、对话中远程排障 |
| [开发计划](docs/development-plan.md) | 里程碑与架构（开发者向） |
| [本地开发](deploy/README.md) | 源码编译、`deploy/local/` 启停脚本 |

---

## 版本状态

**v0.1** — 首个可交付版本：Web 对话排障、LLM 配置、三档命令审批、Skill 加载、SSH 远程执行与报告导出均已可用。详见 [Releases](https://github.com/databufflabs/databuff-diag/releases)。

---

## 仓库结构

```
databuff-diag/
├── cmd/              # CLI 入口（serve / version / config）
├── internal/         # server、agent、llm、policy、skill、web UI
├── deploy/           # 本地开发脚本、内置 Skill、打包
├── docs/             # 用户文档与开发计划
└── Makefile
```

内部开发产物在 `.dev/`（任务验收记录等），不参与日常使用。

---

## 开源

本项目由 [DataBuff](https://github.com/databufflabs) 维护，欢迎 Issue 与 PR。
