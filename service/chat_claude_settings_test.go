package service

import (
	"os"
	"path/filepath"
	"testing"
)

// 用环境变量指向伪造的 HOME，使 GetClaudeDefaultModel 读测试夹具。
// os.UserHomeDir 在 Windows 读 %USERPROFILE%。
func withFakeHome(t *testing.T, settingsJSON string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	if settingsJSON != "" {
		dir := filepath.Join(home, ".claude")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir .claude: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settingsJSON), 0o644); err != nil {
			t.Fatalf("写 settings.json: %v", err)
		}
	}
}

// TestGetClaudeDefaultModel 表驱动：解析优先级 env.ANTHROPIC_MODEL > model 字段，
// 缺失/损坏容错回空串（对齐 claude CLI 模型决策链的文件内两层）。
func TestGetClaudeDefaultModel(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		want         string
	}{
		{
			name: "env.ANTHROPIC_MODEL 优先",
			settingsJSON: `{"model":"opus[1m]","env":{` +
				`"ANTHROPIC_MODEL":"glm-5.3-flash","ANTHROPIC_BASE_URL":"https://ark.example.com"}}`,
			want: "glm-5.3-flash",
		},
		{
			name:         "无 env 时回 model 字段",
			settingsJSON: `{"model":"opus[1m]"}`,
			want:         "opus[1m]",
		},
		{
			name:         "env 有对象但无 ANTHROPIC_MODEL 回 model 字段",
			settingsJSON: `{"model":"sonnet","env":{"OTHER":"x"}}`,
			want:         "sonnet",
		},
		{
			name:         "两者皆缺回空串",
			settingsJSON: `{"includeCoAuthoredBy":false}`,
			want:         "",
		},
		{
			name:         "文件不存在回空串",
			settingsJSON: "",
			want:         "",
		},
		{
			name:         "损坏 JSON 容错回空串",
			settingsJSON: "{not-valid-json",
			want:         "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeHome(t, tc.settingsJSON)
			if got := GetClaudeDefaultModel(); got != tc.want {
				t.Errorf("GetClaudeDefaultModel() = %q, want %q", got, tc.want)
			}
		})
	}
}
