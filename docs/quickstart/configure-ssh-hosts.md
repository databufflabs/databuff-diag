# 快速入门：配置远程主机 SSH

databuff-diag 通过系统 `ssh` 二进制连接客户内网主机，在 Web 对话中由 Agent 自动执行远程只读排障命令。本文说明如何配置 `ssh.hosts`、在 Web UI 管理主机，以及在对话中触发 SSH 排查。

---

## 前置条件

| 依赖 | 说明 |
|------|------|
| `ssh` | 系统已安装 OpenSSH 客户端 |
| `sshpass` | 仅在使用**密码登录**时需要（密钥登录可省略） |
| 网络 | 运行 databuff-diag 的机器能访问目标主机 SSH 端口（默认 22） |

密钥登录时，请在本机配置好 `~/.ssh/id_rsa`（或其它密钥）或 `ssh-agent`，并确保目标主机 `authorized_keys` 已授权。配置文件中**不保存私钥路径**，无密码的主机会自动走密钥认证。

---

## 配置文件位置

```
~/.databuff-diag/
├── config.yaml      # ssh.hosts 等配置（权限 0600）
└── secrets.key      # AES-256 主密钥，用于加密密码（权限 0600）
```

查看路径：`databuff-diag config path`

---

## 方式一：编辑 config.yaml

在 `~/.databuff-diag/config.yaml` 的 `ssh` 节添加主机列表。每个主机包含：

| 字段 | 必填 | 说明 |
|------|------|------|
| `id` | 推荐 | 稳定标识，对话中可用 `@host_id` 引用；省略时首次加载会自动生成 |
| `name` | 否 | 显示名称，便于在对话中用自然语言指代（如「prod-vm」） |
| `host` | 是 | IP 或主机名 |
| `port` | 否 | SSH 端口，默认 22 |
| `user` | 是 | 登录用户名 |
| `password` | 否 | SSH 密码；保存后自动加密为 `enc:v1:...`，磁盘上不以明文出现 |

### 示例：密钥登录 + 密码登录

```yaml
ssh:
  control_path: ~/.databuff-diag/ssh/%r@%h-%p
  control_persist: 10m
  hosts:
    # 密钥登录（不填 password，依赖本机 ~/.ssh 密钥或 ssh-agent）
    - id: host-a1b2c3d4e5f67890
      name: prod-app-01
      host: 10.0.1.11
      port: 22
      user: deploy

    # 密码登录（首次可写明文，保存后自动加密）
    - id: host-b2c3d4e5f6789012
      name: staging-db
      host: 192.168.10.50
      port: 2222
      user: root
      password: "your-ssh-password"
```

保存后 `password` 字段在磁盘上类似：

```yaml
      password: enc:v1:AbCdEfGh...
```

`control_path` 与 `control_persist` 用于 SSH ControlMaster 连接复用，减少多次排障时的握手开销，一般保持默认即可。

### 兼容旧格式

仍支持仅写主机地址的简写列表（加载时自动分配 `id`）：

```yaml
ssh:
  hosts:
    - 10.0.0.100
```

---

## 方式二：Web UI 添加主机

1. 启动服务：`databuff-diag serve`（默认 `http://127.0.0.1:8787`）
2. 登录 Web UI（首次默认用户名 `Admin`，密码见 `config.yaml` 的 `auth` 节）
3. 打开 **设置** → **远程主机**
4. 点击 **添加主机**，填写名称、地址、端口、用户名、密码（可选）
5. 保存后写入 `config.yaml`，密码经 vault 加密

编辑已有主机时，密码框留空表示**保留原密码**（与 API 行为一致）。

---

## 方式三：API / curl

配置读写走 `GET` / `PUT /api/config`（需先登录获取会话 Cookie）。

### 登录

```bash
curl -s -c /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"Admin","password":"Databuff@123"}'
```

### 查看当前主机（密码已脱敏）

```bash
curl -s -b /tmp/diag-cookies.txt http://127.0.0.1:8787/api/config | jq '.ssh.hosts'
```

响应中 `password` 为空，但 `password_configured: true` 表示已保存密码。

### 添加或更新主机

```bash
curl -s -b /tmp/diag-cookies.txt -X PUT http://127.0.0.1:8787/api/config \
  -H 'Content-Type: application/json' \
  -d '{
    "llm": { "active": "deepseek", "providers": {} },
    "policy": { "default": "write_approval" },
    "ssh": {
      "control_path": "~/.databuff-diag/ssh/%r@%h-%p",
      "control_persist": "10m",
      "hosts": [
        {
          "id": "host-prod-case-a",
          "name": "prod-vm",
          "host": "10.0.1.11",
          "port": 22,
          "user": "root",
          "password": "new-or-updated-password"
        }
      ]
    },
    "skills": { "dirs": [] },
    "auth": { "username": "Admin" }
  }'
```

> **提示**：PUT 为全量更新。若只改 SSH 主机，请先 `GET /api/config` 取回完整 JSON，修改 `ssh.hosts` 后再 PUT。更新已有主机且不想改密码时，省略 `password` 字段即可保留原值。

### 直接执行 SSH 只读命令（调试用）

