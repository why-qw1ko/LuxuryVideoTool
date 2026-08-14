# Linux 服务器部署说明

本文适用于 GitHub Actions 生成的 Linux 运行包。

本项目仅用于个人学习、技术测试与自托管验证。不得用于商业转售、公开 SaaS、批量采集、未经授权的内容传播或二次分发。

## 生成运行包

在 GitHub 仓库页面：

1. 进入 `Actions`。
2. 选择 `Server Linux Package`。
3. 点击 `Run workflow`。
4. 等待任务完成后，在本次运行页面下载 `server-linux-packages` artifact。

也可以推送 `v*` 标签自动触发构建。

产物包含：

```text
douyin-capture-linux-amd64-<版本号>.tar.gz
douyin-capture-linux-arm64-<版本号>.tar.gz
SHA256SUMS
```

## 服务器依赖

Ubuntu / Debian：

```bash
sudo apt update
sudo apt install -y ffmpeg chromium
```

如果 `chromium` 包不存在，可改用：

```bash
sudo apt install -y chromium-browser
```

确认路径：

```bash
which ffmpeg
which ffprobe
which chromium
which chromium-browser
```

## 解压运行包

推荐放置目录：

```bash
sudo mkdir -p /opt/douyin-capture
sudo chown -R $USER:$USER /opt/douyin-capture
cd /opt/douyin-capture
```

```bash
tar -xzf douyin-capture-linux-amd64-0.1.0-dev.tar.gz
cd douyin-capture-linux-amd64-0.1.0-dev
```

ARM 服务器使用 `arm64` 包。

## 配置端口

首次运行前，`.env` 不存在时脚本会从 `.env.example` 自动生成。

需要监听服务器 `7788` 端口时，修改 `.env`：

```env
HTTP_ADDR=0.0.0.0:7788
FFMPEG_PATH=ffmpeg
FFPROBE_PATH=ffprobe
DOUYIN_BROWSER_PATH=/usr/bin/chromium
```

如果浏览器路径是 `/usr/bin/chromium-browser`，按实际结果填写。

## 初始化管理员

全新目录首次执行：

```bash
./initialize-admin.sh
```

如果提示用户名已存在，说明 `owner` 已创建，可直接启动。

## 启动服务

临时测试：

```bash
./start-web.sh
```

访问：

```text
http://服务器IP:7788
```

长期运行建议使用 systemd：

```bash
sudo nano /etc/systemd/system/douyin-capture.service
```

示例：

```ini
[Unit]
Description=Douyin Capture
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/douyin-capture/douyin-capture-linux-amd64-0.1.0-dev
ExecStart=/opt/douyin-capture/douyin-capture-linux-amd64-0.1.0-dev/start-web.sh
Restart=always
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
```

启动并设置开机自启：

```bash
sudo systemctl daemon-reload
sudo systemctl enable douyin-capture
sudo systemctl start douyin-capture
```

查看状态和日志：

```bash
sudo systemctl status douyin-capture
journalctl -u douyin-capture -f
```

## 数据目录

运行数据在包目录内：

```text
data/
secrets/
.env
```

迁移或备份时应保留这些文件。删除 `data/` 会清空用户、任务和文件索引；删除 `secrets/` 会导致既有登录令牌失效。

## 正式公网使用

公网建议使用 Nginx 或 Caddy 做 HTTPS 反向代理，不建议长期裸露 HTTP。
