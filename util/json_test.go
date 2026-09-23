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

// TestSaveJSON_OverwriteExisting 已存在目标的覆盖语义锚定：Windows 下 os.Rename 走
// MoveFileEx(MOVEFILE_REPLACE_EXISTING) 原子替换（util.SaveJSON 原子写的前提行为）。
func TestSaveJSON_OverwriteExisting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out.json")
	if err := os.WriteFile(p, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := SaveJSON(p, map[string]int{"new": 2}); err != nil {
		t.Fatalf("SaveJSON overwrite: %v", err)
	}
	var got map[string]int
	if err := LoadJSON(p, &got); err != nil {
		t.Fatalf("LoadJSON after overwrite: %v", err)
	}
	if len(got) != 1 || got["new"] != 2 {
		t.Errorf("覆盖后应为全新内容, 实际=%v", got)
	}
}

// TestSaveJSON_StaleTempResidueDoesNotBreakTarget 模拟崩溃中断残留：预先植入同模式
// 临时文件（点前缀隐藏名），再次保存不受影响，目标文件内容正确且无临时文件被误认为目标。
// 残留文件按设计不做自动清扫（并发保存下清扫他方在写临时文件会误删），但其为隐藏点文件，
// 不被精确路径读取，属无害残留。
func TestSaveJSON_StaleTempResidueDoesNotBreakTarget(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	stale := filepath.Join(dir, ".out.json.tmp-9999")
	if err := os.WriteFile(stale, []byte(`{"crashed":true}`), 0o644); err != nil {
		t.Fatalf("plant stale temp: %v", err)
	}
	if err := SaveJSON(p, map[string]int{"ok": 1}); err != nil {
		t.Fatalf("SaveJSON with stale residue: %v", err)
	}
	var got map[string]int
	if err := LoadJSON(p, &got); err != nil {
		t.Fatalf("stale 残留下目标损坏: %v", err)
	}
	if got["ok"] != 1 {
		t.Errorf("目标内容错误: %v", got)
	}
	// 残留临时文件不应被当作目标消费（内容仍是崩溃半成品，属设计内无害残留）
	if raw, err := os.ReadFile(stale); err != nil || string(raw) != `{"crashed":true}` {
		t.Errorf("残留临时文件不应被改动: raw=%q err=%v", raw, err)
	}
}

// TestSaveJSON_NoTempResidueOnSuccess 成功保存后同目录无临时文件残留。
func TestSaveJSON_NoTempResidueOnSuccess(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	if err := SaveJSON(p, map[string]int{"x": 1}); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".*.tmp-*"))
	if len(matches) != 0 {
		t.Errorf("成功保存不应残留临时文件: %v", matches)
	}
}

// TestSaveJSON_MarshalFailureTargetIntact 序列化失败（不可 JSON 化的值）时目标文件
// 保持原内容不动——原子写失败路径不触碰已有数据。
func TestSaveJSON_MarshalFailureTargetIntact(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	if err := os.WriteFile(p, []byte(`{"keep":true}`), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := SaveJSON(p, map[string]chan int{"bad": nil}); err == nil {
		t.Fatal("不可序列化值应返回错误")
	}
	raw, err := os.ReadFile(p)
	if err != nil || string(raw) != `{"keep":true}` {
		t.Errorf("失败路径目标文件被改动: raw=%q err=%v", raw, err)
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
