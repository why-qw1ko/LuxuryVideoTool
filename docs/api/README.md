# API 文档

M1 提供健康检查与认证 API。所有 JSON 响应使用 UTF-8，错误遵循主设计文档统一结构；成功认证响应和错误响应均携带 `requestId`，HTTP Header 同时返回 `X-Request-ID`。

## 健康检查

- `GET /health/live`：仅确认进程存活。
- `GET /health/ready`：检查数据库与数据目录可写。

## 认证

- `POST /api/v1/auth/login`：用户名、密码及设备信息登录。
- `POST /api/v1/auth/cdk/login` 接收 `{code, device}`。额度用完前可使用同一密钥重复登录原账号，获得普通用户权限。网页沿用登录 Cookie 续期。密钥无效或已吊销时返回 `401 AUTH_INVALID_CDK`，次数用尽时返回 `429 CDK_LIMIT_REACHED`。
- `POST /api/v1/auth/password` 接收 `{currentPassword, newPassword}`。新密码须为 6 至 12 个字符，升级前已设置的密码继续有效。修改成功后撤销其他会话，当前会话保持登录。
- `POST /api/v1/auth/refresh`：消费一次 Refresh Token，返回轮换后的新 Token 对。网页端（`appVersion` 以 `web-` 开头）的 Refresh Token 只写入 httpOnly Cookie（`SameSite=Strict`，HTTPS/回环地址附加 `Secure`），请求体为空、凭 Cookie 续期，并校验 Origin 同源；原生客户端仍通过请求体携带 Refresh Token。
- `POST /api/v1/auth/logout`：撤销当前 Bearer Token 对应会话。
- `GET /api/v1/auth/sessions`：列出当前用户未撤销、未过期的设备会话。
- `DELETE /api/v1/auth/sessions/{id}`：撤销当前用户指定会话。
- `DELETE /api/v1/admin/sessions/{id}`：管理员撤销任意用户会话。

## 管理 API（仅管理员，普通用户一律 403）

- 后台「接口文档」通过 `GET /api/v1/admin/api-catalog` 读取接口目录，仅管理员可访问。
- 创建任务、查询结果和下载图文 ZIP 等操作提供 PowerShell 的 `curl.exe` 示例。执行前替换令牌、任务 ID 等占位符，签名下载使用由站点基址和 `downloadUrl` 组成的完整链接。

### 使用设置

- `GET /api/v1/admin/settings/usage` 返回 `{settings, requestId}`。设置包含 `dailyTaskLimit`、`allowInfo`、`allowDownload`、`allowTranscribe`。
- `PUT /api/v1/admin/settings/usage` 接收上述四个字段并立即保存。三个任务开关分别控制 `info`、`download`、`transcribe/full` 的新建请求。

### 转写服务

- `GET /api/v1/admin/settings/providers` 返回 `{providers, requestId}`。包含 `items`（`id`、`enabled`、`configured`、`available`、`model`）、`defaultProvider`、`effectiveDefaultProvider` 和 `allowUserSelection`。供应商 ID 为 `siliconflow`、`aliyun`、`deepseek`。
- `PUT /api/v1/admin/settings/providers` 支持部分更新，包括 `siliconFlowApiKey`、`aliyunApiKey`、`deepseekApiKey`、硅基流动模型 `asrModel`、`enabledProviders`、`defaultProvider`、`allowUserSelection`。默认服务须已启用，空字符串表示自动选择已就绪的服务。指定的默认服务不可用时，系统停止接受依赖默认服务的转写任务。允许自主选择时，用户仍可明确选择其他可用服务。密钥加密存储，仅返回状态。阿里云要求服务端配置可公开访问的 HTTPS 地址。
- `DELETE /api/v1/admin/settings/providers/{id}/key` 清除指定供应商的生效密钥，同时屏蔽同一供应商的环境变量密钥。保存新密钥后再次启用该凭据。
- `POST /api/v1/admin/settings/providers/{id}/test` 使用该供应商当前保存的配置处理约一秒的合成 WAV，返回 `{provider, durationMs, message, requestId}`。服务商可能计入转写用量。失败区分缺少配置、鉴权失败、限流、额度不足及超时。

### 仪表盘统计

- `GET /api/v1/admin/stats` 返回用户总数、启用账号数、任务总数、今日任务数、各状态任务数（`byStatus`）以及近 14 天每日任务数（`byDay`）。今日和每日趋势均按北京时间划分。

### 用户管理

- `GET /api/v1/admin/users`：列出常规账号，排除已删除账号和 CDK 账号。`dailyTaskLimitOverride` 为 `null` 时继承全局额度。
- `PUT /api/v1/admin/users/{id}/daily-task-limit` 接收 `{dailyTaskLimit}`。`null` 继承默认值，`0` 不限，正整数设置该用户独立额度。
- `POST /api/v1/admin/users`：创建用户，请求体 `{username, displayName, password, role}`。密码须为 6 至 12 个字符，用户名重复返回 409。
- `DELETE /api/v1/admin/users/{id}` 删除常规账号并撤销其会话，保留用户名、历史任务、文件和审计。成功返回 `{requestId}`，已删除或不存在的账号返回 `404 USER_NOT_FOUND`。当前登录的管理员和最后一个可用管理员不能删除。
- `POST /api/v1/admin/users/{id}/password`：重置密码（6 至 12 个字符），并立即撤销该用户全部会话。
- `PATCH /api/v1/admin/users/{id}/active` 启用或禁用账号，请求体为 `{active: bool}`。禁用会撤销该用户全部会话。当前登录账号和最后一个可用管理员不能禁用。
- `GET /api/v1/admin/users/{id}/sessions`：查看任意用户的活跃设备会话。
- `DELETE /api/v1/admin/sessions/{id}`：管理员下线任意用户的会话。

