# Changelog

## [Unreleased]

## [0.1.1] - 2026-07-06

### Fixed

- 修复 DataBuff Ultra 网关测试连接失败：`databuff_ultra_result` 解析器下 Base URL 与 Ultra `OpenAIService` 一致，原样 POST，不再错误追加 `/chat/completions`
- 设置页选择 Ultra 解析器时提示填写完整 API 地址（如 `/apis/ais/qwen2-72b`）

## [0.1.0] - 2026-07-05

首个正式版本。

### Added

- Web 对话式排障
- 大模型配置（OpenAI 兼容 API）
- SSH 远程主机连接
- 危险命令审批
- 诊断报告导出
- Windows 10/11 支持
- `serve --daemon` 后台启动（默认开启，可用 `--foreground` 前台运行）
- 支持 LLM 原生 function calling（read/write/edit/bash/ssh），与 tool JSON 双轨兼容
- 对话消息展示工具调用摘要，避免空白 assistant 气泡
- 不完整回复自动 nudge，减少「接下来将…」类半截回答
- 本地开发：`PI_VERIFY_ROUNDS` pi 项目调研对话验证脚本
- 本地开发：`INTEGRATION_VERIFY` 集成验证（Docker / 文件读写）

### Changed

- 启动成功时输出可点击的完整访问地址；绑定 `0.0.0.0` 时自动显示本机局域网 IP，便于同网段访问
- 正常运行时不再打印 HTTP 访问日志，仅 5xx 错误写入日志
- 更新各 LLM 厂商默认模型至 2026 年主流版本
- 首次安装不再预选 LLM 厂商，需在设置中配置 API Key 后启用
- 发布包解压后统一进入 `databuff-diag` 子目录，避免与下载目录文件混在一起
- 会话工作区与元数据统一存放在 `sessions/<id>/` 目录（`session.json` + 工作区文件），兼容旧版扁平 `*.json` 布局
- 策略引擎：`[`、`test` 等 shell 测试命令归类为只读
- 目录树等 fenced 内容不再误识别为待执行命令
- 启动成功提示补充默认用户名与密码

### Fixed

- Linux 发布包改为 `CGO_ENABLED=0` 静态链接，可在 CentOS 7 / RHEL 8 等旧 glibc 环境直接运行
- 修复未启用厂商仍显示「使用中」的问题
- 加载配置时自动清除已禁用厂商的 `active` 标记
- 修复对话流中工具调用与消息展示不一致的问题
- 修复只读复合命令（`cd && if [ -f … ]; then head …`）被误判为 write 的风险等级
- 修复调研类长总结被误判为「未完成回复」而反复 nudge 的问题
- 修复对话缺失的 bug

### 安装包

| 平台 | 文件 |
|------|------|
| macOS Apple Silicon | `databuff-diag_0.1.0_darwin_arm64.tar.gz` |
| macOS Intel | `databuff-diag_0.1.0_darwin_amd64.tar.gz` |
| Linux x86_64 | `databuff-diag_0.1.0_linux_amd64.tar.gz` |
| Linux ARM64 | `databuff-diag_0.1.0_linux_arm64.tar.gz` |
| Windows x86_64 | `databuff-diag_0.1.0_windows_amd64.zip` |

[0.1.0]: https://github.com/databufflabs/databuff-diag/releases/tag/v0.1.0
