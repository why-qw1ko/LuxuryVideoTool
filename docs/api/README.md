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
- `GET /api/v1/admin/settings/providers`：管理员查看阿里云/硅基流动 API Key 状态及阿里云当前是否具备公网调用条件；不返回密钥原文。
- `PUT /api/v1/admin/settings/providers`：管理员保存或清除供应商 API Key；服务端使用 JWT 主密钥派生密钥加密存储，并立即用于新任务。

## M2 作品信息任务

- `POST /api/v1/jobs`：Bearer 鉴权，必须携带 `Idempotency-Key`；当前接受 `info`、`download`、`transcribe` 与 `full`。
- `GET /api/v1/jobs/{id}`：只返回当前用户拥有的任务及结构化作品信息。
- `GET /api/v1/jobs`：当前用户的历史任务，支持 `q/status/action/limit/offset` 搜索、筛选与分页。
- `DELETE /api/v1/jobs/{id}`：删除已完成、失败或取消的任务及其文件记录；进行中的任务须先取消。
- `POST /api/v1/jobs/{id}/cancel`：立即取消排队任务；本进程处理中任务协作终止下载/解析并持久化取消状态。
- `POST /api/v1/jobs/{id}/retry`：将失败或已取消的任务重新入队。
- `GET /api/v1/files/{id}`：鉴权、归属校验后流式返回文件，支持 Range。

创建示例：

```json
{
  "shareText": "复制打开抖音 https://v.douyin.com/xxx/",
  "action": "info",
  "options": {"force": false}
}
```

解析结果缓存默认 6 小时，可用 `RESOLVER_CACHE_TTL` 调整。服务端仅访问 HTTPS 抖音白名单域名，重定向逐跳校验且最多 5 跳，DNS 返回任何非公网地址时拒绝请求。

`download` 请求立即返回 `queued`；后台 Worker 默认单并发，持久化 Lease 为 60 秒、心跳为 15 秒，服务重启后重新领取过期任务。媒体临时写入后原子改名，登记 SHA-256、MIME 和字节数；视频默认保留 168 小时。

`transcribe/full` 依次执行解析、下载、MP3 提取、9 分钟分段（重叠 1 秒）、ASR 与结果生成。默认使用 `siliconflow_sensevoice/FunAudioLLM/SenseVoiceSmall`，由服务端直接上传本地音频，不需要公网地址。配置阿里云 Key 和 `PUBLIC_BASE_URL` 后，硅基流动出现可重试服务错误、超时或限流时可切换至阿里 Paraformer；认证、输入和预算错误不会切换。

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
