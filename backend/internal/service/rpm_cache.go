package service

import "context"

// RPMCache 账号级每分钟计数器缓存接口
// 同时承载两类每账号/每分钟的 Redis 计数器：
//   - RPM：每分钟请求数（key: rpm:{accountID}:{min}）
//   - TPM：每分钟 token 数（key: tpm:{accountID}:{min}）
// 两者共用同一 Redis 客户端与分钟窗口口径，故合并在同一接口内。
type RPMCache interface {
	// IncrementRPM 原子递增并返回当前分钟的计数
	// 使用 Redis 服务器时间确定 minute key，避免多实例时钟偏差
	IncrementRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPM 获取当前分钟的 RPM 计数
	GetRPM(ctx context.Context, accountID int64) (count int, err error)

	// GetRPMBatch 批量获取多个账号的 RPM 计数（使用 Pipeline）
	GetRPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)

	// AddTPM 原子累加 token 数并返回当前分钟的累计 token 计数。
	// tokens 应为一次请求的总 token（输入+输出+缓存），在请求完成、用量落账时调用。
	AddTPM(ctx context.Context, accountID int64, tokens int) (total int, err error)

	// GetTPM 获取当前分钟的累计 token 计数
	GetTPM(ctx context.Context, accountID int64) (total int, err error)

	// GetTPMBatch 批量获取多个账号的当前分钟 token 计数（使用 Pipeline）
	GetTPMBatch(ctx context.Context, accountIDs []int64) (map[int64]int, error)
}
