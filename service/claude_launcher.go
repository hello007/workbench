package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// claudeCommandResolver 可注入的 claude 启动解析函数（测试替换避免依赖真机 shim）。
// 保留给 AI 功能面板等默认 claude 场景；AI 对话走 chatCommandResolver（支持 launcher 配置）。
var claudeCommandResolver = resolveClaudeCommand

// chatCommandResolver 可注入的 AI 对话启动解析函数（launcher 配置 → 启动命令），
// 测试替换避免依赖真机 shim。
var chatCommandResolver = ResolveChatCommand

// claudeShimTargetRe 从 npm shim 内容提取引号包裹的 .exe/.js 目标路径。
// 兼容两种 batch 前缀形态："%dp0%\..."（npm 新 shim）与 "%~dp0\..."（无尾 %）。
var claudeShimTargetRe = regexp.MustCompile(`"((?:%~dp0|%dp0%)\\[^"]+\.(?:exe|js))"`)

// resolveClaudeCommand 解析 claude 命令的真实启动目标，返回 (可执行文件, 前缀参数)。
//
// 为什么不直接 exec "claude"：Windows 下 npm 全局安装的 claude 是 .cmd shim，
// Go 的 exec 需经 cmd.exe /c 转发，cmd 对参数二次解析时 JSON 类参数
// （--json-schema/--mcp-config）的引号会被破坏，claude 实测报
// 「--json-schema is not valid JSON」exit 1（大 prompt 走 argv 时换行/引号同理必炸）。
// 解析 shim 末行目标：原生 .exe 直接启动；.js 拼 node 启动——均为真实 PE，
// CreateProcess 直接调用走标准 MS 引号规则，JSON 完好。
//
// 解析任一环节失败一律回退 ("claude", nil)（旧行为），保证可用性优先。
func resolveClaudeCommand() (string, []string) {
	if runtime.GOOS != "windows" {
		return "claude", nil
	}
	shim, err := exec.LookPath("claude")
	if err != nil {
		return "claude", nil
	}
	if name, prefix, ok := resolveFromPath(shim); ok {
		return name, prefix
	}
	return "claude", nil
}

// resolveFromPath 解析指定 shim/可执行文件路径的真实启动目标（读 shim → 正则提取
// → batch 变量展开 → 目标存在性校验）。
// 返回 (可执行文件, 前缀参数, 是否解析成功)：原生 exe 直接返回 (path, nil, true)；
// .cmd/.bat 解析出 js 入口返回 (node, [js 路径], true)；任一环节失败返回
// (path, nil, false)，回退策略由调用方决定（resolveClaudeCommand 回退命令名，
// ResolveChatCommand 尽力回退 shim 本身）。
func resolveFromPath(shimPath string) (string, []string, bool) {
	ext := strings.ToLower(filepath.Ext(shimPath))
	if ext != ".cmd" && ext != ".bat" {
		// 非 batch shim（原生安装形态），直接用
		return shimPath, nil, true
	}
	raw, err := os.ReadFile(shimPath)
	if err != nil {
		return shimPath, nil, false
	}
	// npm shim 可能有多行候选，取最后一个匹配（末行才是实际执行行）
	matches := claudeShimTargetRe.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		return shimPath, nil, false
	}
	target := matches[len(matches)-1][1]
	shimDir := filepath.Dir(shimPath)
	// 展开 batch 变量前缀：%dp0% / %~dp0 均展开为 shim 所在目录（尾部带分隔符）
	target = strings.ReplaceAll(target, `%~dp0\`, shimDir+`\`)
	target = strings.ReplaceAll(target, `%dp0%\`, shimDir+`\`)
	// 目标文件必须真实存在，防 shim 内容解析出幽灵路径
	if _, err := os.Stat(target); err != nil {
		return shimPath, nil, false
	}
	if strings.ToLower(filepath.Ext(target)) == ".js" {
		// js 入口：优先 shim 同目录 node.exe（npm/nvm 布局），退而 PATH 上的 node
		nodeExe := filepath.Join(shimDir, "node.exe")
		if _, err := os.Stat(nodeExe); err != nil {
			node, lerr := exec.LookPath("node")
			if lerr != nil {
				return shimPath, nil, false
			}
			nodeExe = node
		}
		return nodeExe, []string{target}, true
	}
	return target, nil, true
}

// ResolveChatCommand 按 launcher 配置解析 AI 对话启动命令，返回 (可执行文件, 前缀参数, 错误)。
// 三分支语义：
//   - launcher 为空：默认 claude（现行为，走 claudeCommandResolver）；
//   - 绝对路径/含分隔符：存在性校验后按 shim 解析（.cmd/.bat 读 shim 变体，
//     其余直用）；路径不存在报中文错误（含配置值）；
//   - 纯命令名：LookPath 命中后同 shim 解析；未命中报中文错误（含配置值）。
//
// shim 解析失败（无目标行/目标不存在/node 缺失）时尽力回退 shim 路径本身
// （与 resolveClaudeCommand 的可用性优先同口径；cmd.exe 二次解析的引号风险同旧行为）。
func ResolveChatCommand(launcher string) (string, []string, error) {
	launcher = strings.TrimSpace(launcher)
	if launcher == "" {
		name, prefix := claudeCommandResolver()
		return name, prefix, nil
	}
	if runtime.GOOS != "windows" {
		// 非 Windows 无 .cmd shim 变体问题：LookPath 同时覆盖命令名与绝对路径
		p, err := exec.LookPath(launcher)
		if err != nil {
			return "", nil, fmt.Errorf("未找到 AI 启动命令「%s」，请确认已安装或在启动配置中填写完整路径", launcher)
		}
		return p, nil, nil
	}
	var candidate string
	if strings.ContainsRune(launcher, '\\') || strings.ContainsRune(launcher, '/') || filepath.IsAbs(launcher) {
		// 显式路径：先存在性校验（LookPath 不覆盖任意绝对路径形态）
		if _, err := os.Stat(launcher); err != nil {
			return "", nil, fmt.Errorf("AI 启动程序不存在: %s（请检查启动命令配置）", launcher)
		}
		candidate = launcher
	} else {
		shim, err := exec.LookPath(launcher)
		if err != nil {
			return "", nil, fmt.Errorf("未在 PATH 中找到 AI 启动命令「%s」，请确认已安装或改填完整路径", launcher)
		}
		candidate = shim
	}
	if name, prefix, ok := resolveFromPath(candidate); ok {
		return name, prefix, nil
	}
	// shim 解析失败：尽力回退 shim 本身（可用性优先，协议兼容性由用户自行判断）
	return candidate, nil, nil
}
