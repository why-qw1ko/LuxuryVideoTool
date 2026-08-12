# Secret 管理基线

- 仓库仅提交 `.env.example`，真实 `.env` 被忽略。
- ASR API Key、JWT 密钥、Android/Windows 签名材料不得写入客户端、源码、CI 日志或异常响应。
- 本地开发通过环境变量或忽略的 Secret 文件注入。
- CI/生产通过平台 Secret 存储注入，生产 JWT 使用只读文件路径。
- 日志不得输出 Authorization、Cookie、Token、完整分享文案或供应商原始响应。
- 若怀疑泄漏，应立即撤销密钥、扫描历史并记录安全事件；仅删除当前文件不足以消除 Git 历史泄漏。

