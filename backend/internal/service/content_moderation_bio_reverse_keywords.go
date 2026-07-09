package service

import "strings"

// 默认内容审核关键词兜底表。
//
// 目的：OpenAI omni-moderation 对"生物武器 / 逆向工程"两类**几乎分不出**（都勉强落到
// illicit 且阈值偏高），漏判之后 Anthropic 自己的审核就会当场封掉转发账号。
// 这里直接用关键词短路，让 pre_block 模式在分类器之前就把这两类拦掉。
//
// 范围原则：只收"明显是要搞"的高特征词，避免误伤"我在做课程作业讲什么是勒索软件"。
// 中英文混排、简体繁体都收。所有关键词 lower-case 匹配（content_moderation 已有归一）。
var defaultBlockedKeywordsBioAndReverse = []string{
	// 生物武器 / 病原
	"生物武器", "生化武器", "细菌武器",
	"bioweapon", "bio-weapon", "biological weapon",
	"合成病毒", "制造病毒", "改造病毒",
	"synthesize virus", "engineer virus", "enhance pathogen", "pathogen enhancement",
	"anthrax weapon", "smallpox synthesis",
	"ricin synthesis", "ricin weapon",
	"如何制造蓖麻毒素",

	// 逆向工程 / 漏洞利用 / 破解（用户原话提到"逆向"作为明显违规）
	"逆向工程", "反编译", "反汇编", "破解密码", "暴力破解", "撞库", "脱壳",
	"keygen", "crack software", "crack me",
	"reverse engineer", "decompile", "disassemble",
	"zero-day exploit", "0day exploit", "exploit code",
	"如何写病毒", "如何做木马",
	"how to hack", "how to crack",

	// 明显违规的色情 / 未成年相关（用户原话提到"色情"，独立分组方便后续调阈值）
	"色情", "黄片", "裸聊", "未成年", "幼女", "萝莉",
	"child sexual", "csam", "minor sexual",
}

// mergeBlockedKeywords 把兜底表与管理员自定义关键词合并并去重。
// 兜底表在前，自定义在后——方便 admin 后台继续追加，调试时一眼能看到自己的条目。
func mergeBlockedKeywords(defaults, custom []string) []string {
	seen := make(map[string]struct{}, len(defaults)+len(custom))
	out := make([]string, 0, len(defaults)+len(custom))
	for _, s := range defaults {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	for _, s := range custom {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}