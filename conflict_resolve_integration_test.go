//go:build integration

package main

// 内嵌冲突解决（一期）集成测试：经 App 层真实链路驱动三个新命令。
//
// 覆盖：
//   - ChooseConflictSide ours/theirs：工作区变为对应侧内容且退出冲突列表
//   - 文件不在冲突列表 / 无冲突态仓库被拒绝
//   - GetConflictFileContent：UTF-8 标记内容、GBK 解码、二进制、>1MB TooLarge、路径逃逸拒绝
//   - SaveConflictResolution：写盘 + 退出冲突列表；非冲突文件拒绝
//
// 复用 git_flows_integration_test.go 的 it* 系列助手（itNewApp / itSetupConflictRepo 等）。

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"

	"workbench/util/testutil"
)

// itConflictRepos 构造已进入 merge 冲突态的仓库 + App，返回 (repo, app)。
func itConflictRepos(t *testing.T) (string, *App) {
	t.Helper()
	repo := itSetupConflictRepo(t)
	app := itNewApp()
	if _, err := app.Merge(repo, "feature", "ff"); err != nil {
		t.Fatalf("冲突 merge 应被接受为正常返回: %v", err)
	}
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != "merge" || len(state.Files) != 1 || state.Files[0] != "a.txt" {
		t.Fatalf("前置冲突态异常: %+v", state)
	}
	return repo, app
}

