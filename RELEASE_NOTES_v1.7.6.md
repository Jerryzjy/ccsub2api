# Sub2API v1.7.6

## Claude OAuth 兼容性修复

- 恢复 Claude OAuth 账号原有的 `sessionKey` 自动授权入口，同时保留手动 OAuth 授权和 `credentials.json` 导入。
- 恢复 `/api/v1/admin/accounts/cookie-auth` 路由，并继续使用原有完整 OAuth scope；只有 Anthropic 当前仍接受的有效 sessionKey 才能完成授权。
- 导入 `credentials.json` 时读取文件自带的 7d 用量百分比和重置时间，使新建账号可以立即显示 7d 用量窗口。
- 不导入缺少独立窗口结束字段的 5h 调度快照，避免对账号调度产生错误限制；5h 数据仍由原有响应头和主动用量流程维护。
- 不改变账号创建后的 Token 刷新、主动/被动缓存、用量同步、限额和调度逻辑。

本版本不会把任意 Cookie 或过期 sessionKey 转换为 OAuth 凭据；`credentials.json` 导入仍要求文件中已经包含有效 OAuth Token。
