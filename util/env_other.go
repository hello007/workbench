//go:build !windows

package util

// SystemEnv 枚举系统级环境变量——非 Windows 平台无此来源，恒返回 ErrEnvSourceUnsupported。
//
// 非 Windows 平台（Linux / darwin 等）无 Windows 注册表，环境变量没有「系统级 / 用户级」
// 持久层概念（shell 配置文件如 /etc/environment、~/.bashrc 不属本功能范围，且随 shell
// 而异）。service 层据 unsupported 哨兵错误静默跳过对应分组（UI 仅显示「进程」一级），
// 不走错误提示。
// 非 Windows 平台断言独立在 env_other_test.go（docs/spec/linux-platform.md §8）。
func SystemEnv() ([]EnvEntry, error) {
	return nil, ErrEnvSourceUnsupported
}

// UserEnv 枚举用户级环境变量——非 Windows 平台无此来源，恒返回 ErrEnvSourceUnsupported。
// 跳过语义同 SystemEnv。
func UserEnv() ([]EnvEntry, error) {
	return nil, ErrEnvSourceUnsupported
}
