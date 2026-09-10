package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoadJSON_Success 正常 JSON 文件反序列化成功。
func TestLoadJSON_Success(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "data.json")
	want := map[string]int{"a": 1, "b": 2}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var got map[string]int
	if err := LoadJSON(p, &got); err != nil {
		t.Fatalf("LoadJSON: %v", err)
	}
	if got["a"] != 1 || got["b"] != 2 {
		t.Errorf("LoadJSON result mismatch: %v", got)
	}
}

// TestLoadJSON_NotExists 文件不存在返回错误。
func TestLoadJSON_NotExists(t *testing.T) {
	var got map[string]int
	if err := LoadJSON(filepath.Join(t.TempDir(), "missing.json"), &got); err == nil {
		t.Error("文件不存在应返回错误")
	}
}

// TestLoadJSON_InvalidJSON 非法 JSON 返回错误。
func TestLoadJSON_InvalidJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	var got map[string]int
	if err := LoadJSON(p, &got); err == nil {
		t.Error("非法 JSON 应返回错误")
	}
}

// TestSaveJSON_Success 保存后内容可被正确反序列化。
func TestSaveJSON_Success(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.json")
	want := map[string]string{"k": "v"}
	if err := SaveJSON(p, want); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	var got map[string]string
	if err := LoadJSON(p, &got); err != nil {
		t.Fatalf("LoadJSON after save: %v", err)
	}
	if got["k"] != "v" {
		t.Errorf("round-trip mismatch: %v", got)
	}
}

// TestSaveJSON_CreatesNestedDir 目标目录不存在时自动创建。
func TestSaveJSON_CreatesNestedDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "deep", "out.json")
	if err := SaveJSON(p, map[string]int{"x": 1}); err != nil {
		t.Fatalf("SaveJSON nested: %v", err)
	}
	if !FileExists(p) {
		t.Error("嵌套目录文件应被创建")
	}
}

// TestFileExists_Exists 文件存在返回 true。
func TestFileExists_Exists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "exists.txt")
	os.WriteFile(p, []byte("x"), 0o644)
	if !FileExists(p) {
		t.Error("存在文件应返回 true")
	}
}

// TestFileExists_NotExists 文件不存在返回 false。
func TestFileExists_NotExists(t *testing.T) {
	if FileExists(filepath.Join(t.TempDir(), "missing.txt")) {
		t.Error("不存在文件应返回 false")
	}
}
