# Flutter 客户端

Android 与 Windows 共用 `lib/` 中的业务和 UI，平台差异仅位于 `lib/platform/` 或原生 runner。

M5 已定义登录、Token 安全存储与自动轮换、任务提交和状态轮询、历史离线缓存、结果复制/下载、删除确认、主题/通知/下载目录设置，以及 Android `ACTION_SEND text/plain` 分享接收。分享只预填首页，必须由用户确认后提交。

首次运行先在设置页填写服务端地址。开发环境可使用局域网地址；生产环境必须使用 M6 配置的 HTTPS 域名。真实 Android/Windows 构建、权限和系统通知仍须在有 Flutter SDK 的环境验证。
