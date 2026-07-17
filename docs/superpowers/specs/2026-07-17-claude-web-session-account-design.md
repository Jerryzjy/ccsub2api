# Claude Web Session Account Design

## 目标

在 Sub2API 中新增原生 `web_session` 账号类型，让管理员可以导入自己控制的 Claude Web 完整 Cookie，并将账号加入现有 Anthropic 账号池。该功能不依赖外部 Claude2api 服务，也不把 Web 会话伪装成 OAuth 凭据。

首版成功标准：

- 管理员可以粘贴 Cookie Header、上传 Netscape Cookie 文件，或通过账号备份 JSON 批量导入账号。
- 完整 Cookie 是首选凭据；只有没有完整 Cookie 时才使用 `sessionKey` 回退。
- 创建或导入账号时自动探测 organization 和会话状态。
- `web_session` 账号复用现有分组、调度、并发、代理、粘性会话、冷却和状态管理能力。
- `/v1/messages` 可以选择 `web_session` 账号，并将 Claude Web SSE 转换为 Anthropic 兼容响应。
- Web Session 不进入 OAuth Token Provider 或 OAuth 刷新任务。

## 方案选择

### 方案 A：原生 Web Session Adapter（采用）

新增独立账号类型和传输适配器。优点是账号状态、代理和调度都由 Sub2API 统一管理；缺点是需要维护 Claude Web 私有协议。

### 方案 B：外部 Sidecar

把 Claude2api 作为 Anthropic 兼容上游。改动少，但账号状态和 Cookie 生命周期被拆到外部服务，不能满足原生账号池管理目标。

### 方案 C：伪装为 APIKey 账号

使用自定义 Base URL 把 Web 会话隐藏在 APIKey 后面。实现快，但错误分类、账号失效、导入导出和代理绑定都不准确。

## 账号模型

新增常量：

```text
AccountTypeWebSession = "web_session"
```

仅允许 `platform=anthropic` 使用该类型。凭据结构：

```json
{
  "cookie": "sessionKey=...; sessionKeyLC=...; lastActiveOrg=...",
  "session_key": "可选回退值",
  "organization_id": "自动探测并缓存",
  "cookie_updated_at": "RFC3339 时间"
}
```

规则：

- `cookie` 和 `session_key` 至少存在一个。
- 如果 `cookie` 包含 `sessionKey`，后端解析得到的值优先于单独提交的 `session_key`。
- 不存储上传文件路径，只存储规范化 Cookie Header。
- `cookie`、`session_key` 作为敏感凭据，沿用现有 credentials 加密、脱敏和管理员备份行为。
- `organization_id` 可以从 `lastActiveOrg` 读取，但首次创建仍需要向上游验证。

## Cookie 导入

管理端创建账号时增加“Claude Web Session”方式：

- 粘贴 Cookie Header。
- 上传 Netscape Cookie 文件；浏览器端读取文本并发送原始内容，后端负责解析和规范化。
- 只粘贴 `sessionKey`，作为兼容回退方式。

后端提供纯解析器，支持：

- 标准 `name=value; name2=value2` Cookie Header。
- Netscape tab-separated Cookie 文件。
- 忽略注释、空行、过期 Cookie 和非 `claude.ai` 域 Cookie。
- 同名 Cookie 优先保留 `.claude.ai`、根路径和较晚过期的条目。
- 输出稳定、可测试的 Cookie Header，不记录 Cookie 值到日志。

账号备份 JSON 继续使用现有数据管理接口；新增类型和凭据通过现有 `credentials` 字段导入导出。导入校验必须拒绝缺少 Cookie/sessionKey 或平台不是 Anthropic 的记录。

## Web Transport

新增独立 `ClaudeWebClient`，不复制无许可证第三方仓库代码。客户端职责：

1. 使用账号绑定的出站代理和固定浏览器环境发送请求。
2. 完整 Cookie 模式原样发送规范化 Cookie；回退模式只补充可由客户端生成的环境 Cookie。
3. 调用 `/api/organizations` 验证会话并获取 organization UUID。
4. 创建临时 conversation。
5. 把 Anthropic Messages 请求转换成 Claude Web completion 请求。
6. 解析上游 SSE，并输出统一的文本增量事件。
7. 请求结束后尽力删除临时 conversation；清理失败不覆盖已经成功的模型响应。

