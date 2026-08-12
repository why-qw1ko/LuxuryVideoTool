# API 文档

M1 提供健康检查与认证 API。所有 JSON 响应使用 UTF-8，错误遵循主设计文档统一结构；成功认证响应和错误响应均携带 `requestId`，HTTP Header 同时返回 `X-Request-ID`。

## 健康检查

- `GET /health/live`：仅确认进程存活。
- `GET /health/ready`：检查数据库与数据目录可写。

## 认证

- `POST /api/v1/auth/login`：用户名、密码及设备信息登录。
- `POST /api/v1/auth/refresh`：消费一次 Refresh Token，返回轮换后的新 Token 对。
- `POST /api/v1/auth/logout`：撤销当前 Bearer Token 对应会话。
- `GET /api/v1/auth/sessions`：列出当前用户未撤销、未过期的设备会话。
- `DELETE /api/v1/auth/sessions/{id}`：撤销当前用户指定会话。
- `DELETE /api/v1/admin/sessions/{id}`：管理员撤销任意用户会话。

Access Token 默认 15 分钟有效；Refresh Token 默认 30 天有效且数据库只保存 SHA-256 哈希。登录按“来源 IP + 规范化用户名”限制为默认每分钟 5 次。

存活探针响应示例：

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
