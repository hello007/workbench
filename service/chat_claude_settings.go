package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
)

// GetClaudeDefaultModel 读取本机 claude 设置文件解析 CLI 默认模型，
// 供前端模型下拉「默认」选项回显真实默认值（空 = 无法确定，前端回退通用文案）。
//
// 解析优先级对齐 claude CLI 的模型决策链（CLI --model 参数 > ANTHROPIC_MODEL
// 环境变量 > settings.json model 字段）：本绑定只读设置文件，前两层中 CLI 参数
// 不在文件里（前端 modelName 为空即用户未覆盖 = 用该默认），故文件内取
// env.ANTHROPIC_MODEL 优先、model 字段兜底。
//
// 只读容错：文件不存在/损坏/字段缺失一律返回空串（slog.Warn 留痕），不阻塞调用方。
// 文件路径：%USERPROFILE%/.claude/settings.json（claude 官方用户级设置）。
func GetClaudeDefaultModel() string {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("读取 claude 默认模型失败（无法定位用户目录）", "err", err)
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		// 文件不存在是常态（未自定义过 claude 设置），debug 级即可；其他读取错误 warn
		if os.IsNotExist(err) {
			slog.Debug("claude settings.json 不存在，默认模型未知")
		} else {
			slog.Warn("读取 claude settings.json 失败", "err", err)
		}
		return ""
	}
	var cfg struct {
		Model string           `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		slog.Warn("解析 claude settings.json 失败，默认模型未知", "err", err)
		return ""
	}
	// env.ANTHROPIC_MODEL 优先于 model 字段（CLI 决策链中环境变量覆盖 settings.model）
	if m := cfg.Env["ANTHROPIC_MODEL"]; m != "" {
		return m
	}
	return cfg.Model
}
