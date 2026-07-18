# Claude Web Session 缓存与错误语义修正设计

## 目标

修正 `web_session` 的两个独立问题：后续请求读取到 conversation 状态后仍被误判为未命中，以及部分普通传输或响应解码错误被扩大为账号切换。修复后，同一会话在账号和凭据不变时必须真实复用原 Claude Web conversation；错误处理不得通过新增轮询或切号掩盖坏账号。

## 已确认根因

1. 旧 conversation 状态确实会从 Redis 读取，但写入状态和后续请求使用的摘要语义不同。Anthropic 字符串、文本块数组及 `cache_control` 元数据会得到不同摘要，即使它们最终发送给 Claude Web 的文本完全相同，因此后续请求被误判为 `history_diverged`。
2. 编辑 `web_session` 凭据时，通用敏感字段合并会保留旧 Cookie 和派生的 `organization_id`。请求构造优先使用完整 Cookie，导致新 `session_key` 可能没有实际生效。
3. 普通传输、响应解码和 malformed SSE 错误原本不是 `UpstreamFailoverError`。提交 `4ee4123` 将它们统一转换为 failover，虽然避免了通用 502，却扩大了账号切换语义，不符合本次确认的行为边界。

## 与官方 OAuth 账号的边界

Anthropic OAuth 账号使用官方 API Prompt Cache；`web_session` 无法获得官方 cache usage，只能复用 Claude Web conversation 并仅发送新增轮次。Sub2API 可以把已确认复用的上游 conversation 映射到本地 `cache_read_input_tokens`，但该数字必须标明为本地估算，不能伪装成 Anthropic 官方计数。

`web_session` 继续使用现有统一调度和粘性账号绑定。正常会话应固定在同一账号；本修复不新增普通错误触发的账号切换，也不改变现有明确 HTTP 状态错误的池级策略。

## 设计

### Conversation 命中

- conversation key 由 API key、group、account ID、credential fingerprint 和稳定 session hash 组成。
- credential fingerprint 只基于实际生效的 sessionKey，且不保存原始凭据。
- 摘要只计算实际发送给 Claude Web 的文本语义：system、user 和 assistant 的展平文本。
- 字符串与单/多文本块数组在文本相同时生成相同摘要；`cache_control`、TTL 和 JSON key 顺序不参与摘要。
- 只有读取状态、摘要前缀匹配、模型一致、TTL 有效且复用原 conversation 时，才产生 `cache_read_input_tokens`。
- 未命中时创建新 conversation 并报告新保留上下文；Redis 不可用时回退为普通 input，不报告 cache creation/read。

### 凭据更新隔离

- 只更新 `session_key` 时删除旧 Cookie、旧 `organization_id` 和旧 email。
- 只更新 Cookie 时删除旧 `session_key`、旧 `organization_id` 和旧 email。
- 仅编辑非认证字段时保留现有认证凭据及派生身份。
- 凭据指纹变化后生成不同 conversation key，旧状态自然失效，不跨凭据复用。

### 错误处理

- `ClaudeWebHTTPError` 和 Claude Web SSE 中明确分类的错误继续使用现有结构化状态及池级策略。
- `context.Canceled` 与 `context.DeadlineExceeded` 原样返回，不转换为上游账号错误。
- 普通传输、响应解码和 malformed SSE 错误返回脱敏的 `web_session_upstream` 诊断，不新增 failover。
- 诊断记录失败阶段（organization、create conversation、completion transport、completion stream），账号 ID 和安全的 HTTP 状态；不得记录 Cookie、sessionKey、完整上游正文或 token。
- 客户端仍收到稳定、脱敏的 Web Session 错误，不再只看到无法定位账号阶段的通用 `Upstream request failed`。

## 测试

1. 两轮请求使用字符串和文本块混合表示，断言第二轮执行 Redis Get 并命中原 conversation。
2. 第二轮只向 completion 端点发送最新用户消息，并包含上一轮 parent message UUID。
3. 断言命中请求产生正数 `cache_read_input_tokens`；首次请求和 Redis 不可用回退不产生 cache read。
4. 更新 sessionKey 或 Cookie 后断言旧认证形式与派生 organization 被删除，且旧 conversation 不被读取。
5. 普通传输/解码错误断言不会变成 `UpstreamFailoverError`，而是结构化、脱敏的 Web Session 错误。
6. 明确 HTTP 401、403、429 及 context cancellation 保持原有语义。
7. 运行全部 `internal/service` 测试、相关 handler 测试、`go vet`、`git diff --check`；全仓库检查中的既有基线失败单独报告，不混入本修复。

## 非目标

- 不把 Cookie/sessionKey 自动转换为 OAuth。
- 不实现 Anthropic 官方 Prompt Cache。
- 不修改统一账号池的既有调度算法。
- 不自动修复无法判断新旧关系的历史冲突凭据；账号在下一次明确更新认证凭据时完成清理。
- 不扩大支持图片、工具调用或其他 Claude Web 当前不支持的内容类型。

## 验收标准

- 相同会话的第二轮请求能够以测试证据证明读取并复用原 conversation，而不是仅写入 Redis。
- 凭据变更不会复用旧 conversation，也不会由旧 Cookie 覆盖新 sessionKey。
- 新增修复不会让普通 Web Session 错误触发额外账号切换。
- 所有新增响应和日志都不泄露认证信息。
