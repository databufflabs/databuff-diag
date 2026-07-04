# Deploy

部署、打包与本地开发相关资源。

| 目录 | 用途 |
|------|------|
| [`local/`](local/) | 本地开发：源码编译、启停 `databuff-diag` HTTP 服务 |
| [`skills/`](skills/) | 内置 Skill 包（`generic-infra`、`databuff-oss` 等） |
| [`scripts/`](scripts/) | 打包脚本（如 `pack-skills.sh`） |
| [`dist/`](dist/) | 本地编译产出（二进制、打包物等，已 gitignore） |

## 本地开发

```bash
# 重装：停止旧进程 → 重新编译 → 启动（默认 :8787）
./deploy/local/install.sh

# 仅启动（默认会编译；已有产物时可 SKIP_BUILD=1）
./deploy/local/start.sh

# 停止
./deploy/local/stop.sh
```

可选环境变量：

| 变量 | 默认 | 说明 |
|------|------|------|
| `LISTEN` | `:8787` | HTTP 监听地址 |
| `SKIP_BUILD` | `0` | `1` 时跳过 `make build` |
| `START_SKIP_READY` | `0` | `1` 时不等待 `/health` |

运行时文件：`deploy/local/run/databuff-diag.pid`、`deploy/local/run/databuff-diag.log`。

编译产物：`deploy/dist/databuff-diag`（`make build` 或 `deploy/local/install.sh` 写入）。

打包 Skill：`./deploy/scripts/pack-skills.sh` → `deploy/dist/databuff-oss-skill.tar.gz`。
