# Flutter 与 Windows 客户端源码备份

备份日期：2026-11-27（Asia/Shanghai）。

按用户要求，Flutter 客户端及其 Windows 桌面端已停止使用。删除工作目录中的 `apps/client_flutter/` 前，先生成本备份，并完成 ZIP CRC 校验及逐文件 SHA-256、字节一致性校验；随后移除原目录及旧客户端运行入口。

## 备份内容

- 压缩包：`flutter-windows-source-2026-11-27.zip`。
- Flutter 客户端源码共 69 个文件：共用 `lib/`、Flutter Android 平台工程、Windows runner、测试、依赖清单与锁文件等，保留原有相对路径和文件字节。
- 附带删除或调整前的检查脚本、Windows 网页服务打包脚本、旧客户端工作流、项目说明、开发运行说明、忽略规则和历史 Flutter ADR，共 8 个辅助文件，供恢复时参考。
- 清单：`flutter-windows-source-2026-11-27.manifest.json`，记录归档路径、大小、逐文件 SHA-256 及排除文件。
- 校验值：`SHA256SUMS`，记录压缩包与清单的 SHA-256。

仅备份源码与辅助文件；排除可重新生成的 `build/`、`.dart_tool/`、`.gradle/`、`.idea/`、`ephemeral/`，以及本机 `local.properties` 和 Flutter 插件缓存，不包含安装包或 Flutter SDK。

## 当前状态声明

- 已移除：Flutter Android 客户端、Flutter Windows 桌面客户端及其共用源码，原路径为 `apps/client_flutter/`。
- 继续保留：`android/` 下的 Kotlin + Jetpack Compose 原生安卓端、Go 服务端与内嵌网页版。
- `scripts/windows/` 用于在 Windows 上运行和打包 Go 网页服务，继续保留；其中旧 Flutter 客户端打包分支已移除。
- 本目录是历史源码归档，不作为当前运行或构建入口。

## 恢复方法

先根据 `SHA256SUMS` 校验压缩包。需要恢复客户端时，在仓库根目录仅提取压缩包中的 `apps/client_flutter/`，即可还原原有源码目录。辅助文件位于压缩包内的原相对路径，按需对照合并，避免覆盖后续修改。依赖、缓存和本机 SDK 路径需要重新配置，旧客户端运行与打包入口需要按需恢复。
