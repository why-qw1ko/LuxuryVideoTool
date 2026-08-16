# Luxury Capture

Luxury Capture 是一个自托管的抖音内容解析、下载与转写工具。用户粘贴抖音分享文本后，由服务端完成作品解析、媒体下载、音频提取、ASR 转写与结果导出。

当前部署重点是嵌入 Go 服务端的网页版。Android 与 Windows 客户端源码、设计和后续产物规划仍保留；只是当前 GitHub Actions 暂时只构建 Linux 服务端运行包，不自动构建 Android APK 或 Windows 客户端。

## 技术测试与使用声明

本项目仅用于个人学习、技术测试与自托管验证，不以盈利为目的，不提供公开 SaaS 服务，不鼓励也不支持批量采集、未经授权的下载、传播或商业使用。

使用者必须自行确认其行为符合所在地法律法规、平台服务条款、版权规则和内容授权范围。不得将本项目用于绕过访问控制、侵犯版权、批量抓取、公开分发第三方内容或其他违法违规用途。

本仓库未附带开源许可证。除作者明确书面授权外，禁止：

- 二次修改、换皮、改名或基于本项目制作衍生版本。
- 修改后发布、传播或提供下载。
- 二次打包、二次分发或上传到第三方平台。
- 商业销售、转售、代部署收费或作为付费服务提供。
- 移除或修改项目名称、声明、版权与使用边界。
- 将本项目用于公开、多租户或大规模采集服务。

本声明不构成法律意见。任何部署、使用、传播行为及其后果由执行者自行承担。

## 功能范围

- 解析抖音视频、图文与动图作品信息。
- 视频：下载无水印视频并转写口播文案。
- 图文/动图：下载全部配图（动图含动态版 MP4），画廊预览、逐张下载或打包 ZIP，配文即文案，支持作品背景音乐播放。
- 使用 FFmpeg / FFprobe 提取音频。
- 默认通过硅基流动 SenseVoice 转写口播文案。
- 可导出 TXT、Markdown 和 meta JSON。
- 支持任务进度、历史记录、搜索、重试、取消和删除；媒体保留到期自动清理，删除任务同时删除媒体。
- 支持管理员在网页中配置 API Key 与转写模型。
- Android 与 Windows 客户端作为后续分发形态保留，当前服务器部署不依赖客户端产物。

## 技术组成

```text
services/api_go/       Go REST API、内嵌网页版、后台 Worker
services/api_go/web/   轻量网页版静态资源
apps/client_flutter/   Android / Windows 客户端源码
scripts/               本地启动、打包与部署辅助脚本
docs/                  运维、API、ADR 与验收文档
deploy/                Caddy、systemd、Docker 等部署草案
```

服务端使用 SQLite 保存用户、任务、文件索引与运行配置；媒体文件、转写结果和临时文件保存在服务端数据目录中。

## Linux 服务器部署

推荐通过 GitHub Actions 构建 Linux 服务端运行包。服务器无需放置源码。

1. 进入 GitHub 仓库 `Actions`。
2. 选择 `Server Linux Package`。
3. 点击 `Run workflow`。
4. 下载构建产物 `server-linux-packages`。
5. 根据服务器架构选择：

```text
douyin-capture-linux-amd64-<版本号>.tar.gz
douyin-capture-linux-arm64-<版本号>.tar.gz
```

普通 x86 云服务器通常使用 `amd64`。

服务器依赖：

```bash
sudo apt update
sudo apt install -y ffmpeg chromium
```

推荐部署目录：

```bash
sudo mkdir -p /opt/douyin-capture
sudo chown -R $USER:$USER /opt/douyin-capture
```

上传并解压：

```bash
cd /opt/douyin-capture
tar -xzf douyin-capture-linux-amd64-<版本号>.tar.gz
cd douyin-capture-linux-amd64-<版本号>
```

首次配置 `.env`：

```env
HTTP_ADDR=0.0.0.0:7788
FFMPEG_PATH=ffmpeg
FFPROBE_PATH=ffprobe
DOUYIN_BROWSER_PATH=/usr/bin/chromium
```

初始化管理员并启动：

```bash
./initialize-admin.sh
./start-web.sh
```

访问：

```text
http://服务器IP:7788
```

长期运行建议使用 systemd 托管，并通过 Nginx 或 Caddy 配置 HTTPS 反向代理。完整说明见 [Linux 服务器部署说明](docs/operations/Linux服务器部署说明.md)。

