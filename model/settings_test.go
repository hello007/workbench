package model

import (
	"encoding/json"
	"testing"
)

func TestAppSettings_DefaultValues(t *testing.T) {
	s := &AppSettings{}
	if s.GpuDisabled != false {
		t.Error("GpuDisabled 默认应为 false")
	}
	if s.DefaultShell != "" {
		t.Error("DefaultShell 默认应为空")
	}
	if s.GitBashPath != "" {
		t.Error("GitBashPath 默认应为空")
	}
	if s.WslDistro != "" {
		t.Error("WslDistro 默认应为空")
	}
}

func TestAppSettings_JSONSerialization(t *testing.T) {
	s := &AppSettings{
		GpuDisabled:  true,
		DefaultShell: "gitbash",
		GitBashPath:  "D:\\custom\\bash.exe",
		WslDistro:    "Ubuntu",
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	var decoded AppSettings
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if decoded.DefaultShell != "gitbash" {
		t.Errorf("DefaultShell: 期望 gitbash, 实际=%s", decoded.DefaultShell)
	}
	if decoded.GitBashPath != "D:\\custom\\bash.exe" {
		t.Errorf("GitBashPath: 期望 D:\\custom\\bash.exe, 实际=%s", decoded.GitBashPath)
	}
	if decoded.WslDistro != "Ubuntu" {
		t.Errorf("WslDistro: 期望 Ubuntu, 实际=%s", decoded.WslDistro)
	}
	if !decoded.GpuDisabled {
		t.Error("GpuDisabled 应为 true")
	}
}

func TestAppSettings_JSONDeserialization_Partial(t *testing.T) {
	// 测试只有部分字段的 JSON（向后兼容）
	jsonStr := `{"gpuDisabled":true}`
	var s AppSettings
	if err := json.Unmarshal([]byte(jsonStr), &s); err != nil {
		t.Fatalf("部分字段反序列化失败: %v", err)
	}
	if !s.GpuDisabled {
		t.Error("GpuDisabled 应为 true")
	}
	if s.DefaultShell != "" {
		t.Error("缺失字段应为零值")
	}
}
