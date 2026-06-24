package service

// AccountHealthInput 健康评分输入信号。
type AccountHealthInput struct {
	ErrorRate        float64 // 0-1
	RecentRateLimits int     // 近窗口内被限流次数
	AvgLatencyMs     int     // 平均延迟
}

// AccountHealthScore 返回 0-100 健康分，越高越健康。
// 综合错误率、近期限流史、平均延迟，供调度降优先级/暂停参考。
func AccountHealthScore(in AccountHealthInput) int {
	score := 100.0
	score -= in.ErrorRate * 50                // 错误率最多扣 50
	score -= float64(in.RecentRateLimits) * 8 // 每次限流扣 8
	if in.AvgLatencyMs > 5000 {
		over := float64(in.AvgLatencyMs-5000) / 1000.0
		score -= over * 2 // 超 5s 部分每秒扣 2
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return int(score)
}
