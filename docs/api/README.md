# API 文档

M0 仅提供不带业务依赖的 `GET /health/live` 存活探针。完整 `/api/v1` OpenAPI 契约从 M1 起随功能同步维护。

响应示例：

```json
{
  "status": "ok",
  "version": {
    "version": "0.1.0-dev",
    "commit": "unknown",
    "buildTime": "unknown"
  }
}
```

