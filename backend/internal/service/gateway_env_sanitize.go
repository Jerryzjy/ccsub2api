package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 机器指纹字段：Claude Code 会在系统提示的 <env> 块（以及消息里的 <system-reminder> 块）
// 注入本机信息（Platform / OS Version / Shell 等）。当网关伪造成 Claude Code（mimic 路径）时，
// 若把下游客户端带来的这些真机字段原样转发，会造成两类封号信号：
//  1. 头体矛盾：伪造头声明 Linux，但体内 <env> 写 darwin —— 一个请求里自相矛盾。
//  2. 多机泄露：同一账号在上游看到很多台不同机器的 <env> —— “账号被多人共享”特征。
// 因此在 mimic 路径、CCH 签名之前，把这些字段归一为与伪造头 OS 一致的 canonical 值。
//
// 仅改写 <env> 与 <system-reminder> 定界块内的字段，绝不触碰块外的用户自由文本。
var (
	envPlatformRe  = regexp.MustCompile(`Platform:\s*\S+`)
	envOSVersionRe = regexp.MustCompile(`OS Version:\s*[^\n<]+`)
	envShellRe     = regexp.MustCompile(`Shell:\s*\S+`)

	envBlockRe          = regexp.MustCompile(`(?s)<env>.*?</env>`)
	systemReminderBlkRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
)

// canonicalEnvValues 返回与给定 X-Stainless-OS 一致的 Platform / OS Version / Shell 值。
// 保持与伪造头（claude.DefaultHeaders["X-Stainless-OS"]）同源，头 OS 变化时体自动跟随。
func canonicalEnvValues(stainlessOS string) (platform, osVersion, shell string) {
	switch strings.ToLower(strings.TrimSpace(stainlessOS)) {
	case "macos", "darwin":
		return "darwin", "Darwin 24.4.0", "zsh"
	case "windows", "win32":
		return "win32", "Windows 10.0.22631", "powershell"
	default: // Linux 及未知，一律归一到 Linux（与当前伪造头一致）
		return "linux", "Linux 6.8.0-45-generic", "bash"
	}
}

// rewriteMachineEnvInBlock 只在单个 <env>/<system-reminder> 块内改写机器字段。
func rewriteMachineEnvInBlock(block, platform, osVersion, shell string) string {
	block = envPlatformRe.ReplaceAllString(block, "Platform: "+platform)
	block = envOSVersionRe.ReplaceAllString(block, "OS Version: "+osVersion)
	block = envShellRe.ReplaceAllString(block, "Shell: "+shell)
	return block
}

// sanitizeMachineEnvText 对一段文本内的所有 <env>/<system-reminder> 块做字段归一，块外文本原样保留。
func sanitizeMachineEnvText(text, platform, osVersion, shell string) string {
	repl := func(block string) string {
		return rewriteMachineEnvInBlock(block, platform, osVersion, shell)
	}
	if strings.Contains(text, "<env>") {
		text = envBlockRe.ReplaceAllStringFunc(text, repl)
	}
	if strings.Contains(text, "<system-reminder>") {
		text = systemReminderBlkRe.ReplaceAllStringFunc(text, repl)
	}
	return text
}

// sanitizeSystemMachineEnv 归一系统提示（system 字段）里 <env>/<system-reminder> 块的机器字段，
// 使其与伪造头 OS 一致。支持 string 与 []block 两种 system 形态。
func sanitizeSystemMachineEnv(body []byte, stainlessOS string) []byte {
	systemResult := gjson.GetBytes(body, "system")
	if !systemResult.Exists() {
		return body
	}
	platform, osVersion, shell := canonicalEnvValues(stainlessOS)

	if systemResult.Type == gjson.String {
		orig := systemResult.String()
		newText := sanitizeMachineEnvText(orig, platform, osVersion, shell)
		if newText != orig {
			if updated, err := sjson.SetBytes(body, "system", newText); err == nil {
				body = updated
			}
		}
		return body
	}

	if systemResult.IsArray() {
		idx := 0
		systemResult.ForEach(func(_, item gjson.Result) bool {
			text := item.Get("text")
			if text.Exists() && text.Type == gjson.String {
				orig := text.String()
				newText := sanitizeMachineEnvText(orig, platform, osVersion, shell)
				if newText != orig {
					if updated, err := sjson.SetBytes(body, fmt.Sprintf("system.%d.text", idx), newText); err == nil {
						body = updated
					}
				}
			}
			idx++
			return true
		})
	}
	return body
}
