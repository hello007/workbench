//go:build windows

package util

import (
	"golang.org/x/sys/windows/registry"

	"workbench/model"
)

// Windows 环境变量注册表键路径（决策 2026-10-13：系统级 / 用户级读注册表区分来源，
// REG_EXPAND_SZ 读取时展开与进程级展示口径一致）。
const (
	// windowsSystemEnvKeyPath 系统级环境变量键（HKLM 下，对所有用户生效）。
	windowsSystemEnvKeyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	// windowsUserEnvKeyPath 用户级环境变量键（HKCU 下，仅当前用户生效）。
	windowsUserEnvKeyPath = `Environment`
)

// envRegValue 注册表单值的原始形态（值名 + 原始字符串 + 值类型）。
// REG_EXPAND_SZ 保留原始值，由 readEnvRegEntries 统一展开。
type envRegValue struct {
	name  string
	value string
	typ   uint32
}

// envRegStore 注册表读取抽象（单测注入点）。
//
// 生产实现走 golang.org/x/sys/windows/registry；单测注入 fake——真实注册表内容随
// 本机环境变化，依赖真实注册表内容的断言属环境外部状态、必然 flaky
// （docs/spec/test-stability.md）。分叉与注入模式对齐 util/process_windows.go。
type envRegStore interface {
	// readKey 读取 root 键下 subKey 路径的全部字符串值：仅返回 REG_SZ / REG_EXPAND_SZ
	//（含原始值与类型）；非字符串类型值（Environment 键可能残留 DWORD 等无效项）跳过。
	readKey(root registry.Key, subKey string) ([]envRegValue, error)
}

// windowsEnvRegStore 生产实现，转发到 x/sys/windows/registry。
type windowsEnvRegStore struct{}

func (windowsEnvRegStore) readKey(root registry.Key, subKey string) ([]envRegValue, error) {
	k, err := registry.OpenKey(root, subKey, registry.QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, err
	}
	values := make([]envRegValue, 0, len(names))
	for _, name := range names {
		raw, typ, err := k.GetStringValue(name)
		if err != nil {
			// 非字符串类型与读取异常值跳过：GetStringValue 仅识别 REG_SZ / REG_EXPAND_SZ
			//（REG_MULTI_SZ / REG_DWORD 等均返回错误），环境变量展示只关心字符串值，
			// 不因单值失败拖垮整个来源
			continue
		}
		values = append(values, envRegValue{name: name, value: raw, typ: typ})
	}
	return values, nil
}

// SystemEnv 枚举系统级环境变量（注册表 HKLM\SYSTEM\CurrentControlSet\Control\Session
// Manager\Environment）。
//
// REG_EXPAND_SZ 读取时经 registry.ExpandString 展开（%VAR% 还原为实际值）：
// 进程级拿到的是系统启动时已展开合并后的值，不展开会造成两级展示口径不一致。
func SystemEnv() ([]EnvEntry, error) {
	return systemEnvWithStore(windowsEnvRegStore{})
}

// systemEnvWithStore 系统级枚举编排（fake store 注入点）。
func systemEnvWithStore(store envRegStore) ([]EnvEntry, error) {
	entries, err := readEnvRegEntries(store, registry.LOCAL_MACHINE, windowsSystemEnvKeyPath)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeEnvEnumFailed, "系统环境变量读取失败，请重试", err)
	}
	return entries, nil
}

// UserEnv 枚举用户级环境变量（注册表 HKCU\Environment），展开规则同 SystemEnv。
func UserEnv() ([]EnvEntry, error) {
	return userEnvWithStore(windowsEnvRegStore{})
}

// userEnvWithStore 用户级枚举编排（fake store 注入点）。
func userEnvWithStore(store envRegStore) ([]EnvEntry, error) {
	entries, err := readEnvRegEntries(store, registry.CURRENT_USER, windowsUserEnvKeyPath)
	if err != nil {
		return nil, model.WrapAppError(model.ErrCodeEnvEnumFailed, "用户环境变量读取失败，请重试", err)
	}
	return entries, nil
}

// readEnvRegEntries 读注册表键并转为环境变量条目：REG_SZ 原样保留，REG_EXPAND_SZ
// 经 registry.ExpandString 展开（展开失败保留原始值，不因单值失败拖垮整个来源——
// ExpandEnvironmentStrings 对未定义引用原样保留，失败仅限 API 层异常）。
// 进程级不经此函数：os.Environ 已是展开后的实际生效值（见 util/env.go ProcessEnv）。
func readEnvRegEntries(store envRegStore, root registry.Key, subKey string) ([]EnvEntry, error) {
	values, err := store.readKey(root, subKey)
	if err != nil {
		return nil, err
	}
	entries := make([]EnvEntry, 0, len(values))
	for _, v := range values {
		value := v.value
		if v.typ == registry.EXPAND_SZ {
			if expanded, err := registry.ExpandString(value); err == nil {
				value = expanded
			}
		}
		entries = append(entries, EnvEntry{Name: v.name, Value: value})
	}
	return entries, nil
}
