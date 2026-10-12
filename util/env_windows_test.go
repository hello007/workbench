//go:build windows

package util

import (
	"errors"
	"reflect"
	"testing"

	"golang.org/x/sys/windows/registry"

	"workbench/model"
)

// fakeEnvRegStore 注册表读取 fake：按 (root, subKey) 返回注入的原始值列表。
// 真实注册表内容随本机环境变化（docs/spec/test-stability.md 禁环境外部状态依赖），
// 枚举断言全部走注入 fixture。
type fakeEnvRegStore struct {
	values map[envRegKeyPath][]envRegValue
	err    error
	calls  []envRegKeyPath
}

// envRegKeyPath fake 的查找键：根键 + 子键路径（registry.Key 为句柄整型，可比较）。
type envRegKeyPath struct {
	root   registry.Key
	subKey string
}

func (f *fakeEnvRegStore) readKey(root registry.Key, subKey string) ([]envRegValue, error) {
	path := envRegKeyPath{root: root, subKey: subKey}
	f.calls = append(f.calls, path)
	if f.err != nil {
		return nil, f.err
	}
	return f.values[path], nil
}

func newFakeEnvRegStore() *fakeEnvRegStore {
	return &fakeEnvRegStore{values: make(map[envRegKeyPath][]envRegValue)}
}

func TestSystemEnv_WithStore(t *testing.T) {
	// 展开验证用测试自注入的进程环境变量（ExpandEnvironmentStrings 展开取当前进程环境），
	// 加未定义引用用例验证「原样保留」——两者均确定性，不依赖本机真实环境变量内容
	t.Setenv("WORCH_ENV_EXPAND_FIXTURE", "expanded-value")
	store := newFakeEnvRegStore()
	store.values[envRegKeyPath{registry.LOCAL_MACHINE, windowsSystemEnvKeyPath}] = []envRegValue{
		{name: "PLAIN", value: "plain-value", typ: registry.SZ},
		{name: "EXPANDED", value: "%WORCH_ENV_EXPAND_FIXTURE%\\bin", typ: registry.EXPAND_SZ},
		{name: "UNDEFINED_REF", value: "%WORCH_ENV_UNDEFINED_FIXTURE%\\keep", typ: registry.EXPAND_SZ},
	}

	entries, err := systemEnvWithStore(store)
	if err != nil {
		t.Fatalf("systemEnvWithStore: %v", err)
	}
	want := []EnvEntry{
		{Name: "PLAIN", Value: "plain-value"},
		{Name: "EXPANDED", Value: "expanded-value\\bin"},                      // REG_EXPAND_SZ 展开为实际值
		{Name: "UNDEFINED_REF", Value: "%WORCH_ENV_UNDEFINED_FIXTURE%\\keep"}, // 未定义引用原样保留
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("系统级条目不符:\n got %+v\nwant %+v", entries, want)
	}
	// 读的是系统级注册表键
	if len(store.calls) != 1 || store.calls[0].root != registry.LOCAL_MACHINE ||
		store.calls[0].subKey != windowsSystemEnvKeyPath {
		t.Fatalf("应读取 HKLM 系统环境键: calls %+v", store.calls)
	}
}

func TestUserEnv_WithStore(t *testing.T) {
	store := newFakeEnvRegStore()
	store.values[envRegKeyPath{registry.CURRENT_USER, windowsUserEnvKeyPath}] = []envRegValue{
		{name: "USER_HOME", value: "C:\\Users\\fixture", typ: registry.SZ},
	}

	entries, err := userEnvWithStore(store)
	if err != nil {
		t.Fatalf("userEnvWithStore: %v", err)
	}
	want := []EnvEntry{{Name: "USER_HOME", Value: "C:\\Users\\fixture"}}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("用户级条目不符:\n got %+v\nwant %+v", entries, want)
	}
	// 读的是用户级注册表键
	if len(store.calls) != 1 || store.calls[0].root != registry.CURRENT_USER ||
		store.calls[0].subKey != windowsUserEnvKeyPath {
		t.Fatalf("应读取 HKCU 用户环境键: calls %+v", store.calls)
	}
}

// TestSystemEnv_StoreError 来源读取失败包为 AppError（用户可读消息 + 根因保留），
// 交由 service 层写入分组 Error 字段（取 Message）或整体失败路径使用。
func TestSystemEnv_StoreError(t *testing.T) {
	store := newFakeEnvRegStore()
	store.err = errors.New("registry access denied")

	_, err := systemEnvWithStore(store)
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("应包装为 *model.AppError，实际 %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeEnvEnumFailed {
		t.Errorf("错误码不符: got %q, want %q", appErr.Code, model.ErrCodeEnvEnumFailed)
	}

	_, err = userEnvWithStore(store)
	if !errors.As(err, &appErr) || appErr.Code != model.ErrCodeEnvEnumFailed {
		t.Fatalf("用户级同构包装 AppError 失败: %v", err)
	}
}

// TestWindowsEnvRegStore_ReadKey_RealRegistryFixture 用测试自建自清理的注册表键验证
// 真实读取实现（OpenKey / ReadValueNames / GetStringValue / 非字符串类型跳过），
// fixture 键属测试自持数据而非本机真实环境变量内容（docs/spec/test-stability.md）。
func TestWindowsEnvRegStore_ReadKey_RealRegistryFixture(t *testing.T) {
	const fixturePath = `Software\WorkbenchTest_EnvVarViewerFixture`
	k, _, err := registry.CreateKey(registry.CURRENT_USER, fixturePath, registry.SET_VALUE)
	if err != nil {
		t.Fatalf("创建 fixture 键失败: %v", err)
	}
	// defer LIFO：先 Close 后 DeleteKey——t.Fatalf 中途退出（如 SetStringValue 失败）时
	// 也释放句柄，否则 Windows 拒绝对持有打开句柄的键删除，fixture 键泄漏在 HKCU
	defer registry.DeleteKey(registry.CURRENT_USER, fixturePath)
	defer k.Close()

	if err := k.SetStringValue("PLAIN", "plain-value"); err != nil {
		t.Fatalf("SetStringValue: %v", err)
	}
	if err := k.SetExpandStringValue("EXPANDED", "%SystemRoot%"); err != nil {
		t.Fatalf("SetExpandStringValue: %v", err)
	}
	if err := k.SetDWordValue("NUMERIC", 42); err != nil {
		t.Fatalf("SetDWordValue: %v", err)
	}

	values, err := windowsEnvRegStore{}.readKey(registry.CURRENT_USER, fixturePath)
	if err != nil {
		t.Fatalf("readKey: %v", err)
	}
	byName := make(map[string]envRegValue, len(values))
	for _, v := range values {
		byName[v.name] = v
	}
	if v, ok := byName["PLAIN"]; !ok || v.value != "plain-value" || v.typ != registry.SZ {
		t.Errorf("REG_SZ 读取不符: %+v", v)
	}
	if v, ok := byName["EXPANDED"]; !ok || v.value != "%SystemRoot%" || v.typ != registry.EXPAND_SZ {
		t.Errorf("REG_EXPAND_SZ 应返回原始值与类型（展开在上层）: %+v", v)
	}
	if _, ok := byName["NUMERIC"]; ok {
		t.Error("非字符串类型（REG_DWORD）应跳过")
	}
}
