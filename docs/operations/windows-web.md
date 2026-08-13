# Windows 网页版使用说明

网页版已经嵌入 Go 服务，不需要 Node.js、Flutter 或单独的 Web 服务器。

## 最简单的运行包

如果拿到的是已构建的 `douyin-capture-windows-<版本号>` 目录，不需要安装 Go、Flutter 或 FFmpeg：

1. 运行 `initialize-admin.ps1`，输入至少 12 位密码。
2. 运行 `start-web.ps1`。
3. 浏览器会自动打开 <http://127.0.0.1:8080>，登录后配置硅基流动 API Key。

必须保留整个目录，不能只复制 EXE；`data`、`secrets` 和 `.env` 都属于本机运行数据。备份或迁移时应连同整个目录一起处理。

## 环境

1. 安装 Go 1.26.5，并确保 `go version` 可在 PowerShell 中执行。
2. 仅解析作品信息不需要 FFmpeg；下载视频需要服务端可访问抖音；转写需安装 FFmpeg/FFprobe 并配置云端 ASR。
3. 在项目根目录执行：

```powershell
.\scripts\windows\start-web.ps1
```

首次启动会从 `.env.example` 创建本地 `.env`，并生成未提交 Git 的 JWT 密钥。启动后访问 <http://127.0.0.1:8080>。

登录页可填写“API 服务地址”。轻量网页版与 API 一起由 Go 服务提供，因此切换地址时浏览器会前往目标服务，不需要开放高风险的跨域访问。

## 创建账号

先把密码保存到项目外的临时文本文件，再执行：

```powershell
$env:DATABASE_PATH = '.\data\app.db'
Set-Location .\services\api_go
go run .\cmd\admin create-user --username owner --display-name Owner --role admin --password-file C:\private\password.txt
```

服务端和管理命令必须使用同一个 `DATABASE_PATH`。创建后删除密码临时文件。

## 功能边界

- 可用：登录、解析、下载、转写任务提交、进度、历史、结果查看和下载、取消、重试、删除。
- 仅本机使用时保持 `HTTP_ADDR=127.0.0.1:8080`，不应直接监听公网。
- 用户始终输入抖音链接。服务端自动下载视频、用 FFmpeg 提取临时音频并上传硅基流动，所以 `localhost` 可以完成解析、下载和转写，不需要公网域名。
- 第三方密钥可通过服务端 `.env` 配置：`SILICONFLOW_API_KEY`、可选 `ALIYUN_DASHSCOPE_API_KEY`、模型、端点、单价及预算；密钥不会写入浏览器。
- 管理员也可在网页版“大模型 API Key”区域保存或清除密钥。浏览器只提交本次输入，服务端加密保存且不会回显；生产环境必须使用 HTTPS。
- 只配置硅基流动即可在本地转写；同时配置两者且提供公网 `PUBLIC_BASE_URL` 时，硅基流动为主服务，只有可重试错误才切换至阿里云。
- 生产 HTTPS、签名安装包、真实抖音/ASR 联调仍属于部署与验收工作，不能由源码代替。