浏览器环境必须保持一致：TLS profile、User-Agent、`sec-ch-ua` 和平台 Header 使用同一版本配置。`cf_clearance`、`__cf_bm` 和 routing hint 只能复用真实值，不能生成伪造值。

首版不实现持久 Claude Web conversation。每个入站请求已经包含完整消息历史，因此使用临时 conversation 能避免跨用户状态泄漏。

## Gateway 集成

Anthropic Gateway 在账号选择完成后按类型分流：

- `oauth`、`setup-token` 和 `apikey` 保持现有逻辑。
- `web_session` 调用 Web Session Transport。

Web Transport 接收现有网关已完成的模型映射和请求清理结果，并返回与现有网关一致的流式/非流式响应。账号继续参与：

- 分组和优先级调度。
- 并发占用与释放。
- API Key scoped sticky session。
- 账号级代理绑定。
- 429、过载和临时不可调度冷却。
- 用量记录；上游没有精确 token usage 时使用现有估算逻辑，并标记为估算值。

## 能力边界

Claude Web 私有协议不是正式 Anthropic API。首版保证文本消息和基础流式输出，以下能力明确降级：

- 不承诺 prompt cache usage 精确值。
- 不承诺完整透传 Anthropic beta 功能。
- 工具调用只在已验证 Claude Web 请求/响应格式后逐项开放，未支持时返回明确错误，不静默改写为文本。
- 不提供 OAuth access token、refresh token 或 OAuth 自动续期。

## 错误处理

上游错误映射：

- `401`、登录页重定向、`Sign in again`：账号状态设为 error，原因 `web_session_expired`。
- `403` 或 Cloudflare challenge HTML：临时不可调度，原因 `web_session_cloudflare`；保留账号以便管理员更新 Cookie/代理。
- `429`：进入现有 rate-limit 冷却，遵循 `Retry-After`。
- `5xx`、网络超时：按现有出站尝试预算处理，不把单个请求错误立即永久禁用账号。
- organization 为空或无权限：账号创建/测试失败，不进入调度池。

日志只记录账号 ID、阶段、HTTP 状态和错误分类，不记录 Cookie、sessionKey、Cloudflare 内容或完整上游响应。

## 管理端体验

创建账号弹窗新增 Web Session 选项，字段包括：

- Cookie 文件上传。
- Cookie Header 文本框。
- sessionKey 回退输入框。
- 代理和分组等现有通用字段。

提交按钮执行“解析 -> 探测 -> 创建”。探测期间展示加载状态；失败时保留表单内容并显示可操作的错误分类。编辑账号时敏感字段保持现有脱敏语义：留空表示不修改，显式替换才更新。

账号列表将类型显示为“Claude Web”，并在状态详情中区分“会话失效”“Cloudflare/出口异常”“上游限流”。

## 测试策略

测试先行，按以下顺序实现：

1. 账号类型常量、平台限制和导入校验。
2. Cookie Header/Netscape 解析、去重、域过滤和敏感日志保护。
3. Web Client organization、conversation、completion 和 cleanup 请求契约。
4. SSE 文本增量与停止事件转换。
5. Gateway 对 `web_session` 的分流、并发释放、流式响应和错误分类。
6. 401/403/429 与现有账号状态、冷却和粘性会话清理的集成测试。
7. 管理端凭据构造、文件导入和类型展示测试。
8. 现有 OAuth、Setup Token 和 APIKey 回归测试。

所有后端测试使用 `httptest.Server` 或注入传输，不使用真实 Claude 账号。真实 Cookie 只用于管理员手动验收，不进入测试夹具、日志或仓库。

## 非目标

- 自动绕过验证码、重新登录或 recent-sign-in。
- 从 sessionKey 兑换 OAuth Token。
- 自动生成 Cloudflare 签名 Cookie。
- 首版支持附件、Artifacts、MCP Connector 或 Claude Web 全部工具能力。
- 将管理员 Cookie 暴露给普通用户或客户端。

## 发布与回滚

功能通过新增账号类型自然隔离。没有 `web_session` 账号时，现有路径行为不变。出现上游协议变更时，可以在管理端禁用该类型账号，而不影响 OAuth/APIKey 账号。数据库不新增表，回滚代码后保留的未知类型账号不会被调度，重新部署支持版本后可恢复。
