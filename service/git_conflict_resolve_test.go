package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// isSafeRelativePath 纯函数表测：合法仓库相对路径通过，绝对路径/../卷名/冗余分隔符拒绝。
func TestIsSafeRelativePath(t *testing.T) {
	cases := []struct {
		file string
		want bool
	}{
		{"a.txt", true},
		{"dir/a.txt", true},
		{"dir/sub/a.txt", true},
		{"", false},
		{"/etc/passwd", false},
		{`\etc\passwd`, false},
		{"../a.txt", false},
		{"dir/../../a.txt", false},
		{"a..b.txt", false}, // 宁严勿漏：任何 .. 片段一律拒绝
		{`C:\temp\a.txt`, false},
		{"dir//a.txt", false},
		{"dir/./a.txt", false},
		{"dir/", false},
		{".", false},
	}
	for _, c := range cases {
		if got := isSafeRelativePath(c.file); got != c.want {
			t.Errorf("isSafeRelativePath(%q) = %v, want %v", c.file, got, c.want)
		}
	}
}

// TestChooseConflictSide_InvalidSide side 非 ours/theirs 拒绝（中文错误）。
func TestChooseConflictSide_InvalidSide(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	err := svc.ChooseConflictSide(dir, "a.txt", "base")
	if err == nil {
		t.Fatal("非法 side 应报错")
	}
	if !strings.Contains(err.Error(), "ours") {
		t.Errorf("错误应说明仅支持 ours/theirs, got %v", err)
	}
}

// TestChooseConflictSide_EmptyFile 文件路径空报错。
func TestChooseConflictSide_EmptyFile(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	if err := svc.ChooseConflictSide(dir, "", "ours"); err == nil {
		t.Error("空文件路径应报错")
	}
}

// TestChooseConflictSide_NotInConflictList 文件不在冲突列表时拒绝。
func TestChooseConflictSide_NotInConflictList(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	err := svc.ChooseConflictSide(dir, "b.txt", "ours")
	if err == nil {
		t.Fatal("非冲突文件应报错")
	}
	if !strings.Contains(err.Error(), "不在当前冲突列表中") {
		t.Errorf("错误应提示不在冲突列表, got %v", err)
	}
}

// TestChooseConflictSide_NoConflict 无冲突态时拒绝。
func TestChooseConflictSide_NoConflict(t *testing.T) {
	dir := testutil.SetupFFRepo(t)
	svc := NewGitService()
	err := svc.ChooseConflictSide(dir, "a.txt", "ours")
	if err == nil {
		t.Fatal("无冲突态应报错")
	}
	if !strings.Contains(err.Error(), "无进行中") {
		t.Errorf("错误应提示无进行中冲突, got %v", err)
	}
}

// TestChooseConflictSide 选边成功：工作区变为所选侧内容且退出冲突列表。
func TestChooseConflictSide(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)

	if err := svc.ChooseConflictSide(dir, "a.txt", "ours"); err != nil {
		t.Fatalf("ChooseConflictSide ours: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if want := "line1\nmaster-line2\nline3\n"; normalizeCRLF(string(data)) != want {
		t.Errorf("选 ours 后内容应为 master 版本, got %q", data)
	}
	state, _ := svc.GetConflictState(dir)
	if len(state.Files) != 0 {
		t.Errorf("选边+add 后冲突列表应空, got %v", state.Files)
	}

	// theirs 侧在新的冲突上验证
	dir2 := testutil.SetupConflictRepo(t)
	svc2 := NewGitService()
	svc2.Merge(dir2, "feature", model.MergeModeFF)
	if err := svc2.ChooseConflictSide(dir2, "a.txt", "theirs"); err != nil {
		t.Fatalf("ChooseConflictSide theirs: %v", err)
	}
	data2, _ := os.ReadFile(filepath.Join(dir2, "a.txt"))
	if want := "line1\nfeature-line2\nline3\n"; normalizeCRLF(string(data2)) != want {
		t.Errorf("选 theirs 后内容应为 feature 版本, got %q", data2)
	}
	state2, _ := svc2.GetConflictState(dir2)
	if len(state2.Files) != 0 {
		t.Errorf("选边+add 后冲突列表应空, got %v", state2.Files)
	}
}

// TestGetConflictFileContent_NotInConflictList 非冲突文件/非法路径拒绝。
func TestGetConflictFileContent_Rejects(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)

	if _, err := svc.GetConflictFileContent(dir, "b.txt"); err == nil {
		t.Error("非冲突文件应报错")
	}
	if _, err := svc.GetConflictFileContent(dir, "../escape.txt"); err == nil {
		t.Error("路径逃逸应报错")
	}
	if _, err := svc.GetConflictFileContent(dir, ""); err == nil {
		t.Error("空路径应报错")
	}
	// 无冲突态仓库
	clean := testutil.SetupFFRepo(t)
	if _, err := svc.GetConflictFileContent(clean, "a.txt"); err == nil {
		t.Error("无冲突态应报错")
	}
}

// TestGetConflictFileContent_UTF8MarkerContent 返回含冲突标记的 UTF-8 内容。
func TestGetConflictFileContent_UTF8MarkerContent(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)

	result, err := svc.GetConflictFileContent(dir, "a.txt")
	if err != nil {
		t.Fatalf("GetConflictFileContent: %v", err)
	}
	if result.Binary || result.TooLarge {
		t.Errorf("文本冲突文件不应 binary/tooLarge: %+v", result)
	}
	if result.Encoding != "utf-8" {
		t.Errorf("编码应为 utf-8, got %s", result.Encoding)
	}
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(result.Content, marker) {
			t.Errorf("内容应含冲突标记 %s, got %q", marker, result.Content)
		}
	}
}

// TestSaveConflictResolution_BinaryAndTooLarge 写入路径对二进制/超大不拦截（仅读取侧拦截），
// 此处验证非法路径与空文件名拒绝。
func TestSaveConflictResolution_Rejects(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)

	if err := svc.SaveConflictResolution(dir, "", "x", "utf-8"); err == nil {
		t.Error("空文件路径应报错")
	}
	if err := svc.SaveConflictResolution(dir, "../escape.txt", "x", "utf-8"); err == nil {
		t.Error("路径逃逸应报错")
	}
	if err := svc.SaveConflictResolution(dir, "b.txt", "x", "utf-8"); err == nil {
		t.Error("非冲突文件应报错")
	}
	clean := testutil.SetupFFRepo(t)
	if err := svc.SaveConflictResolution(clean, "a.txt", "x", "utf-8"); err == nil {
		t.Error("无冲突态应报错")
	}
}

// normalizeCRLF 统一 CRLF 为 LF 后比对（Windows autocrlf 环境下 git checkout/add
// 会转换行尾，内容语义比对不受行尾影响）。
func normalizeCRLF(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// TestSaveConflictResolution 保存内容写盘并退出冲突列表。
func TestSaveConflictResolution(t *testing.T) {
	dir := testutil.SetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)

	resolved := "line1\nresolved\nline3\n"
	if err := svc.SaveConflictResolution(dir, "a.txt", resolved, "utf-8"); err != nil {
		t.Fatalf("SaveConflictResolution: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if normalizeCRLF(string(data)) != resolved {
		t.Errorf("写盘内容应原样, got %q", data)
	}
	state, _ := svc.GetConflictState(dir)
	if len(state.Files) != 0 {
		t.Errorf("保存+add 后冲突列表应空, got %v", state.Files)
	}
}
