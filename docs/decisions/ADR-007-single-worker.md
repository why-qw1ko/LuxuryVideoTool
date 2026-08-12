# ADR-007：默认单 Worker

- 状态：accepted
- 日期：2026-08-11

## 决策

首版 `WORKER_CONCURRENCY=1`，架构允许未来通过配置提升到有限并发。

## 影响

不得无界创建 goroutine；任务通过持久队列、Lease 和心跳协调。

