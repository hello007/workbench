package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util"
	"workbench/util/testutil"
)

// newMergeConflictRepo 基于 SetupConflictRepo 触发真实 merge 冲突态：
// master 与 feature 各改 a.txt 同一行，merge feature 必然冲突（exit 1 属预期，
// 不能用 RunGit——其对非零退出 t.Fatalf）。
func newMergeConflictRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.SetupConflictRepo(t)
	cmd := exec.Command("git", "merge", "feature")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("merge 应产生冲突，实际干净合并: %s", out)
	}
	return dir
}

// TestMergeToolPresetTemplate 预设三向模板：内置五预设均含四占位符；
// custom 与未知预设返回空串（回落 settings.DiffToolMergeArgs）。
func TestMergeToolPresetTemplate(t *testing.T) {
	for _, name := range []string{"beyondcompare", "winmerge", "vscode", "kdiff3", "meld"} {
		tpl := MergeToolPresetTemplate(name)
		for _, ph := range []string{"{base}", "{local}", "{remote}", "{merged}"} {
			if !strings.Contains(tpl, ph) {
				t.Errorf("预设 %s 模板缺 %s: %q", name, ph, tpl)
			}
		}
	}
	if got := MergeToolPresetTemplate("custom"); got != "" {
		t.Errorf("custom 应返回空串: %q", got)
	}
	if got := MergeToolPresetTemplate("unknown-tool"); got != "" {
		t.Errorf("未知预设应返回空串: %q", got)
	}
}

func TestOpenInExternalMerge_NotConfigured(t *testing.T) {
	repo := newMergeConflictRepo(t)
	svc := NewGitService()

	// exePath 空
	err := svc.OpenInExternalMerge(repo, "", "{base} {local} {remote} {merged}", "a.txt")
	assertDiffNotConfigured(t, err)

	// 模板空
	err = svc.OpenInExternalMerge(repo, "cmd", "", "a.txt")
	assertDiffNotConfigured(t, err)

	// 模板缺占位符
	err = svc.OpenInExternalMerge(repo, "cmd", "{left} {right}", "a.txt")
	assertDiffNotConfigured(t, err)

	// file 空
	if err := svc.OpenInExternalMerge(repo, "cmd", "{base} {local} {remote} {merged}", ""); err == nil {
		t.Error("file 空应报错")
	}
}

// TestOpenInExternalMerge_LaunchSuccess 真实冲突仓库全链路：stage 提取 → 三侧
// 临时文件内容正确 → 假工具（平台回显命令）启动成功。
func TestOpenInExternalMerge_LaunchSuccess(t *testing.T) {
	repo := newMergeConflictRepo(t)
	svc := NewGitService()

	exePath, argsTemplate := "cmd", "/c echo {base} {local} {remote} {merged}"
	if runtime.GOOS != "windows" {
		exePath, argsTemplate = "/bin/echo", "{base} {local} {remote} {merged}"
	}
	if err := svc.OpenInExternalMerge(repo, exePath, argsTemplate, "a.txt"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// 三侧临时文件内容与 stage 一致（base=共同祖先、local=master、remote=feature）
	entries, err := os.ReadDir(util.DiffTempRoot())
	if err != nil || len(entries) == 0 {
		t.Fatalf("启动后应有临时目录: %v", err)
	}
	want := map[string]string{
		"base":   "line1\nline2\nline3\n",
		"local":  "line1\nmaster-line2\nline3\n",
		"remote": "line1\nfeature-line2\nline3\n",
	}
	found := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		for side, wantContent := range want {
			data, rerr := os.ReadFile(filepath.Join(util.DiffTempRoot(), e.Name(), side, "a.txt"))
			if rerr != nil {
				continue
			}
			if string(data) != wantContent {
				t.Errorf("%s 侧内容异常: %q, want %q", side, data, wantContent)
			}
			found[side] = true
		}
	}
	for side := range want {
		if !found[side] {
			t.Errorf("未找到 %s 侧临时文件", side)
		}
	}
}

// TestOpenInExternalMerge_LaunchFailed 无效 exePath → ErrCodeDiffToolLaunchFailed。
func TestOpenInExternalMerge_LaunchFailed(t *testing.T) {
	repo := newMergeConflictRepo(t)
	svc := NewGitService()

	err := svc.OpenInExternalMerge(repo, "Z:\\no\\such\\merge-tool.exe",
		"{base} {local} {remote} {merged}", "a.txt")
	if err == nil {
		t.Fatal("expected launch error")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeDiffToolLaunchFailed {
		t.Errorf("code: got %q, want %q", appErr.Code, model.ErrCodeDiffToolLaunchFailed)
	}
}

// TestOpenInExternalMerge_NonConflictFile 非冲突态文件（index 仅 stage 0）应报
// 「不在冲突状态」而非底层 git 错误。
func TestOpenInExternalMerge_NonConflictFile(t *testing.T) {
	repo := testutil.SetupConflictRepo(t) // 分叉但未 merge，无冲突态
	svc := NewGitService()

	exePath, argsTemplate := "cmd", "/c echo {base} {local} {remote} {merged}"
	if runtime.GOOS != "windows" {
		exePath, argsTemplate = "/bin/echo", "{base} {local} {remote} {merged}"
	}
	err := svc.OpenInExternalMerge(repo, exePath, argsTemplate, "a.txt")
	if err == nil {
		t.Fatal("expected non-conflict error")
	}
	if !strings.Contains(err.Error(), "不在冲突状态") {
		t.Errorf("应报不在冲突状态: %v", err)
	}
}

// TestOpenInExternalMerge_WorkspaceDeleted delete/modify 冲突（工作区文件已删除）
// 应报工作区文件不存在。
func TestOpenInExternalMerge_WorkspaceDeleted(t *testing.T) {
	repo := newMergeConflictRepo(t)
	svc := NewGitService()

	// 删除工作区冲突文件（模拟 delete/modify 场景；不动 index 保持冲突态）
	if err := os.Remove(filepath.Join(repo, "a.txt")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	exePath, argsTemplate := "cmd", "/c echo {base} {local} {remote} {merged}"
	if runtime.GOOS != "windows" {
		exePath, argsTemplate = "/bin/echo", "{base} {local} {remote} {merged}"
	}
	err := svc.OpenInExternalMerge(repo, exePath, argsTemplate, "a.txt")
	if err == nil {
		t.Fatal("expected workspace-deleted error")
	}
	if !strings.Contains(err.Error(), "工作区文件不存在") {
		t.Errorf("应报工作区文件不存在: %v", err)
	}
}
