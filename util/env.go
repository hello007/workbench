package util

import (
	"errors"
	"os"
	"strings"
)

// ErrEnvSourceUnsupported 当前平台不支持该来源的环境变量枚举（Linux 无注册表，
// 环境变量没有「系统级 / 用户级」持久层概念）。
//
// service 层据此哨兵错误跳过对应分组（Linux UI 仅显示「进程」一级），不走错误提示；
// 与真实失败（读注册表出错等）区分开，后者写入分组 Error 字段对用户可见。
var ErrEnvSourceUnsupported = errors.New("当前平台不支持该来源的环境变量枚举")

// EnvEntry 环境变量条目（名称 + 值）。值恒为真实值，脱敏是展示层职责
// （util.IsSensitiveEnvName 供 service 层打 Sensitive 标记）。
type EnvEntry struct {
	Name  string
	Value string
}

// EnvSourceProvider 环境变量来源枚举能力抽象。
//
// 生产实现为 util 包平台分叉函数（Windows 读注册表区分系统/用户级 + os.Environ
// 进程级；Linux 仅进程级），由 service 层适配转发；单测注入 fake——本机真实环境
// 变量与注册表内容属环境外部状态，依赖其内容的断言必然 flaky
// （docs/spec/test-stability.md）。
type EnvSourceProvider interface {
	// SystemEnv 枚举系统级环境变量（Windows 注册表 HKLM；Linux 返回 ErrEnvSourceUnsupported）
	SystemEnv() ([]EnvEntry, error)
	// UserEnv 枚举用户级环境变量（Windows 注册表 HKCU；Linux 返回 ErrEnvSourceUnsupported）
	UserEnv() ([]EnvEntry, error)
	// ProcessEnv 枚举进程级环境变量（全平台可用）
	ProcessEnv() ([]EnvEntry, error)
}

// ProcessEnv 枚举当前进程实际生效的环境变量（os.Environ()，全平台共享实现）。
//
// Windows / Linux 语义一致：返回父进程注入合并后的实际生效值（Windows 含系统级、
// 用户级启动时合并的结果），与终端里 `env` / `set` 看到的口径一致。故 Windows 的
// 三级来源无需再对进程级做平台分叉。
func ProcessEnv() ([]EnvEntry, error) {
	return parseEnvEntries(os.Environ()), nil
}

// parseEnvEntries 解析 os.Environ() 形态的 "NAME=VALUE" 切片为条目列表（纯函数，单测入口）。
//
//   - 按首个 "=" 切分：值本身可含 "="（如 PROMPT=$P$G）
//   - 无 "=" 的行与名称为空的行（Windows 环境块中的隐藏盘符条目 "=C:=..."）跳过
//   - 同名重复保留首次出现（与 os.Getenv 取首个的语义一致；正常环境块内键唯一，
//     Linux 环境块理论上允许重复，防御性去重避免展示列表出现同名两行）
func parseEnvEntries(environ []string) []EnvEntry {
	entries := make([]EnvEntry, 0, len(environ))
	seen := make(map[string]bool, len(environ))
	for _, line := range environ {
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue // 无 "=" 或名称为空（含 "=C:=..." 隐藏盘符条目）
		}
		name := line[:idx]
		if seen[name] {
			continue
		}
		seen[name] = true
		entries = append(entries, EnvEntry{Name: name, Value: line[idx+1:]})
	}
	return entries
}

// sensitiveEnvNamePatterns 敏感环境变量名模式（大小写不敏感子串包含即判定敏感）。
// 取值经用户确认（2026-10-13）：TOKEN / SECRET / PASSWORD / PASSWD / KEY / CREDENTIAL。
var sensitiveEnvNamePatterns = []string{
	"TOKEN", "SECRET", "PASSWORD", "PASSWD", "KEY", "CREDENTIAL",
}

// IsSensitiveEnvName 判定环境变量名是否命中敏感命名模式（大小写不敏感子串包含）。
//
// 子串包含而非全名精确匹配：命名变体（GITHUB_TOKEN / MINIO_SECRET_KEY /
// GIT_HTTP_PASSWD 等）无需维护枚举表。代价是少数误伤（如 MONKEY_TEST 命中 KEY），
// 前端可点击临时显值不影响使用，漏报（真实密钥明文展示）比误伤（多打一次码）
// 危害大，方向安全。
func IsSensitiveEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, pattern := range sensitiveEnvNamePatterns {
		if strings.Contains(upper, pattern) {
			return true
		}
	}
	return false
}
