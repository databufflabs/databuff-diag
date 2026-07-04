# Changelog

## [Unreleased]

## [0.1.0] - 2026-07-04

首个正式版本。

### Added

- Web 对话式排障
- 大模型配置（OpenAI 兼容 API）
- SSH 远程主机连接
- 危险命令审批
- 诊断报告导出
- Windows 10/11 支持
- `serve --daemon` 后台启动（默认开启，可用 `--foreground` 前台运行）

### Changed

- 启动成功时输出可点击的完整访问地址；绑定 `0.0.0.0` 时自动显示本机局域网 IP，便于同网段访问
- 正常运行时不再打印 HTTP 访问日志，仅 5xx 错误写入日志
- 更新各 LLM 厂商默认模型至 2026 年主流版本
- 首次安装不再预选 LLM 厂商，需在设置中配置 API Key 后启用
- 发布包解压后统一进入 `databuff-diag` 子目录，避免与下载目录文件混在一起
- 会话工作区与元数据统一存放在 `sessions/<id>/` 目录（`session.json` + 工作区文件），兼容旧版扁平 `*.json` 布局

### Fixed

- Linux 发布包改为 `CGO_ENABLED=0` 静态链接，可在 CentOS 7 / RHEL 8 等旧 glibc 环境直接运行
- 修复未启用厂商仍显示「使用中」的问题
- 加载配置时自动清除已禁用厂商的 `active` 标记

### 安装包

| 平台 | 文件 |
|------|------|
| macOS Apple Silicon | `databuff-diag_0.1.0_darwin_arm64.tar.gz` |
| macOS Intel | `databuff-diag_0.1.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `databuff-diag_0.1.0_linux_amd64.tar.gz` |
| Linux ARM64 | `databuff-diag_0.1.0_linux_arm64.tar.gz` |
| Windows x86_64 | `databuff-diag_0.1.0_windows_amd64.zip` |

[0.1.0]: https://github.com/databufflabs/databuff-diag/releases/tag/v0.1.0
