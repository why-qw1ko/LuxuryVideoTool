# Douyin Capture

面向 Android、Windows 的抖音内容提取客户端，以及运行于 Linux 的 Go 服务端。项目采用 Flutter + Go + SQLite + FFmpeg + 云端 ASR，当前处于 **M0 工程基线**。

## 仓库结构

```text
apps/client_flutter/   Android + Windows Flutter 客户端
services/api_go/       Go REST API 与后台 Worker
deploy/                Caddy、systemd 与可选 Docker 部署文件
docs/                  API、运维、安全、ADR 与验收文档
scripts/               本地检查脚本
```

## 当前范围

- M0：工程空壳、版本注入、自动化检查、配置模板与文档。
- M1-M7：尚未实施，详见[主设计文档](DouyinCapture_Product_Technical_Design.md)。
- 仓库不保存 API Key、签名密钥、Token 或生产配置。

开发环境与命令见 [docs/operations/development.md](docs/operations/development.md)，M0 验收状态见 [docs/acceptance/M0.md](docs/acceptance/M0.md)。
