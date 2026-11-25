package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMergeClaudeEnv 覆盖 overlay 语义：同 key 覆盖、settings 独有 key 追加、
// base 原有顺序保留、无等号片段原样保留、空 overlay 直接返回 base。
func TestMergeClaudeEnv(t *testing.T) {
	base := []string{"PATH=/usr/bin", "ANTHROPIC_MODEL=old", "NO_EQUALS", "ANTHROPIC_BASE_URL=https://old"}
	overlay := map[string]string{
		"ANTHROPIC_MODEL":      "new-model",
		"ANTHROPIC_AUTH_TOKEN": "fixture-token",
	}
	got := mergeClaudeEnv(base, overlay)

	want := map[string]string{
		"PATH":                 "/usr/bin",
		"ANTHROPIC_MODEL":      "new-model",
		"ANTHROPIC_BASE_URL":   "https://old",
		"ANTHROPIC_AUTH_TOKEN": "fixture-token",
	}
	seen := make(map[string]string, len(got))
	for _, kv := range got {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			if kv != "NO_EQUALS" {
				t.Errorf("无等号片段应原样保留, got %q", kv)
			}
			continue
		}
		seen[k] = v
	}
	for k, v := range want {
		if seen[k] != v {
			t.Errorf("env %q = %q, want %q", k, seen[k], v)
		}
	}
	// base 原有顺序保持：PATH 仍在 ANTHROPIC_MODEL 之前，覆盖项原位不后移。
	if len(got) < 2 || got[0] != "PATH=/usr/bin" || got[1] != "ANTHROPIC_MODEL=new-model" {
		t.Errorf("base 顺序应保留且覆盖项原位, got %v", got)
	}
	// 空 overlay 原样返回（同一底层切片即可）。
	if out := mergeClaudeEnv(base, nil); len(out) != len(base) {
		t.Errorf("空 overlay 应原样返回, got %v", out)
	}
}

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

// TestReadClaudeUserSettings 表驱动：原样返回 settings.model 与 settings.env，
// 缺失/损坏容错回 ("", nil)（供默认模型回显与集成测试环境注入两条链路复用）。
func TestReadClaudeUserSettings(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		wantModel    string
		wantEnv      map[string]string
	}{
		{
			name: "完整 env 段与 model 字段",
			settingsJSON: `{"model":"opus[1m]","env":{` +
				`"ANTHROPIC_MODEL":"glm-5.3-flash","ANTHROPIC_BASE_URL":"https://ark.example.com",` +
				`"ANTHROPIC_AUTH_TOKEN":"fixture-token"}}`,
			wantModel: "opus[1m]",
			wantEnv: map[string]string{
				"ANTHROPIC_MODEL":      "glm-5.3-flash",
				"ANTHROPIC_BASE_URL":   "https://ark.example.com",
				"ANTHROPIC_AUTH_TOKEN": "fixture-token",
			},
		},
		{
			name:         "仅 model 字段无 env",
			settingsJSON: `{"model":"sonnet"}`,
			wantModel:    "sonnet",
			wantEnv:      nil,
		},
		{
			name:         "仅 env 无 model",
			settingsJSON: `{"env":{"OTHER":"x"}}`,
			wantModel:    "",
			wantEnv:      map[string]string{"OTHER": "x"},
		},
		{
			name:         "文件不存在回零值",
			settingsJSON: "",
			wantModel:    "",
			wantEnv:      nil,
		},
		{
			name:         "损坏 JSON 容错回零值",
			settingsJSON: "{not-valid-json",
			wantModel:    "",
			wantEnv:      nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withFakeHome(t, tc.settingsJSON)
			gotModel, gotEnv := readClaudeUserSettings()
			if gotModel != tc.wantModel {
				t.Errorf("readClaudeUserSettings() model = %q, want %q", gotModel, tc.wantModel)
			}
			if len(gotEnv) != len(tc.wantEnv) {
				t.Fatalf("readClaudeUserSettings() env = %v, want %v", gotEnv, tc.wantEnv)
			}
			for k, want := range tc.wantEnv {
				if got := gotEnv[k]; got != want {
					t.Errorf("env[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}
