-- 154: prevent duplicate anthropic accounts by account_uuid (partial unique).
-- 防封根因修复：同一上游 Anthropic 账号被重复导入成多条记录，会被调度劈裂、
-- 摧毁 prompt cache 命中率（2026-06-23/24 大批 429 事故根因）。库层加唯一约束兜底。
--
-- CONCURRENTLY 避免长写锁；必须在事务外执行（文件名 _notx 约定）。
-- 应用前置：库内同一 account_uuid 不能已有重复，否则建索引会失败——
-- 先用 cmd/dedup-accounts 工具去重，再应用本迁移。
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS uq_accounts_anthropic_account_uuid
ON accounts ((credentials->>'account_uuid'))
WHERE deleted_at IS NULL
  AND platform = 'anthropic'
  AND credentials->>'account_uuid' IS NOT NULL
  AND credentials->>'account_uuid' != '';
