package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// === TruncateDiff 测试 ===

func TestTruncateDiff_NoTruncate(t *testing.T) {
	text := "line1\nline2\nline3"
	out, truncated, dropped := TruncateDiff(text, 5)
	if truncated {
		t.Error("行数未超阈值不应截断")
	}
	if dropped != 0 {
		t.Errorf("不应丢弃行，got %d", dropped)
	}
	if out != text {
		t.Errorf("未截断应原样返回")
	}
}

func TestTruncateDiff_AtLimit(t *testing.T) {
	// 正好等于阈值不截断（<= 判定）
	text := "line1\nline2\nline3"
	out, truncated, dropped := TruncateDiff(text, 3)
	if truncated {
		t.Error("行数等于阈值不应截断")
	}
	if dropped != 0 {
		t.Errorf("等于阈值不应丢弃行，got %d", dropped)
	}
	if out != text {
		t.Errorf("等于阈值应原样返回")
	}
}

func TestTruncateDiff_OverLimit(t *testing.T) {
	// 5 行超阈值 3，截断保留前 3 行，丢弃 2 行
	text := "l1\nl2\nl3\nl4\nl5"
	out, truncated, dropped := TruncateDiff(text, 3)
	if !truncated {
		t.Error("超阈值应截断")
	}
	if dropped != 2 {
		t.Errorf("应丢弃 2 行，got %d", dropped)
	}
	if !strings.HasPrefix(out, "l1\nl2\nl3") {
		t.Errorf("应保留前 3 行，got %q", out)
	}
	if strings.Contains(out, "l4") {
		t.Errorf("截断后不应含 l4，got %q", out)
	}
}

func TestTruncateDiff_Disabled(t *testing.T) {
	// maxLines <= 0 禁用截断
	text := "l1\nl2\nl3"
	out, truncated, _ := TruncateDiff(text, 0)
	if truncated {
		t.Error("maxLines=0 应禁用截断")
	}
	if out != text {
		t.Errorf("禁用截断应原样返回")
	}
}

// === GetStagedDiff 测试 ===

func TestGetStagedDiff_UnstagedReturnsEmpty(t *testing.T) {
	// 文件有工作区改动但未 add，staged diff 为空
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "workdir-change\n") // 未 add

	svc := NewGitService()
	diff, err := svc.GetStagedDiff(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetStagedDiff 失败: %v", err)
	}
	if diff != "" {
		t.Errorf("未暂存文件 staged diff 应为空，got %q", diff)
	}
}

func TestGetStagedDiff_StagedReturnsDiff(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "staged-change\n")
	testutil.RunGit(t, repo, "add", "a.txt") // 暂存

	svc := NewGitService()
	diff, err := svc.GetStagedDiff(repo, "a.txt")
	if err != nil {
		t.Fatalf("GetStagedDiff 失败: %v", err)
	}
	if diff == "" {
		t.Error("已暂存文件应有 staged diff")
	}
	if !strings.Contains(diff, "staged-change") {
		t.Errorf("staged diff 应含暂存内容，got %q", diff)
	}
}

// === AggregateStagedDiff 测试 ===

func TestAggregateStagedDiff_EmptyStagedReturnsAppError(t *testing.T) {
	// 工作区有改动但无暂存，返回 AppError{E_GIT_NO_STAGED_CHANGES}
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "workdir-only\n") // 未 add

	svc := NewGitService()
	_, err := svc.AggregateStagedDiff(repo)
	if err == nil {
		t.Fatal("无暂存文件应返回错误")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("应返回 AppError，got %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeGitNoStagedChanges {
		t.Errorf("code 应为 E_GIT_NO_STAGED_CHANGES，got %s", appErr.Code)
	}
}

func TestAggregateStagedDiff_SingleFileContainsHeader(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "staged-change\n")
	testutil.RunGit(t, repo, "add", "a.txt")

	svc := NewGitService()
	text, err := svc.AggregateStagedDiff(repo)
	if err != nil {
		t.Fatalf("AggregateStagedDiff 失败: %v", err)
	}
	if !strings.Contains(text, "=== a.txt ===") {
		t.Errorf("聚合文本应含文件头 === a.txt ===，got %q", text)
	}
	if !strings.Contains(text, "staged-change") {
		t.Errorf("聚合文本应含暂存内容，got %q", text)
	}
}

func TestAggregateStagedDiff_MultipleFiles(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base-a\n")
	testutil.WriteFile(t, filepath.Join(repo, "b.txt"), "base-b\n")
	testutil.RunGit(t, repo, "add", "a.txt", "b.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "change-a\n")
	testutil.WriteFile(t, filepath.Join(repo, "b.txt"), "change-b\n")
	testutil.RunGit(t, repo, "add", "a.txt", "b.txt")

	svc := NewGitService()
	text, err := svc.AggregateStagedDiff(repo)
	if err != nil {
		t.Fatalf("AggregateStagedDiff 失败: %v", err)
	}
	if !strings.Contains(text, "=== a.txt ===") || !strings.Contains(text, "=== b.txt ===") {
		t.Errorf("多文件聚合应含两个文件头，got %q", text)
	}
	if !strings.Contains(text, "change-a") || !strings.Contains(text, "change-b") {
		t.Errorf("聚合文本应含两文件暂存内容，got %q", text)
	}
}

func TestAggregateStagedDiff_TruncatesManyFiles(t *testing.T) {
	// 暂存文件数超 maxDiffFiles（20），应截断并拼提示
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	// 先建基线提交（空 commit 占位）
	testutil.WriteFile(t, filepath.Join(repo, "init.txt"), "init\n")
	testutil.RunGit(t, repo, "add", "init.txt")
	testutil.RunGit(t, repo, "commit", "-m", "init")

	// 暂存 25 个文件超 maxDiffFiles=20
	for i := 0; i < 25; i++ {
		name := "f" + string(rune('a'+i)) + ".txt"
		testutil.WriteFile(t, filepath.Join(repo, name), "content-"+name+"\n")
		testutil.RunGit(t, repo, "add", name)
	}

	svc := NewGitService()
	text, err := svc.AggregateStagedDiff(repo)
	if err != nil {
		t.Fatalf("AggregateStagedDiff 失败: %v", err)
	}
	if !strings.Contains(text, "[已截断") {
		t.Errorf("超文件数阈值应拼截断提示，got %q", text)
	}
	if !strings.Contains(text, "剩余") {
		t.Errorf("截断提示应含剩余信息，got %q", text)
	}
}
