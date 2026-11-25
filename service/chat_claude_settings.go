package service

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// mergeClaudeEnv 把 settings env 覆盖到基础环境变量切片上（同 key settings 为准）。
// 输出保留 base 原有顺序；settings 独有的 key 追加在末尾。Windows 环境变量大小写不敏感，
// 但 settings 键与 os.Environ 实际键均为大写形态，按精确 key 匹配即可。
// 生产代码零调用：为集成测试注入用户级 claude settings.json 的 env 段而保留，
// 放在非 integration 构建文件以便常规单测覆盖其覆盖/追加/保序语义。
func mergeClaudeEnv(base []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overlay))
	overridden := make(map[string]bool, len(overlay))
	for _, kv := range base {
		k, _, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		if v, hit := overlay[k]; hit {
			out = append(out, k+"="+v)
			overridden[k] = true
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overlay {
		if !overridden[k] {
			out = append(out, k+"="+v)
		}
	}
	return out
}

// readClaudeUserSettings 读取用户级 claude 设置文件（%USERPROFILE%/.claude/settings.json），
// 返回 settings.model 字段与 settings.env 段（原样字面值，不做取值决策）。
//
// 只读容错（对齐 GetClaudeDefaultModel 历史语义）：文件不存在/读取失败/解析失败
// 一律返回 ("", nil) 并留痕日志，不返回 error——调用方（默认模型回显、集成测试
// 环境注入）均按「有则用、无则降级」消费，设置文件缺失不构成错误。
//
// 调用方注意：env 可能为 nil，取值前须判空（读 nil map 取零值安全）。
func readClaudeUserSettings() (modelField string, env map[string]string) {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("读取 claude 用户设置失败（无法定位用户目录）", "err", err)
		return "", nil
	}
	raw, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		// 文件不存在是常态（未自定义过 claude 设置），debug 级即可；其他读取错误 warn
		if os.IsNotExist(err) {
			slog.Debug("claude settings.json 不存在，用户级设置未知")
		} else {
			slog.Warn("读取 claude settings.json 失败", "err", err)
		}
		return "", nil
	}
	var cfg struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		slog.Warn("解析 claude settings.json 失败，用户级设置未知", "err", err)
		return "", nil
	}
	return cfg.Model, cfg.Env
}

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
	modelField, env := readClaudeUserSettings()
	// env.ANTHROPIC_MODEL 优先于 model 字段（CLI 决策链中环境变量覆盖 settings.model）
	if m := env["ANTHROPIC_MODEL"]; m != "" {
		return m
	}
	return modelField
}
