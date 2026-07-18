# Claude Cookie / sessionKey 自动转换 OAuth 账号设计

## 目标

管理员只需提供自己控制的 Claude Cookie 文件、Cookie Header，或直接粘贴 `sessionKey`。Sub2API 在服务端完成解析、OAuth 换签、凭据校验和账号创建，最终生成 `platform=anthropic`、`type=oauth` 的账号。

用户不需要生成、下载或再次上传 `credentials.json`。该文件只是 Claude CLI 的凭据载体；Sub2API 直接保存等价的 OAuth 字段。

首版成功标准：

- 支持 Netscape Cookie 文件、Cookie Header 和原始 `sessionKey` 三种输入。
- 输入有效且上游允许授权时，自动获得 access token、refresh token、到期时间和 scopes。
- 整个操作原子化：换签成功并验证后才创建账号；失败时不留下半成品账号。
- 创建结果直接进入现有 Anthropic OAuth 账号池，复用刷新、调度、代理、并发、分组和限流能力。
- 原始 Cookie 和 `sessionKey` 不写入 OAuth 账号 credentials，不出现在响应、日志或错误信息中。

## 方案选择

### 方案 A：服务端换签并原子创建 OAuth 账号（采用）

新增一个面向管理员的原子接口。前端提交 Cookie/sessionKey 和账号配置，后端解析输入、执行 OAuth 换签、验证凭据并创建账号，只向前端返回脱敏后的账号信息。

优点是 token 不经过浏览器二次传递，也不存在 `credentials.json` 中间文件；失败边界清晰。缺点是需要维护 Claude OAuth 授权请求的兼容策略。

### 方案 B：先返回 credentials.json，再由用户导入

与截图工具交互类似，但增加一次敏感凭据下载、保存和上传，容易泄漏，且不符合用户希望“一步创建账号”的目标，不采用。

### 方案 C：继续保存为 web_session 账号

不需要 OAuth 换签，但仍依赖 Claude Web Cookie 协议和会话安全检查，无法获得 OAuth 自动刷新能力，不符合本功能目标。

## 管理端交互

在 Anthropic 的 OAuth 账号创建流程中增加“Cookie / sessionKey 自动转换”方式，接受：

1. Netscape 格式 Cookie 文件；
2. `name=value; name2=value2` Cookie Header；
3. 原始 `sk-ant-sid...` sessionKey。

输入优先级：完整 Cookie 文件 > Cookie Header > 原始 sessionKey。完整 Cookie 中解析出的 `sessionKey` 优先于备用输入框。

表单继续使用现有账号配置：名称、代理、分组、并发、优先级、倍率及配额保护。代理必须贯穿组织查询、授权码获取、token 交换、首次凭据验证和后续刷新。

提交期间显示阶段化状态：

- 正在解析 Cookie；
- 正在验证 Claude 会话；
- 正在获取 OAuth 授权；
- 正在验证 OAuth 凭据；
- 正在创建账号。

成功后关闭表单并刷新账号列表。失败时保留表单输入，但前端不得把敏感字段写入持久化存储或错误上报。

## 后端接口

新增原子接口：

```text
POST /api/v1/admin/accounts/claude-cookie-oauth
```

请求由两部分组成：

```json
{
  "name": "Claude OAuth Account",
  "cookie": "可选，Cookie 文件内容或 Cookie Header",
  "session_key": "可选，原始 sessionKey",
  "proxy_id": 1,
  "group_ids": [],
  "concurrency": 1,
  "priority": 1,
  "rate_multiplier": 1
}
```

规则：

- `cookie` 与 `session_key` 至少存在一个。
- 复用现有 `NormalizeClaudeWebCookie` 解析器，不新增第二套 Cookie 语法。
- 接口不接受客户端直接指定 access token、refresh token、OAuth client ID 或任意授权端点。
- 响应只返回标准脱敏账号 DTO，不返回 token 或 Cookie。

## OAuth 换签流程

服务层新增独立的 `ClaudeSessionOAuthConverter`，负责把有效 sessionKey 换成现有 `TokenInfo`：

1. 解析并规范化 Cookie，提取 `sessionKey`。
2. 使用账号选择的代理和一致的浏览器 TLS/HTTP 指纹调用组织查询接口。
3. 生成新的 PKCE verifier、challenge 和 state。
4. 使用兼容 scope 请求授权码。
5. 校验回调 state，并使用 verifier 交换 OAuth token。
6. 校验 access token、refresh token、到期时间及 scope。
7. 使用 access token执行一次无计费的账号/用量探测。
8. 将 token 转换为 Sub2API 现有扁平 credentials 并创建 OAuth 账号。

兼容 scope 采用已从有效 Claude CLI 凭据中验证的最小集合：