### CDK 管理

- `GET /api/v1/admin/cdks?limit=25&offset=0` 返回 `{items, total, requestId}`，每条记录包含 `id`、`prefix`、`taskLimit`、`taskCount`、`status` 和时间字段。完整密钥只随生成和轮换响应返回。CDK 与后台任务列表均支持 `limit` 取 1 至 100（默认 50），`offset` 为从 0 开始的记录偏移（默认 0）。分页参数非法时返回 `400 INVALID_REQUEST`。
- `POST /api/v1/admin/cdks` 接收 `{count, taskLimit}`。一次生成 1 至 100 个密钥，每个密钥可分配 1 至 100000 次累计新建任务额度。
- `PATCH /api/v1/admin/cdks/{id}` 接收 `{taskLimit}`，修改该密钥的任务总额度，已用次数保留。
- `POST /api/v1/admin/cdks/{id}/reset` 将累计已用次数清零，保留总额度、每日用量和任务历史，返回 `{item, requestId}`。
- `POST /api/v1/admin/cdks/{id}/rotate` 返回 `{item, requestId}`，`item.code` 含新密钥。旧密钥及其会话立即失效，新密钥沿用原账号、剩余额度和任务历史。已吊销的 CDK 禁止重置次数及轮换，返回 `409 CDK_REVOKED`。
- `DELETE /api/v1/admin/cdks/{id}` 吊销密钥及其登录会话，保留密钥记录和任务历史。
- `DELETE /api/v1/admin/cdks/{id}/record` 删除已吊销的密钥记录，保留关联账号、任务历史和审计记录。密钥未吊销时返回 `409 CDK_NOT_REVOKED`。
- CDK 会话的 `GET /api/v1/jobs/usage` 额外返回 `usage.cdk`，含 `used`、`limit`。累计额度用完时新建任务返回 `429 CDK_LIMIT_REACHED`，当前会话仍可查看任务历史。每日额度继续适用，幂等重放、任务重试和删除均不改变累计用量。

### 任务管理（跨用户）

- `GET /api/v1/admin/users?includeDeletedJobOwners=1`：附带 `deletedJobOwners`，包含仍有任务的已删除账号的 ID、用户名和显示名。可用账号 ID 按 `userId` 筛选历史任务。
- `GET /api/v1/admin/jobs` 支持使用 `q/status/action/userId` 筛选，使用 `limit/offset` 分页。返回 `{jobs, total, limit, offset, requestId}`，其中 `total` 是符合筛选条件的任务总数。每条任务含 `ownerUsername`、`ownerDisplayName`。指定 `view=summary` 时仅返回列表所需字段。
- `GET /api/v1/admin/jobs/{id}`：管理员视角查看任意任务详情（含媒体文件）。
- `POST /api/v1/admin/jobs/{id}/images/archive-download`：为任意用户已完成的图文任务获取配图 ZIP 短期下载地址。返回 `{downloadUrl, expiresAt}`，有效期 5 分钟。
- `POST /api/v1/admin/jobs/{id}/cancel`、`POST /api/v1/admin/jobs/{id}/retry`、`DELETE /api/v1/admin/jobs/{id}`：管理员取消、重试、删除任意用户的任务。
- `GET /api/v1/admin/files/{id}`：管理员下载任意用户的媒体文件。

以上管理操作均写入审计日志（`admin.user.*`、`admin.job.*`、`admin.file.*`）。

## M2 作品信息任务