```bash
# 按 host_id（推荐，使用已保存凭证）
curl -s -b /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/exec/ssh \
  -H 'Content-Type: application/json' \
  -d '{"host_id":"host-prod-case-a","command":"df -h"}' | jq .

# 按地址 + 用户（无保存主机时）
curl -s -b /tmp/diag-cookies.txt -X POST http://127.0.0.1:8787/api/exec/ssh \
  -H 'Content-Type: application/json' \
  -d '{"host":"10.0.1.11","user":"root","command":"uname -a"}' | jq .
```

响应含 `risk` 字段（`readonly` / `write` / `dangerous` / `blocked`），与本地命令共用同一套策略引擎。

---

## 对话中触发 SSH 排障

Agent 在系统提示中会注入已保存主机目录（含 `id`、名称、地址，**不含密码**）。你可以用以下方式指定目标：

### 用 host_id 引用

在消息中写出主机 id（可加 `@` 前缀），并说明要检查的内容：

```
请在 @host-prod-case-a 上执行 df -h 和 free -m，看磁盘和内存
```

Agent 会生成类似工具调用：

```json
{"tool":"ssh","host_id":"host-prod-case-a","command":"df -h && free -m"}
```

### 用显示名称或地址

```
请 SSH 登录 prod-vm 查看 docker 容器状态
```

```
帮我在 10.0.1.11 上跑 journalctl -u nginx --no-pager -n 50
```

系统会按 **id → name → host 地址** 匹配 `ssh.hosts` 中的记录，并自动注入已保存的密码。

### 临时在对话中提供凭证（不推荐生产）

若未预先配置主机，可在单条消息中附带账号密码（仅写入当前会话覆盖，不落盘）：

```
主机: 192.168.1.100  用户: root  密码: TempPass123
请检查该机器磁盘使用情况
```

支持中英文多种写法（`user@host/password`、`主机/用户/密码` 等）。**生产环境请优先使用 `ssh.hosts` + vault，避免在聊天中明文输入密码。**

### 常用只读排障示例

默认策略为 **写入审批**（`write_approval`）：下列只读命令通常**自动执行**，无需逐条点批准。

| 场景 | 对话示例 |
|------|----------|
| 系统资源 | 「在 @host-xxx 上执行 `df -h` 和 `free -m`」 |
| 进程与容器 | 「登录 staging 执行 `docker ps -a`」 |
| 日志 | 「在 prod-vm 上 `journalctl -u databuff --no-pager -n 100`」 |
| K8s | 「在 @host-xxx 执行 `kubectl get pods -A`」 |
| 网络监听 | 「查看 10.0.1.11 上 `ss -tlnp`」 |

以下命令会被标为 **write** 或 **dangerous**，需人工批准（或改用更保守的「全部审批」策略）：

- `systemctl restart …`、`sed -i …`、`kubectl apply/delete`、`docker compose …`
- `rm -rf /` 等危险操作会被 **blocked**，无法执行

---

## 安全说明

### 文件权限

| 路径 | 权限 | 说明 |
|------|------|------|
| `~/.databuff-diag/` | `0700` | 配置目录 |
| `config.yaml` | `0600` | 含加密后的 SSH 密码 |
| `secrets.key` | `0600` | AES-256-GCM 主密钥，丢失后已加密密码无法解密 |

### 密码与 vault

- SSH 密码与 LLM API Key 一样，保存时经 `secrets.key` 加密为 `enc:v1:...`
- 首次写入明文密码会在下次 `Load` 时自动迁移加密
- API `GET /api/config` **不回传**密码明文，仅 `password_configured`
- Agent 工具调用与审计日志**不包含**密码；会话历史中会对用户消息里的临时密码做隔离

### 密钥权限

若使用密钥登录，请在本机遵守 OpenSSH 惯例：

```bash
chmod 700 ~/.ssh
chmod 600 ~/.ssh/id_rsa          # 私钥
chmod 644 ~/.ssh/id_rsa.pub      # 公钥
chmod 600 ~/.ssh/config          # 若有
```

### 只读策略

- 默认 `policy.default: write_approval`：`ls`、`cat`、`grep`、`df`、`free`、`docker ps`、`kubectl get`、`journalctl` 等归类为 **readonly**，自动执行
- 可在新建会话时切换为 **全部审批**（最保守）或 **全部开放**（仅拦截危险命令）
- 远程与本地命令共用 `internal/policy` 分类，SSH 不会绕过审批闸门

### 运维建议

1. 生产环境优先 **密钥登录**，密码仅作备用
2. 为每台机器设置有意义的 `name`，便于对话指代
3. 固定 `id` 后再写入自动化脚本或 runbook，避免 YAML 重排后 id 变化（手动指定 `id` 即可）
4. 定期备份 `~/.databuff-diag/`，`secrets.key` 与 `config.yaml` 需一并保管

---

## 故障排查

| 现象 | 处理 |
|------|------|
| `sshpass is required for password authentication` | 安装 `sshpass`（macOS: `brew install sshpass`） |
| 连接超时 | 检查防火墙、安全组、`host`/`port` 是否正确 |
| `Permission denied (publickey)` | 未配置密码且本机密钥未授权；添加 `password` 或配置密钥 |
| 对话未连到预期主机 | 在设置页核对 `id`/`name`，或消息中显式写 `@host_id` |
| 命令一直待批准 | 命令被标为 write/dangerous；确认是否必要，或调整会话审批档位 |

---

## 相关文档

- [安装与启动](install.md)
- [配置 LLM](configure-llm.md)
- 项目 README：[SSH 与 ControlMaster 概述](../../README.md)
