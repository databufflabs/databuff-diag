# 安装与启动

本文说明如何在客户内网或本机安装 **databuff-diag**，并启动 Web 排障服务。支持 **linux / macOS（darwin）** 与 **amd64 / arm64** 四种预编译包；也可在源码仓库内用 `deploy/local/` 进行开发模式启停。

---

## 前置要求

| 场景 | 要求 |
|------|------|
| Release 下载 | 可访问 GitHub（`github.com`）；离线环境请在可联网机器下载后拷贝入内网 |
| 启动服务 | 无需 root；需能绑定本地端口（默认 `8787`） |
| 本地开发编译 | Go 1.22+、`make`、`curl` |

---

## 方式一：下载 Release 包（推荐）

从 [Releases](https://github.com/databufflabs/databuff-diag/releases) 下载与当前平台匹配的 tar.gz，解压后安装到 `PATH`。

**版本示例**：`VERSION=v0.1.0`（Release 标签为 `v0.1.0`）

资产命名规则：

```text
databuff-diag_{版本号}_{os}_{arch}.tar.gz
```

其中 `{版本号}` 为去掉 `v` 前缀的标签，例如 `v0.1.0` → `0.1.0`。

### v0.1.0 四平台下载地址

| 平台 | 文件名 | 下载 URL |
|------|--------|----------|
| Linux amd64 | `databuff-diag_0.1.0_linux_amd64.tar.gz` | https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_linux_amd64.tar.gz |
| Linux arm64 | `databuff-diag_0.1.0_linux_arm64.tar.gz` | https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_linux_arm64.tar.gz |
| macOS amd64 | `databuff-diag_0.1.0_darwin_amd64.tar.gz` | https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_darwin_amd64.tar.gz |
| macOS arm64 | `databuff-diag_0.1.0_darwin_arm64.tar.gz` | https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_darwin_arm64.tar.gz |

### 解压与安装

以 Linux amd64 为例：

```bash
curl -fsSL -O https://github.com/databufflabs/databuff-diag/releases/download/v0.1.0/databuff-diag_0.1.0_linux_amd64.tar.gz
tar -xzf databuff-diag_0.1.0_linux_amd64.tar.gz
install -m 755 databuff-diag /usr/local/bin/databuff-diag
# 或：mv databuff-diag ~/.local/bin/
databuff-diag version
```

压缩包内包含 `databuff-diag` 二进制及 `LICENSE`、`README.md`。

---

## 方式二：源码仓库本地开发

在克隆的仓库根目录下，使用 `deploy/local/` 脚本编译并后台启动服务，适合联调与二次开发。

```bash
# 重装：停止旧进程 → make build → 启动（默认 :8787）
./deploy/local/install.sh

# 仅启动（默认会编译；已有产物时可跳过编译）
./deploy/local/start.sh

# 停止
./deploy/local/stop.sh
```

常用环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `LISTEN` | `:8787` | HTTP 监听地址 |
| `SKIP_BUILD` | `0` | 设为 `1` 时跳过 `make build` |
| `START_SKIP_READY` | `0` | 设为 `1` 时不等待 `/health` 就绪 |

编译产物：`deploy/dist/databuff-diag`。运行时 PID 与日志：`deploy/local/run/databuff-diag.pid`、`deploy/local/run/databuff-diag.log`。

也可手动编译：

```bash
make build
./deploy/dist/databuff-diag version
```

更多细节见 [deploy/README.md](../../deploy/README.md)。

---

## 启动服务

安装或编译完成后，启动 HTTP 服务：

```bash
databuff-diag serve
```

默认监听 **`:8787`**（所有网卡）。启动时会打印：

```text
databuff-diag listening on :8787
```

### 自定义端口

```bash
databuff-diag serve --listen :9000
```

### 浏览器访问

```bash
# macOS
open http://127.0.0.1:8787

# Linux / 通用
# 浏览器打开 http://127.0.0.1:8787
```

若需从其他机器访问，将 `127.0.0.1` 换为服务器内网 IP，并确保防火墙放行对应端口（见下文）。

### 健康检查

```bash
curl http://127.0.0.1:8787/health
```

正常响应：

```json
{"status":"ok"}
```

### 首次启动与配置目录

首次运行会在用户主目录创建 **`~/.databuff-diag/`**：

```text
~/.databuff-diag/
├── config.yaml    # LLM、SSH、策略、Skill 路径
├── skills/        # 用户安装的 Skill 包
├── sessions/      # 对话会话
└── reports/       # 导出报告
```

查看配置目录绝对路径：

```bash
databuff-diag config path
```

下一步：[配置大模型](configure-llm.md)。

---

## 防火墙与内网部署提示

databuff-diag 设计为在**客户内网**单机或跳板机上运行，不依赖公网中控。部署时请注意：

1. **出站网络**  
   - 安装需能访问 GitHub 下载 Release；若完全离线，请在可联网环境下载 tar.gz 后拷贝入内网。  
   - 配置大模型后，服务需能访问您指定的 LLM API 端点（内网网关或公网，视客户策略而定）。

2. **入站端口**  
   - 默认 `8787`。仅本机使用时绑定 `127.0.0.1:8787` 更安全：  
     `databuff-diag serve --listen 127.0.0.1:8787`  
   - 需团队共享访问时，使用 `--listen :8787` 或 `0.0.0.0:8787`，并在主机防火墙 / 安全组中放行 TCP `8787`（或您自定义的端口）。

3. **SSH 远程排障**  
   - Agent 从运行 databuff-diag 的机器发起 SSH 到目标主机；请确保该机器到目标机的 `22`（或自定义 SSH 端口）可达，而非要求目标机访问 `8787`。

4. **敏感数据**  
   - API Key、SSH 密码等保存在 `~/.databuff-diag/config.yaml`（权限 `0600`）。请勿将配置目录提交到版本库或外传。

---

## 常见问题

| 现象 | 处理 |
|------|------|
| `command not found: databuff-diag` | 确认安装目录在 `PATH` 中，或使用绝对路径运行 |
| 端口被占用 | 换用 `--listen :其他端口`，或停止占用 `8787` 的进程 |
| 本地 `start.sh` 报 unhealthy | 查看 `deploy/local/run/databuff-diag.log` |

---

## 相关文档

- [配置大模型](configure-llm.md)
- [配置远程主机 SSH](configure-ssh-hosts.md)
- [README 快速开始](../../README.md#快速开始)
