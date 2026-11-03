package service

import (
	"os/exec"
	"strings"

	"workbench/model"
)

// 本文件承载 AI 对话启动命令的 PATH 发现扫描（启动命令下拉候选数据源）。
// 纯只读：不写文件、不进 ChatService 状态，App 绑定层薄委托直接调用。

// chatCliKnownNames 已知 AI CLI 命令名集合（claude 兼容或类协议 CLI 的常见命名）。
// 白名单口径：PATH 扫描只认这些名，防无关同名命令（如用户自建 ccr）误报；
// 后续支持新 CLI 在此追加即可（表驱动便于增删）。测试可临时替换。
var chatCliKnownNames = []string{
	"claude",
	"claude-code-router",
	"ccr",
	"qwen",
	"iflow",
	"crush",
	"opencode",
}

// DiscoverChatClis 扫描 PATH 上的已知 AI CLI 候选，返回 {name, path} 列表。
// 对每个已知命令名 exec.LookPath，命中收录；不同名命中同一路径（嵌套 shim
// 指向同一 exe）按解析后的真实目标去重，保留首个命中名（即集合声明序优先）。
// 找不到返回空列表不报错（Windows PATHEXT 语义由 Go LookPath 处理）。
func DiscoverChatClis() []model.ChatCliInfo {
	seen := make(map[string]struct{})
	out := make([]model.ChatCliInfo, 0)
	for _, name := range chatCliKnownNames {
		path, err := exec.LookPath(name)
		if err != nil || path == "" {
			continue
		}
		// 去重键优先取 shim 解析后的真实启动目标（嵌套 shim 指向同一 exe 视为
		// 同项）；解析失败退回 LookPath 原路径。统一小写比较（Windows 路径
		// 大小写不敏感；POSIX 侧极端同路径异大小写场景代价仅为多去重一项，可接受）
		// js 入口形态：真实目标是 node.exe + cli.js 前缀参数——键须拼上 js 路径，
		// 否则同一 node.exe 启动的不同 js CLI（如 claude 与 ccr 的 js 安装形态）
		// 会被误判同项去重丢候选
		key := path
		if target, prefix, ok := resolveFromPath(path); ok && target != "" {
			key = strings.Join(append(append([]string{}, prefix...), target), "\x00")
		}
		key = strings.ToLower(key)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, model.ChatCliInfo{Name: name, Path: path})
	}
	return out
}
