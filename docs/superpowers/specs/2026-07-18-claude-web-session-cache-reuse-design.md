# Claude Web Session 缓存复用设计

## 目标

在不伪造 Anthropic 官方 Prompt Cache 数据的前提下，为 `web_session` 账号实现跨请求的 Claude Web conversation 复用，减少重复发送完整历史造成的本地费用增长和上游上下文重建。

核心 SLO：

- `eligible_reuse_hit_rate >= 95%`。
- 分母只包含满足复用前置条件的连续请求；首轮请求、TTL 到期、模型变化、账号故障切换、历史分叉和显式禁用不进入分母。
- 该指标是 Sub2API 的 Web conversation 复用率，不是 Anthropic 官方 `cache_read_input_tokens` 命中率。

## 方案选择

### 方案 A：仅使用 OAuth/Setup Token

能够使用官方 5m/1h Prompt Cache，但无法利用只有 Cookie 的账号，不满足账号池目标。

### 方案 B：本地伪造缓存 Token

只修改计费结果，不减少上游请求内容和上下文处理，可能低估账号负载，不采用。

### 方案 C：缓存能力路由 + Web conversation 复用（采用）

- 含 `cache_control` 的请求优先选择 OAuth/Setup Token。
- 只能使用 Web Session 时，复用同一 Claude Web conversation，并只发送新增轮次。
- 本地单独记录估算复用数据，不冒充官方缓存数据。

## 请求路由

账号选择增加缓存能力偏好：

1. 请求含顶层或内容块级 `cache_control` 时，优先候选为 Anthropic OAuth/Setup Token。
2. 缓存能力账号不可用时，允许回退到 `web_session`，但必须进入 conversation 复用路径。
3. 同一会话已有有效 sticky 绑定时，优先保留原账号，除非账号已经不可调度。
4. 不同新会话继续使用现有 LRU/负载感知策略分散到不同真实账号。
5. `organization_id`、`account_uuid` 或邮箱相同的重复导入记录折叠为同一上游身份，避免重复记录之间轮询。

## Web conversation 状态

Redis key 必须按以下维度隔离：

```text
web-session:conversation:{api_key_id}:{group_id}:{account_id}:{session_hash}
```

状态结构：

```json
{
  "organization_id": "...",
  "conversation_id": "...",
  "parent_message_uuid": "...",
  "model": "...",
  "digest_chain": "...",
  "context_tokens_estimated": 0,
  "created_at": "RFC3339",
  "last_used_at": "RFC3339",
  "ttl_seconds": 300
}
```

安全要求：

- key 必须包含 API Key、分组、账号和会话 hash，任一维度不同都不得复用。
- Redis value 不保存 Cookie、sessionKey 或完整消息正文。
- digest 只保存不可逆摘要链。
- 同一 key 使用短时分布式锁，防止并发请求同时追加导致 conversation 分叉。

## 复用流程

### 首轮或未命中

1. 创建 Claude Web conversation。
2. 发送完整 `system + messages` 文本。
3. 成功后保存 conversation ID、assistant message UUID、模型、摘要链和估算上下文 token。
4. 不再在每次成功响应后立即删除 conversation。

### 连续轮次命中

1. 读取 Redis 状态并验证账号、模型、TTL。
2. 计算当前请求摘要链，确认它严格扩展已保存历史。
3. 只提取新增的 user/tool-result 文本作为本轮 prompt。
4. 使用保存的 assistant message UUID 作为 `parent_message_uuid`。
5. 成功后原子更新摘要链、parent UUID、估算 token 和 TTL。

### 失效与重建

以下情况不得复用：

- 历史不是已保存摘要链的严格扩展。
- 模型改变。
- sticky 账号改变或发生故障切换。
- conversation 返回 401、403、404 或上游明确表示 conversation 无效。
- TTL 到期。

失效时删除 Redis 状态，尽力删除旧 conversation，并以完整上下文创建新 conversation。重建失败不得回写半成品状态。

## TTL

- 请求没有缓存 TTL 时，conversation 状态默认保留 5 分钟。
- 请求明确包含 `cache_control.ttl = "1h"` 时，状态保留 1 小时。
- 每次成功复用刷新剩余 TTL。
- 本地 TTL 仅控制 conversation 状态寿命，不宣称等同 Anthropic 官方 Prompt Cache。

## 用量与金额

新增 Web Session 估算字段：

- `web_context_reused_input_tokens_estimated`
- `web_context_new_input_tokens_estimated`
- `web_conversation_reuse_hit`
- `web_conversation_reuse_miss_reason`

计费与调度规则：

- 首轮完整输入按现有 Web Session 输入估算记录。
- 复用轮次只把本轮新增 prompt 计入普通输入；历史复用量单独记录为估算值。
- 不直接填充或伪造官方 `cache_creation_input_tokens`、`cache_read_input_tokens`。
- 五小时窗口仍保留保守安全边界；conversation 复用成功后使用新增输入估算，未命中时使用完整输入估算。

## 账号池与安全控制

`web_session` 必须进入与订阅账号一致的：

- RPM/TPM 计数和预检查。
- User Message Queue 串行或节流模式。
- 最大活跃会话限制。
- 五小时窗口费用和日/周配额。
- sticky 绑定刷新、失效清理和故障切换。

同一会话不得为了普通负载均衡主动切号。只有原账号不可用时才允许切换；切换后的第一轮一定是缓存未命中和完整重建。

## 可观测性

新增计数器：

- `web_conversation_reuse_eligible_total`
- `web_conversation_reuse_hit_total`
- `web_conversation_reuse_miss_total{reason}`
- `web_conversation_rebuild_total{reason}`
- `web_conversation_sticky_switch_total`

指标：

```text
eligible_reuse_hit_rate = hit_total / eligible_total
```

后台展示最近 5 分钟、1 小时和 24 小时复用率。低于 95% 时按 miss reason 排查，不通过降低分母或伪造 hit 达标。

## 测试

必须先写失败测试覆盖：

1. 同一 API Key、分组、账号和 session 的连续历史只创建一次 conversation。
2. 第二轮只发送新增内容，并使用上一轮 assistant UUID 作为 parent。
3. 不同 API Key、分组、账号或 session 绝不共享 conversation。
4. 模型变化、历史分叉、TTL 到期和故障切换会完整重建。
5. 失败响应不会更新摘要链或 parent UUID。
6. 同一 key 并发追加保持串行且不分叉。
7. `organization_id` 相同的重复 Web Session 记录会折叠。
8. Web Session 纳入 RPM/TPM/UMQ 与运行状态统计。
9. OAuth/Setup Token 的官方 5m/1h 缓存路径保持不变。
10. 复用指标只统计 eligible follow-up，不包含首轮和强制重建。

## 发布

- 默认通过功能开关启用，支持按账号关闭。
- 先灰度少量 Web Session 账号，观察复用率、conversation 失效率、429 和五小时窗口消耗。
- 达到 `eligible_reuse_hit_rate >= 95%` 且错误率无明显上升后扩大范围。
- 回滚时关闭 conversation 复用即可恢复每请求临时 conversation；OAuth/Setup Token 不受影响。