```text
user:chat user:inference user:profile
```

现有手动 Claude Code OAuth 流程继续使用原 scope，不做全局替换。Cookie 自动转换使用独立策略，避免影响已经工作的浏览器授权和 setup-token 流程。

如果上游明确要求重新登录、验证码或 recent-sign-in，转换器返回可操作的错误，不尝试伪造验证结果、生成 Cloudflare Cookie或循环重试。

## 凭据存储

成功换签后保存现有 Anthropic OAuth 格式：

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": "Unix 秒",
  "scope": "user:chat user:inference user:profile",
  "token_type": "Bearer"
}
```

账号固定为：

```text
platform = anthropic
type = oauth
```

不把以下内容写入 OAuth 账号：

- 原始 Cookie；
- `sessionKey`；
- 上传文件名或本地路径；
- 授权码、PKCE verifier、state；
- 上游原始响应。

后续 access token 刷新复用现有 `ClaudeTokenProvider`、刷新锁、token 版本检查和缓存逻辑。首次创建必须确认 refresh token 存在；缺少 refresh token 时拒绝创建，避免生成几小时后永久失效的账号。

## 错误分类

- `cookie_invalid`：Cookie 格式无效或没有可用 sessionKey。
- `session_invalid`：sessionKey 失效或组织查询返回未登录。
- `region_or_proxy_blocked`：区域不可用、代理失败或被重定向到不可用页面。
- `oauth_recent_signin_required`：上游明确要求最近登录或重新认证。
- `oauth_authorization_failed`：授权码阶段失败且不属于上述类别。
- `oauth_token_exchange_failed`：token 交换失败。
- `oauth_refresh_token_missing`：响应没有 refresh token。
- `oauth_validation_failed`：token 已签发但首次账号/用量验证失败。
- `account_create_failed`：OAuth 成功但数据库创建失败；不得把 token 返回给客户端。

日志只记录账号操作 ID、代理 ID、阶段、HTTP 状态和错误分类。所有 Cookie、sessionKey、authorization code、access token、refresh token 和上游敏感响应必须沿用现有脱敏策略。

## 原子性与重复提交

- 在 OAuth 换签开始前生成一次性操作 ID。
- 同一管理员、相同 sessionKey 指纹的并发换签只允许一个执行，避免重复签发和风控放大。
- sessionKey 指纹使用服务端 HMAC，仅用于短期去重，不保存可离线撞库的裸 SHA-256。
- token 验证成功后再进入数据库事务创建账号和分组关系。
- 根据账号 UUID、组织 UUID 或 access token 指纹执行现有重复账号检查。
- 前端超时后重复提交时，后端返回已有成功结果或“操作仍在进行”，不再次请求上游。

## 测试策略

实现遵循测试先行：

1. Cookie 文件、Cookie Header 和原始 sessionKey 的输入优先级测试。
2. 请求 DTO 校验、敏感字段脱敏和响应不包含 token 的处理器测试。
3. 窄 scope、PKCE、state 校验和代理贯穿的服务测试。
4. 组织查询、授权码、token 交换及首次验证的 `httptest.Server` 契约测试。
5. refresh token 缺失、recent-sign-in、区域重定向、Cloudflare HTML、401、403、429 和 5xx 分类测试。
6. 换签失败不创建账号、创建失败不返回 token 的原子性测试。
7. 重复提交和并发换签去重测试。
8. 新建 OAuth 账号进入现有 token provider、用量查询和网关调度的集成测试。
9. 前端文件读取、sessionKey 输入、阶段状态和错误保留测试。
10. 现有手动 OAuth、setup-token 和 web_session 流程的回归测试。

自动化测试不使用真实 Cookie 或 token。真实账号只用于管理员在可用地区代理下进行一次人工验收，凭据不得进入测试夹具、日志或仓库。

## 发布与回滚

新入口与现有 OAuth 和 web_session 入口并存，不修改已有账号。上线初期通过管理员设置开关控制 Cookie 自动转换入口。

关闭开关后：

- 禁止新的 Cookie/sessionKey 自动换签；
- 已成功创建的 OAuth 账号继续正常刷新和调度；
- 不影响手动 OAuth、setup-token、API key 或 web_session 账号。

## 非目标

- 不要求用户生成或导入 `credentials.json`。
- 不保存 Cookie 作为 OAuth 账号的长期凭据。
- 不伪造验证码、recent-sign-in 结果或 Cloudflare 安全 Cookie。
- 不调用未知第三方转换服务，也不把 sessionKey 发送给除管理员所配置 Claude 上游以外的服务。
- 不在本功能中改变现有 Claude Web `web_session` 协议和网关行为。