// itWriteBytes 在仓库内写原始字节（GBK/二进制 fixture 用）。
func itWriteBytes(t *testing.T, repoPath, rel string, data []byte) {
	t.Helper()
	full := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, data, 0644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// itAssertResolved 断言 a.txt 已退出冲突列表。
func itAssertResolved(t *testing.T, app *App, repo string) {
	t.Helper()
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type == "merge" && len(state.Files) != 0 {
		t.Errorf("冲突列表应空, got %v", state.Files)
	}
}

// TestIntegration_ChooseConflictSide_Ours 选 ours：a.txt 变为 master 侧（:2）内容并标记已解决。
func TestIntegration_ChooseConflictSide_Ours(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	if err := app.ChooseConflictSide(repo, "a.txt", "ours"); err != nil {
		t.Fatalf("ChooseConflictSide ours: %v", err)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "line1\nmaster-line2\nline3\n" {
		t.Errorf("选 ours 后应为 master 版本, got %q", got)
	}
	itAssertResolved(t, app, repo)
}

// TestIntegration_ChooseConflictSide_Theirs 选 theirs：a.txt 变为 feature 侧（:3）内容并标记已解决。
func TestIntegration_ChooseConflictSide_Theirs(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	if err := app.ChooseConflictSide(repo, "a.txt", "theirs"); err != nil {
		t.Fatalf("ChooseConflictSide theirs: %v", err)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "line1\nfeature-line2\nline3\n" {
		t.Errorf("选 theirs 后应为 feature 版本, got %q", got)
	}
	itAssertResolved(t, app, repo)

	// 选边后可继续完成合并（全链路闭环）
	if _, err := app.ContinueMerge(repo); err != nil {
		t.Fatalf("ContinueMerge: %v", err)
	}
}

// TestIntegration_ChooseConflictSide_Rejects 非冲突文件 / 非法 side / 无冲突态仓库拒绝。
func TestIntegration_ChooseConflictSide_Rejects(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	if err := app.ChooseConflictSide(repo, "no-such.txt", "ours"); err == nil {
		t.Error("非冲突文件应拒绝")
	}
	if err := app.ChooseConflictSide(repo, "a.txt", "middle"); err == nil {
		t.Error("非法 side 应拒绝")
	}

	clean := itSetupForkRepo(t)
	if err := app.ChooseConflictSide(clean, "a.txt", "ours"); err == nil {
		t.Error("无冲突态仓库应拒绝")
	}
}

// TestIntegration_GetConflictFileContent_UTF8 返回含冲突标记的 UTF-8 工作区内容。
func TestIntegration_GetConflictFileContent_UTF8(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	result, err := app.GetConflictFileContent(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetConflictFileContent: %v", err)
	}
	if result.Binary || result.TooLarge {
		t.Fatalf("文本冲突文件不应 binary/tooLarge: %+v", result)
	}
	if result.Encoding != "utf-8" {
		t.Errorf("编码应为 utf-8, got %s", result.Encoding)
	}
	for _, marker := range []string{"<<<<<<<", "=======", ">>>>>>>"} {
		if !strings.Contains(result.Content, marker) {
			t.Errorf("内容应含标记 %s, got %q", marker, result.Content)
		}
	}
}

// TestIntegration_GetConflictFileContent_GBK GBK 编码文件解码为 UTF-8 并回报原编码。
func TestIntegration_GetConflictFileContent_GBK(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	// 冲突仍在（index 未变），外部以 GBK 重写工作区文件模拟用户外部编辑
	gbkData, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("line1\n中文解决\nline3\n"))
	if err != nil {
		t.Fatalf("GBK 编码失败: %v", err)
	}
	itWriteBytes(t, repo, "a.txt", gbkData)

	result, err := app.GetConflictFileContent(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetConflictFileContent: %v", err)
	}
	if result.Encoding != "gbk" {
		t.Errorf("编码应为 gbk, got %s", result.Encoding)
	}
	if result.Content != "line1\n中文解决\nline3\n" {
		t.Errorf("GBK 内容解码错误, got %q", result.Content)
	}

	// 按原编码写回后磁盘应为 GBK 字节（R5 编码保持闭环）
	if err := app.SaveConflictResolution(repo, "a.txt", result.Content, result.Encoding); err != nil {
		t.Fatalf("SaveConflictResolution: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "a.txt"))
	if err != nil {
		t.Fatalf("read a.txt: %v", err)
	}
	if string(data) != string(gbkData) {
		t.Errorf("写回应保持 GBK 字节, got %q", data)
	}
	itAssertResolved(t, app, repo)
}

// TestIntegration_GetConflictFileContent_Binary 二进制冲突文件标记 Binary，且可用选边解决（R1/R4）。
func TestIntegration_GetConflictFileContent_Binary(t *testing.T) {
	itRequireGit(t)
	// 独立仓库构造二进制冲突：base 为 ASCII，master/feature 改为含 NUL 的不同内容
	repo := itInitRepo(t)
	itWriteBytes(t, repo, "bin.dat", []byte("BIN\x01v1\n"))
	itCommitAll(t, repo, "bin base")
	testutil.RunGit(t, repo, "checkout", "-b", "bin-feature")
	itWriteBytes(t, repo, "bin.dat", []byte("BIN\x00theirs\n"))
	itCommitAll(t, repo, "bin theirs")
	testutil.RunGit(t, repo, "checkout", "master")
	itWriteBytes(t, repo, "bin.dat", []byte("BIN\x00ours\n"))
	itCommitAll(t, repo, "bin ours")
	app := itNewApp()
	if _, err := app.Merge(repo, "bin-feature", "no-ff"); err != nil {
		t.Fatalf("二进制冲突 merge 应被接受: %v", err)
	}

	result, err := app.GetConflictFileContent(repo, "bin.dat")
	if err != nil {
		t.Fatalf("GetConflictFileContent: %v", err)
	}
	if !result.Binary {
		t.Errorf("二进制冲突文件应标记 Binary: %+v", result)
	}
	if result.Content != "" {
		t.Errorf("二进制内容应为空, got %q", result.Content)
	}

	// R1：二进制冲突可用选边解决
	if err := app.ChooseConflictSide(repo, "bin.dat", "theirs"); err != nil {
		t.Fatalf("二进制选边 theirs: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "bin.dat"))
	if err != nil {
		t.Fatalf("read bin.dat: %v", err)
	}
	if string(data) != "BIN\x00theirs\n" {
		t.Errorf("选 theirs 后应为 theirs 二进制内容, got %q", data)
	}
}

// TestIntegration_GetConflictFileContent_TooLarge 超过 1MB 的冲突文件标记 TooLarge。
func TestIntegration_GetConflictFileContent_TooLarge(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	// 外部把冲突文件膨胀到 >1MB（index 仍冲突，工作区超大）
	big := strings.Repeat("x", 1024*1024+128)
	itWriteFile(t, repo, "a.txt", big)

	result, err := app.GetConflictFileContent(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetConflictFileContent: %v", err)
	}
	if !result.TooLarge {
		t.Errorf("超过 1MB 应标记 TooLarge: %+v", result)
	}
	if result.Content != "" {
		t.Errorf("超大文件内容应为空, got %d bytes", len(result.Content))
	}
}

// TestIntegration_GetConflictFileContent_Rejects 非冲突文件 / 路径逃逸 / 空文件名拒绝。
func TestIntegration_GetConflictFileContent_Rejects(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	if _, err := app.GetConflictFileContent(repo, "b.txt"); err == nil {
		t.Error("非冲突文件应拒绝")
	}
	if _, err := app.GetConflictFileContent(repo, "../escape.txt"); err == nil {
		t.Error("路径逃逸应拒绝")
	}
	if _, err := app.GetConflictFileContent(repo, ""); err == nil {
		t.Error("空文件名应拒绝")
	}
}

// TestIntegration_SaveConflictResolution 保存合并结果写盘 + 标记已解决 + Continue 完成合并。
func TestIntegration_SaveConflictResolution(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	resolved := "line1\nboth sides kept\nline3\n"
	if err := app.SaveConflictResolution(repo, "a.txt", resolved, "utf-8"); err != nil {
		t.Fatalf("SaveConflictResolution: %v", err)
	}
	if got := itReadFile(t, repo, "a.txt"); got != resolved {
		t.Errorf("写盘内容应原样, got %q", got)
	}
	itAssertResolved(t, app, repo)

	if _, err := app.ContinueMerge(repo); err != nil {
		t.Fatalf("ContinueMerge: %v", err)
	}
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != "none" {
		t.Errorf("continue 后应退出冲突态, got %s", state.Type)
	}
}

// TestIntegration_SaveConflictResolution_Rejects 非冲突文件 / 无冲突态仓库拒绝。
func TestIntegration_SaveConflictResolution_Rejects(t *testing.T) {
	itRequireGit(t)
	repo, app := itConflictRepos(t)

	if err := app.SaveConflictResolution(repo, "b.txt", "x", "utf-8"); err == nil {
		t.Error("非冲突文件应拒绝")
	}

	clean := itSetupForkRepo(t)
	if err := app.SaveConflictResolution(clean, "a.txt", "x", "utf-8"); err == nil {
		t.Error("无冲突态仓库应拒绝")
	}
}
