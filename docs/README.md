# databuff-diag 文档

**DataBuff 客户环境排查助手** 的用户与开发者文档索引。日常部署与排障请从「快速入门」开始；需要查阅 `config.yaml` 全量字段时见「配置参考」。

---

## 快速入门

按顺序阅读即可完成安装、模型与远程主机配置：

| 文档 | 说明 |
|------|------|
| [安装与启动](quickstart/install.md) | Release 下载、本地开发模式、`databuff-diag serve` |
| [配置大模型](quickstart/configure-llm.md) | Web / YAML / API 三种方式；Provider、API Key、Response Processor、`POST /api/llm/test` |
| [配置远程主机 SSH](quickstart/configure-ssh-hosts.md) | `ssh.hosts`、Web 设置、对话中 `@host_id` 远程排障 |

---

## 配置参考

| 文档 | 说明 |
|------|------|
| [configuration.md](configuration.md) | `~/.databuff-diag/config.yaml` 全量参考：`llm`、`ssh`、`skills`、`policy`、`auth`、`sessions`；服务监听与 CLI 参数 |

配置文件路径：`databuff-diag config path`（默认 `~/.databuff-diag/config.yaml`）。

---

## 开发与规划

| 文档 | 说明 |
|------|------|
| [development-plan.md](development-plan.md) | 里程碑、架构、Response Processor 与分阶段任务（开发者向） |
| [deploy/README.md](../deploy/README.md) | 源码编译、`deploy/local/` 启停脚本、内置 Skill 目录 |

---

## 仓库入口

- 项目概览与功能表：[README.md](../README.md)
- 版本与变更：[CHANGELOG.md](../CHANGELOG.md) · [GitHub Releases](https://github.com/databufflabs/databuff-diag/releases)

---

## 推荐阅读路径

```text
新用户：install → configure-llm → configure-ssh-hosts → 开始对话排障
运维：  configuration（全量 YAML）+ install（防火墙与内网部署）
开发：  development-plan → deploy/README → configuration
```
