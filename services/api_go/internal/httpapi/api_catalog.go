package httpapi

import "net/http"

type apiOperation struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Group       string `json:"group"`
	Summary     string `json:"summary"`
	Auth        string `json:"auth"`
	Details     string `json:"details,omitempty"`
	CurlExample string `json:"curlExample,omitempty"`
}

const (
	apiAuthPublic  = "public"
	apiAuthBearer  = "bearer"
	apiAuthAdmin   = "admin"
	apiAuthRefresh = "refresh"
	apiAuthSigned  = "signed"

	curlCreateJob   = "create-job"
	curlRequest     = "request"
	curlDownloadZIP = "download-zip"
	curlSignedZIP   = "signed-zip"

	groupReleases   = "安卓发版"
	groupHealth     = "健康检查"
	groupAuth       = "登录与会话"
	groupJobs       = "用户任务"
	groupSigned     = "签名媒体"
	groupOverview   = "后台概览"
	groupAdminUsers = "用户管理"
	groupAdminCDKs  = "CDK 管理"
	groupAdminJobs  = "任务管理"
	groupUsage      = "使用设置"
	groupProviders  = "转写服务"
)

var apiCatalog = []apiOperation{
	{Method: http.MethodGet, Path: "/api/v1/app/android/version", Group: groupReleases, Auth: apiAuthPublic,
		Summary: "检查安卓最新版本", Details: "公开读取已发布版本及强制更新开关，不包含草稿。"},
	{Method: http.MethodGet, Path: "/api/v1/app/android/releases/{id}/apk", Group: groupReleases, Auth: apiAuthPublic,
		Summary: "下载已发布安卓安装包", Details: "公开下载曾发布的不可变 APK，支持 Range 和 HEAD。"},
	{Method: http.MethodGet, Path: "/api/v1/admin/app/android/release", Group: groupReleases, Auth: apiAuthAdmin,
		Summary: "读取安卓发布草稿和当前版本"},
	{Method: http.MethodPost, Path: "/api/v1/admin/app/android/apk", Group: groupReleases, Auth: apiAuthAdmin,
		Summary: "上传安卓安装包", Details: "multipart 字段 apk，最大 256 MiB；从 APK 读取包名和版本，上传仅创建草稿。"},
	{Method: http.MethodPut, Path: "/api/v1/admin/app/android/release", Group: groupReleases, Auth: apiAuthAdmin,
		Summary: "编辑安卓发布草稿", Details: "提交 draftId、原 APK 的 versionCode/versionName、changelog 和 forceUpdate。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/app/android/publish", Group: groupReleases, Auth: apiAuthAdmin,
		Summary: "发布安卓新版本", Details: "提交 draftId；版本编号必须递增，同一 APK 可以重新发布说明。"},
	{Method: http.MethodPut, Path: "/api/v1/admin/app/android/policy", Group: groupReleases, Auth: apiAuthAdmin,
		Summary: "切换安卓强制更新", Details: "提交 releaseId 和 forceUpdate，立即修改当前发布策略。"},

	{Method: http.MethodGet, Path: "/health/live", Group: groupHealth, Auth: apiAuthPublic,
		Summary: "检查进程存活", Details: "返回当前服务版本。"},
	{Method: http.MethodGet, Path: "/health/ready", Group: groupHealth, Auth: apiAuthPublic,
		Summary: "检查服务是否就绪", Details: "检查数据库和数据目录。"},

	{Method: http.MethodPost, Path: "/api/v1/auth/login", Group: groupAuth, Auth: apiAuthPublic,
		Summary: "使用账号密码登录", Details: "请求体包含 username、password 和 device。"},
	{Method: http.MethodPost, Path: "/api/v1/auth/cdk/login", Group: groupAuth, Auth: apiAuthPublic,
		Summary: "使用 CDK 密钥登录", Details: "请求体包含 code 和 device。网页请求校验同源。"},
	{Method: http.MethodPost, Path: "/api/v1/auth/refresh", Group: groupAuth, Auth: apiAuthRefresh,
		Summary: "续期登录会话", Details: "网页使用同源请求和刷新 Cookie，原生客户端在请求体中提供 refreshToken。"},
	{Method: http.MethodPost, Path: "/api/v1/auth/logout", Group: groupAuth, Auth: apiAuthBearer,
		Summary: "退出当前会话"},
	{Method: http.MethodGet, Path: "/api/v1/auth/sessions", Group: groupAuth, Auth: apiAuthBearer,
		Summary: "查看当前账号的活跃会话"},
	{Method: http.MethodDelete, Path: "/api/v1/auth/sessions/{id}", Group: groupAuth, Auth: apiAuthBearer,
		Summary: "下线当前账号的指定会话", Details: "id 为会话 ID。"},
	{Method: http.MethodPost, Path: "/api/v1/auth/password", Group: groupAuth, Auth: apiAuthBearer,
		Summary: "修改当前账号密码", Details: "请求体包含 currentPassword 和 newPassword。成功后下线其他会话。"},

	{Method: http.MethodPost, Path: "/api/v1/jobs", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "创建任务", Details: "请求头须带 Idempotency-Key。每个新的任务请求使用不同的键。请求体包含 shareText 和 action，可传 options。",
		CurlExample: curlCreateJob},
	{Method: http.MethodGet, Path: "/api/v1/jobs", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "查看自己的任务列表", Details: "支持 q、status、action、limit、offset 和 view=summary。"},
	{Method: http.MethodGet, Path: "/api/v1/jobs/usage", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "查看任务配额和创建权限", Details: "返回每日配额和可用任务类型。CDK 会话还返回累计额度。"},
	{Method: http.MethodGet, Path: "/api/v1/jobs/{id}", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "查看自己的任务详情", Details: "id 为创建任务时返回的 job.id。", CurlExample: curlRequest},
	{Method: http.MethodPost, Path: "/api/v1/jobs/{id}/music/refresh", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "刷新任务音乐地址", Details: "仅重新解析已完成任务中的作品，不创建任务、不扣额度、不下载媒体。请求体须为空；同作品每分钟最多一次。返回完整 job。"},
	{Method: http.MethodPost, Path: "/api/v1/jobs/{id}/cancel", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "取消自己的任务", Details: "id 为任务 ID。"},
	{Method: http.MethodPost, Path: "/api/v1/jobs/{id}/retry", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "重试自己的任务", Details: "id 为任务 ID。适用于失败或已取消的任务。"},
	{Method: http.MethodDelete, Path: "/api/v1/jobs/{id}", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "删除自己的任务", Details: "id 为任务 ID。进行中的任务须先取消。"},
	{Method: http.MethodGet, Path: "/api/v1/jobs/{id}/images/archive", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "直接下载图文 ZIP", Details: "id 为任务 ID。用于兼容已有客户端。", CurlExample: curlDownloadZIP},
	{Method: http.MethodPost, Path: "/api/v1/jobs/{id}/images/archive-download", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "获取图文 ZIP 下载链接", Details: "id 为任务 ID。返回的 downloadUrl 有效期为 5 分钟。", CurlExample: curlRequest},
	{Method: http.MethodGet, Path: "/api/v1/files/{id}", Group: groupJobs, Auth: apiAuthBearer,
		Summary: "下载自己的任务文件", Details: "id 为文件 ID。支持 Range。"},

	{Method: http.MethodGet, Path: "/api/v1/media-preview/{id}", Group: groupSigned, Auth: apiAuthSigned,
		Summary: "预览任务媒体", Details: "id 为文件 ID。使用链接中的 expires 和 signature。"},
	{Method: http.MethodGet, Path: "/api/v1/media-preview/{id}/thumbnail", Group: groupSigned, Auth: apiAuthSigned,
		Summary: "获取媒体缩略图", Details: "id 为文件 ID。使用缩略图链接中的 expires 和 signature。"},
	{Method: http.MethodGet, Path: "/api/v1/job-cover/{id}", Group: groupSigned, Auth: apiAuthSigned,
		Summary: "获取任务封面", Details: "id 为任务 ID。使用封面链接中的 expires 和 signature。"},
	{Method: http.MethodGet, Path: "/api/v1/archive-download/{id}", Group: groupSigned, Auth: apiAuthSigned,
		Summary: "下载已签名的图文 ZIP", Details: "将服务基址和签发接口返回的 downloadUrl 拼成完整链接，填入 DOWNLOAD_URL。", CurlExample: curlSignedZIP},
	{Method: http.MethodGet, Path: "/api/v1/asr-source/{id}", Group: groupSigned, Auth: apiAuthSigned,
		Summary: "获取转写服务所需的音频", Details: "使用音频链接中的 expires 和 signature。"},

	{Method: http.MethodGet, Path: "/api/v1/admin/stats", Group: groupOverview, Auth: apiAuthAdmin,
		Summary: "读取后台仪表盘统计"},
	{Method: http.MethodGet, Path: "/api/v1/admin/api-catalog", Group: groupOverview, Auth: apiAuthAdmin,
		Summary: "读取接口目录"},

	{Method: http.MethodGet, Path: "/api/v1/admin/users", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "查看用户列表", Details: "返回常规账号及其每日额度配置。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/users", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "创建账号", Details: "请求体包含 username、displayName、password 和 role。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/users/{id}", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "删除用户账号", Details: "id 为用户 ID。撤销账号会话，保留历史任务。当前登录的管理员和最后一个可用管理员不能删除。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/users/{id}/password", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "重置用户密码", Details: "id 为用户 ID。请求体包含 password，重置后下线该用户的会话。"},
	{Method: http.MethodPatch, Path: "/api/v1/admin/users/{id}/active", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "启用或禁用账号", Details: "id 为用户 ID。请求体使用 active 指定启用状态。"},
	{Method: http.MethodPut, Path: "/api/v1/admin/users/{id}/daily-task-limit", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "设置用户的每日任务额度", Details: "id 为用户 ID。dailyTaskLimit 为 null 时继承默认值，为 0 时不限。"},
	{Method: http.MethodGet, Path: "/api/v1/admin/users/{id}/sessions", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "查看指定用户的活跃会话", Details: "id 为用户 ID。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/sessions/{id}", Group: groupAdminUsers, Auth: apiAuthAdmin,
		Summary: "下线指定会话", Details: "id 为会话 ID。"},

	{Method: http.MethodGet, Path: "/api/v1/admin/cdks", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "列出 CDK 记录", Details: "支持 limit 和 offset 分页。列表仅返回密钥前缀。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/cdks", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "批量生成 CDK", Details: "请求体包含 count 和 taskLimit。生成响应会返回完整密钥。"},
	{Method: http.MethodPatch, Path: "/api/v1/admin/cdks/{id}", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "修改 CDK 任务额度", Details: "id 为密钥记录 ID。请求体包含 taskLimit。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/cdks/{id}/reset", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "重置 CDK 已用次数", Details: "id 为密钥记录 ID。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/cdks/{id}/rotate", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "轮换 CDK 密钥", Details: "id 为密钥记录 ID。响应返回新密钥，旧密钥和登录会话立即失效。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/cdks/{id}", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "吊销 CDK 及其会话", Details: "id 为密钥记录 ID。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/cdks/{id}/record", Group: groupAdminCDKs, Auth: apiAuthAdmin,
		Summary: "删除已吊销的 CDK 记录", Details: "id 为密钥记录 ID。相关任务和审计记录会保留。"},

	{Method: http.MethodGet, Path: "/api/v1/admin/jobs", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "查看所有用户的任务", Details: "支持 q、status、action、userId、limit、offset 和 view=summary。"},
	{Method: http.MethodGet, Path: "/api/v1/admin/jobs/{id}", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "查看任意任务的详情", Details: "id 为任务 ID。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/jobs/{id}/music/refresh", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "刷新任务音乐地址", Details: "仅重新解析已完成任务中的作品，不创建任务、不扣额度、不下载媒体。请求体须为空；同作品每分钟最多一次。返回完整 job。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/jobs/{id}/cancel", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "取消任意用户的任务", Details: "id 为任务 ID。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/jobs/{id}/retry", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "重试任意用户的任务", Details: "id 为任务 ID。适用于失败或已取消的任务。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/jobs/{id}", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "删除任意用户的任务", Details: "id 为任务 ID。进行中的任务须先取消。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/jobs/{id}/images/archive-download", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "获取任意用户的图文 ZIP 下载链接", Details: "id 为任务 ID。返回的 downloadUrl 有效期为 5 分钟。", CurlExample: curlRequest},
	{Method: http.MethodGet, Path: "/api/v1/admin/files/{id}", Group: groupAdminJobs, Auth: apiAuthAdmin,
		Summary: "下载任意用户的任务文件", Details: "id 为文件 ID。支持 Range。"},

	{Method: http.MethodGet, Path: "/api/v1/admin/settings/usage", Group: groupUsage, Auth: apiAuthAdmin,
		Summary: "读取全局任务额度和权限"},
	{Method: http.MethodPut, Path: "/api/v1/admin/settings/usage", Group: groupUsage, Auth: apiAuthAdmin,
		Summary: "保存全局任务额度和权限", Details: "请求体包含 dailyTaskLimit、allowInfo、allowDownload 和 allowTranscribe。"},

	{Method: http.MethodGet, Path: "/api/v1/admin/settings/providers", Group: groupProviders, Auth: apiAuthAdmin,
		Summary: "读取转写服务状态", Details: "返回启用和可用状态、默认服务与用户选择权限。"},
	{Method: http.MethodPut, Path: "/api/v1/admin/settings/providers", Group: groupProviders, Auth: apiAuthAdmin,
		Summary: "更新转写服务设置", Details: "支持部分更新。默认要求 HTTPS 或从回环地址访问。"},
	{Method: http.MethodDelete, Path: "/api/v1/admin/settings/providers/{id}/key", Group: groupProviders, Auth: apiAuthAdmin,
		Summary: "清除转写服务密钥", Details: "id 为供应商 ID。默认要求 HTTPS 或从回环地址访问。"},
	{Method: http.MethodPost, Path: "/api/v1/admin/settings/providers/{id}/test", Group: groupProviders, Auth: apiAuthAdmin,
		Summary: "测试转写服务连接", Details: "id 为供应商 ID。使用合成音频测试，可能计入用量。"},
}

func handleAPICatalog(w http.ResponseWriter, r *http.Request) {
	principal, _ := Principal(r.Context())
	if !adminAllowed(principal, w, r) {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, struct {
		Operations []apiOperation `json:"operations"`
	}{Operations: apiCatalog})
}