## Windows 本地运行

源码运行：

```powershell
.\scripts\windows\initialize-local.ps1
.\scripts\windows\start-web.ps1
```

生成 Windows 本地运行包：

```powershell
.\scripts\windows\build-local-package.ps1 -FFmpegBin C:\ffmpeg\bin
```

输出目录：

```text
dist\douyin-capture-windows-<版本号>
```

运行包内执行：

```powershell
.\initialize-admin.ps1
.\start-web.ps1
```

更多说明见 [Windows 网页版使用说明](docs/operations/Windows网页版使用说明.md) 和 [启动与测试说明](docs/operations/启动与测试说明.md)。

## Android 与 Windows 客户端

仓库仍包含 Flutter 客户端源码：

```text
apps/client_flutter/
```

客户端目标：

- Android APK / AAB。
- Windows 桌面客户端或安装包。
- 与同一 Go 服务端账号、任务历史和结果数据联动。

当前阶段 GitHub Actions 不自动构建这些客户端产物，避免服务器部署流程被 Flutter、Android SDK 和 Windows 构建环境阻塞。需要客户端产物时，应在具备 Flutter、Android Studio、JDK 和 Windows C++ 桌面工具链的构建环境中单独执行。

## 关键配置

`.env` 中常用配置：

```env
HTTP_ADDR=127.0.0.1:8080
DATABASE_PATH=./data/app.db
DATA_DIR=./data
JWT_SIGNING_KEY_FILE=./secrets/jwt.key
FFMPEG_PATH=ffmpeg
FFPROBE_PATH=ffprobe
DOUYIN_BROWSER_PATH=
ASR_PROVIDER=siliconflow_sensevoice
ASR_MODEL=FunAudioLLM/SenseVoiceSmall
SILICONFLOW_API_KEY=
DAILY_ASR_BUDGET_CNY=5
MONTHLY_ASR_BUDGET_CNY=20
# 自托管内网/私有部署想通过纯 HTTP 远程配置 API Key 时设为 1（默认 0，公网保持关闭）
ALLOW_INSECURE_PROVIDER_SETTINGS=0
```

生产或公网环境至少应确认：

- `HTTP_ADDR`：服务器端口，例如 `0.0.0.0:7788`。
- `JWT_SIGNING_KEY_FILE`：至少 32 字节随机密钥文件。
- `FFMPEG_PATH` / `FFPROBE_PATH`：媒体处理工具路径。
- `DOUYIN_BROWSER_PATH`：Linux 服务器建议显式配置 Chromium 路径。
- `SILICONFLOW_API_KEY`：也可由管理员登录网页后保存。
- `DAILY_ASR_BUDGET_CNY` / `MONTHLY_ASR_BUDGET_CNY`：ASR 费用预算上限。

真实密钥、`.env`、数据库和 `secrets/` 不得提交到仓库。

## 数据与备份

运行数据通常位于运行包目录内：

```text
.env
data/
secrets/
```

备份或迁移时应保留这些文件。删除 `data/` 会清空用户、任务记录和文件索引；删除 `secrets/` 会导致既有登录令牌失效。

## 技术测试状态

本项目处于技术测试阶段：

- 真实抖音解析可能受平台页面结构、接口参数、Cookie、风控和 CDN 策略影响。
- 云端 ASR 调用会产生第三方服务费用，应设置预算上限。
- 公网部署必须自行配置 HTTPS、防火墙、安全组、备份和进程守护。
- 当前不面向公开注册、多租户、大规模任务队列或商业化运营。

如需验证服务状态：

```bash
curl http://127.0.0.1:7788/health/live
curl http://127.0.0.1:7788/health/ready
```

## 安全边界

- 服务端仅应部署给少量可信用户使用。
- 不提供开放下载代理能力。
- 不应暴露数据库、数据目录、密钥文件或运行日志。
- API Key 不应写入客户端包、公开配置或日志。
- 公网访问建议只通过 HTTPS 入口暴露。

## 相关文档

- [Linux 服务器部署说明](docs/operations/Linux服务器部署说明.md)
- [Windows 网页版使用说明](docs/operations/Windows网页版使用说明.md)
- [启动与测试说明](docs/operations/启动与测试说明.md)
- [开发运行说明](docs/operations/开发运行说明.md)
- [API 文档](docs/api/README.md)
