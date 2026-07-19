# Sub2API v1.7.5

## Claude Web Session 修复

- 修复 `web_session` 账号后台测试绕过生产代理策略，导致“测试正常、客户端调用 502”的问题。
- 缺少必需代理时返回明确的 `web_session_proxy_required`，不再显示无法定位原因的 `Upstream request failed`。
- 账号测试和自动探测现在会将缺少必需代理的账号标记为不可调度。
- 客户端请求包含当前不支持的工具、图片或文档内容时返回明确的 `400 invalid_request_error`，不再误报为账号连接失败。
- 保留既有 Web Session 账号粘性和调度语义，不新增自动轮询或切号。

## 升级后检查

如果账号提示 `web_session_proxy_required`，请为该账号绑定上游代理，或在明确接受直连风险后关闭 `proxy.require_for_upstream`。
