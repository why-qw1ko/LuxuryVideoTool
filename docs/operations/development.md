# 开发运行说明

## 锁定工具链

- Go：1.26.5
- Flutter：3.44.0 stable
- Android：最低 API 26，JDK 17
- Windows：Windows 10 1809+ / Windows 11，Visual Studio C++ 桌面工具链

CI 锁定上述版本。升级工具链必须单独审查依赖兼容性并记录变更。

## 配置

复制仓库根目录 `.env.example` 到本地 `.env`，不要提交 `.env`、Secret 或签名文件。M0 服务只读取 `HTTP_ADDR` 与 `LOG_LEVEL`；其他变量在对应里程碑启用。

## Go 服务

```powershell
Set-Location services/api_go
go test ./...
go run ./cmd/server
```

默认监听 `127.0.0.1:8080`，存活探针为 `GET /health/live`。

版本构建参数：

```text
-X .../internal/version.Version=<version>
-X .../internal/version.Commit=<commit>
-X .../internal/version.BuildTime=<rfc3339>
```

## Flutter 客户端

```powershell
Set-Location apps/client_flutter
flutter pub get
flutter test
flutter run -d windows --dart-define=APP_VERSION=0.1.0-dev
```

Android 使用 `flutter run -d <device>`。客户端只启用 Android 与 Windows；业务与 UI 放在 `lib/`，平台差异放在 `lib/platform/`。

## 一次性检查

Windows 运行 `./scripts/check.ps1`，Linux/macOS 运行 `./scripts/check.sh`。这些脚本不调用真实抖音或收费 ASR。

