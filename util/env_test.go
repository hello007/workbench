package util

import (
	"reflect"
	"testing"
)

// ===== IsSensitiveEnvName 敏感命名判定（无 tag，双平台同测） =====

func TestIsSensitiveEnvName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"TOKEN", true},                 // 全名精确命中
		{"GITHUB_TOKEN", true},          // 前缀变体
		{"token", true},                 // 小写（大小写不敏感）
		{"Token", true},                 // 混合大小写
		{"MINIO_SECRET_KEY", true},      // 中缀
		{"MY_PASSWORD", true},           // PASSWORD 前缀
		{"APP_ADMIN_PASSWORD", true},    // PASSWORD 中缀
		{"GIT_HTTP_PASSWD", true},       // PASSWD 变体
		{"SSH_KEY", true},               // KEY 后缀
		{"GIT_CREDENTIAL_HELPER", true}, // CREDENTIAL
		{"aws_secret_access_key", true}, // 全小写多模式命中
		{"PATH", false},
		{"HOME", false},
		{"JAVA_HOME", false},
		{"PROCESSOR_ARCHITECTURE", false},
		{"", false},
		// 已接受的误伤（子串包含模式的代价，PRD 决策 2026-10-13）：MONKEY_TEST
		// 命中 KEY。前端可点击临时显值不影响使用，方向安全（漏报比误伤危害大）。
		{"MONKEY_TEST", true},
	}
	for _, c := range cases {
		if got := IsSensitiveEnvName(c.name); got != c.want {
			t.Errorf("IsSensitiveEnvName(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// ===== ProcessEnv / parseEnvEntries =====

func TestParseEnvEntries(t *testing.T) {
	got := parseEnvEntries([]string{
		"PATH=/usr/local/bin",
		"NO_EQUALS_SIGN",          // 无 "=" 的异常行：跳过
		"=C:=C:\\hidden",          // Windows 隐藏盘符条目（名称为空）：跳过
		"GLM_API_KEY=abc=def=ghi", // 值含 "="：按首个 "=" 切分
		"EMPTY_VAR=",              // 空值：合法条目，保留
		"DUP=first",
		"DUP=second", // 同名重复：保留首次出现（与 os.Getenv 语义一致）
	})
	want := []EnvEntry{
		{Name: "PATH", Value: "/usr/local/bin"},
		{Name: "GLM_API_KEY", Value: "abc=def=ghi"},
		{Name: "EMPTY_VAR", Value: ""},
		{Name: "DUP", Value: "first"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseEnvEntries 不符:\n got %+v\nwant %+v", got, want)
	}
}

func TestParseEnvEntries_Empty(t *testing.T) {
	if got := parseEnvEntries(nil); len(got) != 0 {
		t.Fatalf("空入参应返回空列表: got %+v", got)
	}
}

// TestProcessEnv_ContainsInjectedVar 只断言本测试自注入的变量存在且值正确，不断言
// 本机真实环境变量内容（docs/spec/test-stability.md：环境外部状态禁依赖）。
func TestProcessEnv_ContainsInjectedVar(t *testing.T) {
	t.Setenv("WORCH_ENV_VIEWER_TEST_VAR", "fixture-value")

	entries, err := ProcessEnv()
	if err != nil {
		t.Fatalf("ProcessEnv 不应报错: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "WORCH_ENV_VIEWER_TEST_VAR" {
			found = true
			if e.Value != "fixture-value" {
				t.Errorf("注入变量值不符: got %q, want %q", e.Value, "fixture-value")
			}
		}
	}
	if !found {
		t.Error("ProcessEnv 应包含 os.Setenv 注入的变量")
	}
}
