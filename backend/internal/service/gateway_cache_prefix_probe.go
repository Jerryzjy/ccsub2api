package service

import (
	"log/slog"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
)

// 前缀稳定性探针（诊断 prompt cache 命中率低）。
//
// Anthropic prompt cache 是前缀精确匹配：渲染顺序 tools → system → messages，
// 前缀里任何一个字节变化都会让其后的所有缓存断点失效。订阅号路径会重写 system、
// 把原始 system 搬进 messages 开头、注入 cache 断点等，这些改写有可能让"本应逐字节
// 冻结"的前缀逐轮变化，从而毁掉缓存命中。
//
// 该探针在 body 最终敲定（所有改写 + CCH 签名之后、发往上游之前）对 tools / system /
// messages 三段分别算 xxHash64 并打一条 Info 日志。同一会话连续两轮对比这三个哈希：
//   - tools/system 哈希变了 → 前缀被改写污染（真凶1：system 不稳定 / 工具集变化）
//   - 只有 messages 哈希变、且变的是末尾追加 → 正常多轮，缓存可命中前缀
//   - tools/system 稳定但命中率仍低 → 多半是切号（见 sticky.account_switched）
//
// 纯观测：只读 body、算哈希、打日志，不修改 body，也不影响转发。
func logCachePrefixProbe(body []byte, accountID int64, sessionHash, modelID string) {
	if len(body) == 0 {
		return
	}

	toolsHash := hashJSONField(body, "tools")
	systemHash := hashJSONField(body, "system")

	// messages 拆成"除最后一条之外的前缀"和"最后一条"两部分：
	// 正常多轮对话里最后一条每轮都不同（新 user turn），但前缀应保持稳定。
	// 分开算哈希能区分"前缀被污染"(messagesPrefixHash 变) 和"正常追加"(只有 lastHash 变)。
	messagesPrefixHash, messagesLastHash := hashMessagesSplit(body)

	slog.Info("cache.prefix_probe",
		"session_hash", shortSessionHash(sessionHash),
		"account_id", accountID,
		"model", modelID,
		"tools_hash", toolsHash,
		"system_hash", systemHash,
		"messages_prefix_hash", messagesPrefixHash,
		"messages_last_hash", messagesLastHash,
	)
}

// hashJSONField 返回 body 中某个顶层字段原始 JSON 的 xxHash64（16 进制）。
// 字段不存在返回 "-"。
func hashJSONField(body []byte, field string) string {
	r := gjson.GetBytes(body, field)
	if !r.Exists() {
		return "-"
	}
	return hashHex([]byte(r.Raw))
}

// hashMessagesSplit 分别返回 messages[0..n-2] 前缀与 messages[n-1] 末条的哈希。
// messages 不存在或为空返回 ("-", "-")；只有一条时前缀为空串哈希。
func hashMessagesSplit(body []byte) (prefixHash, lastHash string) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return "-", "-"
	}
	arr := messages.Array()
	if len(arr) == 0 {
		return "-", "-"
	}

	d := xxhash.New()
	for i := 0; i < len(arr)-1; i++ {
		_, _ = d.WriteString(arr[i].Raw)
		_, _ = d.Write([]byte{0}) // 分隔符，避免相邻元素拼接歧义
	}
	prefixHash = formatHash(d.Sum64())
	lastHash = hashHex([]byte(arr[len(arr)-1].Raw))
	return prefixHash, lastHash
}

func hashHex(data []byte) string {
	return formatHash(xxhash.Sum64(data))
}

func formatHash(h uint64) string {
	const hexdigits = "0123456789abcdef"
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = hexdigits[h&0xf]
		h >>= 4
	}
	return string(b[:])
}
