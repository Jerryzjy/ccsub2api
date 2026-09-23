# Sub2API v1.7.7

## 修复新模型被上游拒绝（claude_code_version_too_old）

- 将对外伪装的 Claude Code CLI 版本号提升到 `2.1.280`。Anthropic 会按出站 `User-Agent` 的版本号对新模型做准入判定，版本过低会直接返回 400：`Claude Code X.Y.Z does not support this model; version 2.1.280 or newer is required`。
- 新增账号指纹的版本地板：缓存指纹的 CLI 版本低于内置版本时自动抬升并回写。此前指纹只有在客户端送来更高版本 `User-Agent` 时才会升级，老账号会一直停留在指纹创建时的版本上，升级服务端也不生效。
- `device_id`、操作系统、架构等设备特征在抬升版本时保持不变，不会被风控判定为新设备。
- billing attribution 块中的 `cc_version` 由指纹 `User-Agent` 派生，随之一并修正。
- 模型列表新增 `claude-opus-5`、`claude-sonnet-5`；`claude-opus-5` 按 Opus 4.5 以来的规则在转发前剔除采样参数（temperature / top_p / top_k）。

升级后无需清理 Redis：已有账号在下一次请求时自动抬升指纹版本。
