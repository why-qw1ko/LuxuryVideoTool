# ADR-001：Flutter 构建 Android 与 Windows

- 状态：accepted
- 日期：2026-08-11

## 决策

Android 与 Windows 使用同一 Flutter UI 和业务代码；平台差异仅进入 `platform/` 适配层。

## 影响

核心依赖必须同时支持两端，平台专属实现不能渗入业务层。

