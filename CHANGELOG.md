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

- 启动成功时输出可点击的完整访问地址
- 正常运行时不再打印 HTTP 访问日志，仅 5xx 错误写入日志

### 安装包

| 平台 | 文件 |
|------|------|
| macOS Apple Silicon | `databuff-diag_0.1.0_darwin_arm64.tar.gz` |
| macOS Intel | `databuff-diag_0.1.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `databuff-diag_0.1.0_linux_amd64.tar.gz` |
| Linux ARM64 | `databuff-diag_0.1.0_linux_arm64.tar.gz` |
| Windows x86_64 | `databuff-diag_0.1.0_windows_amd64.zip` |

[0.1.0]: https://github.com/databufflabs/databuff-diag/releases/tag/v0.1.0
