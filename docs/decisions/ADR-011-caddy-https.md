# ADR-011：Caddy 提供 HTTPS

- 状态：accepted
- 日期：2026-08-11

## 决策

生产入口使用 Caddy 提供自动证书、HTTPS、反向代理和请求限制。

## 影响

Go 服务默认只监听回环地址；数据目录不通过 Caddy 暴露。

