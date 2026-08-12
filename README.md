# Douyin Capture

面向 Android、Windows 的抖音内容提取客户端，以及运行于 Linux 的 Go 服务端。项目采用 Flutter + Go + SQLite + FFmpeg + 云端 ASR，当前进入 **M3 任务队列与媒体**。

## 仓库结构

```text
apps/client_flutter/   Android + Windows Flutter 客户端
services/api_go/       Go REST API 与后台 Worker
deploy/                Caddy、systemd 与可选 Docker 部署文件
docs/                  API、运维、安全、ADR 与验收文档
scripts/               本地检查脚本
```

## 当前范围

- M0-M1：工程基线、SQLite、认证与设备会话。
- M2：链接规范化、SSRF 防护、视频/图文解析、缓存与 `info` 任务 API。
- M3：持久化队列、Lease/恢复、媒体下载、FFmpeg Adapter、文件登记与清理、`download` 任务 API。
- M4-M7：尚未实施，详见[主设计文档](DouyinCapture_Product_Technical_Design.md)。
- 仓库不保存 API Key、签名密钥、Token 或生产配置。

开发环境与命令见 [docs/operations/development.md](docs/operations/development.md)，阶段验收状态见 [docs/acceptance](docs/acceptance)。
