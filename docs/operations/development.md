# 开发运行说明

## 锁定工具链

- Go：1.26.5
- Flutter：3.44.0 stable
- Android：最低 API 26，JDK 17
- Windows：Windows 10 1809+ / Windows 11，Visual Studio C++ 桌面工具链

CI 锁定上述版本。升级工具链必须单独审查依赖兼容性并记录变更。

## 配置

复制仓库根目录 `.env.example` 到本地 `.env`，不要提交 `.env`、Secret 或签名文件。M1 需要 `DATABASE_PATH`、`DATA_DIR`、`JWT_SIGNING_KEY_FILE`、Token TTL 和登录限速配置。

JWT HMAC 密钥文件必须至少包含 32 个随机字节，并限制为服务账号只读。不要把示例路径下的真实文件提交到仓库。

## Go 服务

```powershell
Set-Location services/api_go
go test ./...
go run ./cmd/server
```

默认监听 `127.0.0.1:8080`，存活探针为 `GET /health/live`。

## 初始化两个账号

密码通过私有临时文件传入，命令不会把密码写入参数列表。Linux 权限不得超过 `0600`；Windows 应使用仅当前用户可读的 ACL：

```text
go run ./cmd/admin create-user --username owner --display-name Owner --role admin --password-file <private-file>
go run ./cmd/admin create-user --username collaborator --display-name Collaborator --role user --password-file <private-file>
```

重置密码会撤销该用户全部设备会话：

```text
go run ./cmd/admin reset-password --user-id <user-id> --password-file <private-file>
go run ./cmd/admin set-active --user-id <user-id> --active=false
```

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
