# ADR-002：Go REST API 服务端

- 状态：accepted
- 日期：2026-08-11

## 决策

服务端使用 Go 构建 REST API，并以单二进制部署。

## 影响

HTTP Handler、Use Case、Repository、Domain 与外部 Adapter 分层，Handler 不直接访问 SQL、FFmpeg 或第三方 HTTP。