- `POST /api/v1/jobs`：Bearer 鉴权，必须携带 `Idempotency-Key`；当前接受 `info`、`download`、`transcribe` 与 `full`。
- `GET /api/v1/jobs/{id}`：只返回当前用户拥有的任务及结构化作品信息。
- `GET /api/v1/jobs`：当前用户的历史任务，支持 `q/status/action/limit/offset` 搜索、筛选与分页。
- 两个任务列表接口均支持 `view=summary`。摘要只包含列表展示字段，文案、文件和签名通过任务详情接口读取。现有客户端省略 `view` 时继续获取完整列表。
- `GET /api/v1/jobs/usage` 返回当前账号的每日用量 `usage.{used, limit, resetAt}` 和新建权限 `usage.{allowInfo, allowDownload, allowTranscribe}`。`limit` 为 `0` 时代表不限。每账号在北京时间零点重置，每个新建任务计 1 次，重试、幂等重放和删除均不会增加或退回次数。
- `usage.asr` 包含生效的 `defaultProvider`、`allowUserSelection` 和已启用且已配置的服务 `providers`（每项含 `id`）。创建 `transcribe/full` 任务时可通过 `options.asrProvider` 明确选择供应商。省略时使用默认服务。所选服务不可用或管理员关闭自主选择时返回 `403 ASR_PROVIDER_UNAVAILABLE`。
- 创建任务达到额度时返回 `429 DAILY_TASK_LIMIT` 和 `Retry-After`。任务类型暂停新建时返回 `403 TASK_ACTION_DISABLED`。
- `DELETE /api/v1/jobs/{id}`：删除已完成、失败或取消的任务及其文件记录；进行中的任务须先取消。
- `POST /api/v1/jobs/{id}/cancel`：立即取消排队任务；本进程处理中任务协作终止下载/解析并持久化取消状态。
- `POST /api/v1/jobs/{id}/retry`：将失败或已取消的任务重新入队。
- `GET /api/v1/files/{id}`：鉴权、归属校验后流式返回文件，支持 Range。
- `GET /api/v1/jobs/{id}/images/archive`：鉴权后将该任务仍有效的配图/动图打包为 ZIP 返回，兼容原有客户端（无此类文件返回 404）。
- `POST /api/v1/jobs/{id}/images/archive-download`：Bearer 鉴权并校验任务归属、完成状态和源文件，返回 `{downloadUrl, expiresAt}`；同源下载地址有效期 5 分钟，只授权指定用户的指定任务下载 ZIP。
- `GET /api/v1/archive-download/{id}?user=&expires=&signature=`：凭上述短期签名下载，无需 Authorization 头；重新校验归属、任务状态和媒体有效期。与预览、ASR 的签名用途隔离，响应禁止缓存；完整 ZIP 成包后才返回，提供 Content-Length，支持 HEAD、Range。网页通过原生下载交给浏览器显示进度，不再先把整个 ZIP 接收到 JS 内存。
- `GET /api/v1/media-preview/{id}?expires=&signature=`：同源签名的媒体预览地址（HMAC 签名，约 24 小时有效），供 `<img>`/`<video>` 直接加载，无需 Authorization 头；仅限 image/animated/video 类型。
- `GET /api/v1/media-preview/{id}/thumbnail?expires=&signature=`：同源签名缩略图。已下载的视频优先使用服务端抽帧生成的 WebP。
- `GET /api/v1/job-cover/{id}?expires=&signature=`：同源签名的视频封面地址，由任务详情的 `work.coverPreviewUrl` 提供。服务端从允许的媒体域名获取封面图片，可用于尚未下载视频的解析结果。

创建示例：

```json
{
  "shareText": "复制打开抖音 https://v.douyin.com/xxx/",
  "action": "info",
  "options": {"force": false}
}
```

作品保留首选 `coverUrl`、`videoUrl`、`musicUrl`、图片的 `url` / `animatedUrl`，并附带按来源顺序去重的候选地址。Worker 在同一种媒体的候选地址间依次切换，最多尝试 8 个；取消或文件超出大小限制时停止，旧记录没有候选列表也能继续下载。候选字段存于原有 `metadata_json`，解析缓存版本为 `8`。

图集任务结果包含 `mediaDownload`：`expected`、`downloaded`、`skipped`、`skippedIndices`（从 1 开始）和 `musicDownloaded`。背景音乐不计入配图成功数；部分媒体失败时，任务完成提示与网页会显示实际保存数量，压缩包包含已保存的媒体。

解析结果缓存默认 6 小时，可用 `RESOLVER_CACHE_TTL` 调整。服务端仅访问 HTTPS 抖音白名单域名，重定向逐跳校验且最多 5 跳，DNS 返回任何非公网地址时拒绝请求。

单轮抖音作品解析最多等待 25 秒。作品不可访问或解析路径耗尽时，任务直接失败，仍可手动重试。解析超时的任务在详情中返回 `DOUYIN_RESOLVE_TIMEOUT`。

`download` 请求立即返回 `queued`；后台 Worker 默认单并发，持久化 Lease 为 60 秒、心跳为 15 秒，服务重启后重新领取过期任务。媒体临时写入后原子改名，登记 SHA-256、MIME 和字节数；视频与图文/动图配图默认保留 168 小时（`RETENTION_VIDEO_HOURS`），由后台每小时自动清理，删除任务会同时删除其媒体。

`transcribe/full` 依次执行解析、下载、音频提取、分段、ASR 与结果生成。硅基流动和阿里云使用 MP3。DeepSeek 使用单声道 PCM WAV，分段适配 120 秒和 8 MiB 上限。管理员可在网页选择默认服务。自动选择模式下，优先使用硅基流动，阿里云已启用且可用时，硅基流动出现可重试服务错误、超时或限流可切换至阿里云。明确指定服务商的任务使用对应服务。

**图文/动图（note）作品**：无需 ASR。`download`、`transcribe` 和 `full` 会下载配图（动图下载动态版 MP4），并以作品配文（desc）作为文案生成结果；网页端提供逐图下载与打包 ZIP。纯解析（`info`）首次请求同步返回任务结果，手动重试进入队列，全程只读取作品信息。

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
