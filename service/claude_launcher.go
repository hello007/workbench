package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// claudeCommandResolver 可注入的 claude 启动解析函数（测试替换避免依赖真机 shim）。
var claudeCommandResolver = resolveClaudeCommand

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
	ext := strings.ToLower(filepath.Ext(shim))
	if ext != ".cmd" && ext != ".bat" {
		// LookPath 命中真实可执行（原生安装形态），直接用
		return shim, nil
	}
	raw, err := os.ReadFile(shim)
	if err != nil {
		return "claude", nil
	}
	// npm shim 可能有多行候选，取最后一个匹配（末行才是实际执行行）
	matches := claudeShimTargetRe.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		return "claude", nil
	}
	target := matches[len(matches)-1][1]
	shimDir := filepath.Dir(shim)
	// 展开 batch 变量前缀：%dp0% / %~dp0 均展开为 shim 所在目录（尾部带分隔符）
	target = strings.ReplaceAll(target, `%~dp0\`, shimDir+`\`)
	target = strings.ReplaceAll(target, `%dp0%\`, shimDir+`\`)
	// 目标文件必须真实存在，防 shim 内容解析出幽灵路径
	if _, err := os.Stat(target); err != nil {
		return "claude", nil
	}
	if strings.ToLower(filepath.Ext(target)) == ".js" {
		// js 入口：优先 shim 同目录 node.exe（npm/nvm 布局），退而 PATH 上的 node
		nodeExe := filepath.Join(shimDir, "node.exe")
		if _, err := os.Stat(nodeExe); err != nil {
			node, lerr := exec.LookPath("node")
			if lerr != nil {
				return "claude", nil
			}
			nodeExe = node
		}
		return nodeExe, []string{target}
	}
	return target, nil
}
