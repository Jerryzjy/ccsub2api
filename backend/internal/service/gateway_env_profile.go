package service

import (
	"hash/fnv"
	"net/http"
)

// 环境画像多样化（车队多样化）：
//
// 现状问题：mimic 步骤把每个账号的出站头强制刷成同一张 claude.DefaultHeaders，
// 导致全车队 X-Stainless-OS/Arch/UA 逐字节相同——“一堆账号长着同一台 Linux/arm64 机器”
// 是明显的聚类特征。
//
// 方案：给每账号按其冻结的 device_id 加权分配一套 OS 画像，并**冻结**（device_id 稳定 →
// 画像稳定，账号 A 永远是某台 Mac、B 永远是某台 Windows）。只变 OS 相关维度
// （X-Stainless-OS/Arch + <env> 机器字段），复用现有 CLI/SDK/runtime/beta 常量。
//
// 关键：两套命名约定不能混淆——
//   - X-Stainless-OS 头：真实 @anthropic-ai/sdk detect-platform.ts 用**大写** MacOS/Windows/Linux；
//   - <env> 块 "Platform:" 行：node process.platform，用**小写** darwin/win32/linux。
// 值均已对照 anthropic-sdk-typescript/src/internal/detect-platform.ts 核对。
// （注：claude2api 在这里把 X-Stainless-OS 写成了小写 platform，是其一处指纹 bug，本实现不照抄。）

// envOSProfile 只承载随 OS 变化的两个头字段；<env> 块的机器字段由 canonicalEnvValues
// 统一按 XStainlessOS 派生，保证头与体一致、且只有一个真源。
type envOSProfile struct {
	XStainlessOS   string // "MacOS" / "Windows" / "Linux"
	XStainlessArch string // "arm64" / "x64"
}

// envOSProfilesWeighted 三套主流真实配对及其权重（近似真实 Claude Code 用户 OS 分布：
// 开发者以 macOS 为主，Linux 次之，Windows 再次）。权重仅用于打散分布，非精确统计。
var envOSProfilesWeighted = []struct {
	weight  int
	profile envOSProfile
}{
	{50, envOSProfile{XStainlessOS: "MacOS", XStainlessArch: "arm64"}},
	{30, envOSProfile{XStainlessOS: "Linux", XStainlessArch: "x64"}},
	{20, envOSProfile{XStainlessOS: "Windows", XStainlessArch: "x64"}},
}

// envOSProfileForSeed 按 seed 加权确定一套画像；同一 seed 恒定返回同一套（冻结）。
// seed 传账号冻结的 device_id（ClientID），从而画像随 device_id 一并冻结、无需额外持久化。
func envOSProfileForSeed(seed string) envOSProfile {
	total := 0
	for _, e := range envOSProfilesWeighted {
		total += e.weight
	}
	if seed == "" || total <= 0 {
		return envOSProfile{XStainlessOS: "Linux", XStainlessArch: "x64"}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	pick := int(h.Sum32() % uint32(total))
	for _, e := range envOSProfilesWeighted {
		if pick < e.weight {
			return e.profile
		}
		pick -= e.weight
	}
	return envOSProfilesWeighted[0].profile
}

// applyAccountEnvProfileHeaders 用账号冻结画像覆盖 mimic 之后的 X-Stainless-OS/Arch。
// 需在 applyClaudeCodeMimicHeaders（强制全局 DefaultHeaders）之后调用，覆盖才生效。
func applyAccountEnvProfileHeaders(h http.Header, p envOSProfile) {
	if h == nil {
		return
	}
	setHeaderRaw(h, "X-Stainless-OS", p.XStainlessOS)
	setHeaderRaw(h, "X-Stainless-Arch", p.XStainlessArch)
}
