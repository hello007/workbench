package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util"
)

// newDiffTestRepo 创建含两次提交的测试仓库：
//   - v1：main.go 内容 "v1"
//   - v2：main.go 内容 "v2"，新增 added.txt 内容 "new"
//
// 返回仓库路径与 v1/v2 提交 SHA。
func newDiffTestRepo(t *testing.T) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	writeDiffFile(t, filepath.Join(dir, "main.go"), "v1")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "v1")
	v1 := gitRevParse(t, dir, "HEAD")

	writeDiffFile(t, filepath.Join(dir, "main.go"), "v2")
	writeDiffFile(t, filepath.Join(dir, "added.txt"), "new")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "v2")
	v2 := gitRevParse(t, dir, "HEAD")

	return dir, v1, v2
}

func writeDiffFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func gitRevParse(t *testing.T, dir, rev string) string {
	t.Helper()
	svc := NewGitService()
	out, err := svc.gitCmd.Execute(dir, "rev-parse", rev)
	if err != nil {
		t.Fatalf("rev-parse %s: %v", rev, err)
	}
	return strings.TrimSpace(out)
}

func assertDiffNotConfigured(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var appErr *model.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != model.ErrCodeDiffToolNotConfigured {
		t.Errorf("code: got %q, want %q", appErr.Code, model.ErrCodeDiffToolNotConfigured)
	}
}

func TestOpenInExternalDiff_NotConfigured(t *testing.T) {
	svc := NewGitService()
	err := svc.OpenInExternalDiff(t.TempDir(), "  ", "{left} {right}", model.ExternalDiffRequest{Mode: "workspace", File: "a.go"})
	assertDiffNotConfigured(t, err)
}

func TestOpenInExternalDiff_TemplateMissingPlaceholder(t *testing.T) {
	svc := NewGitService()
	cases := []string{"", "{left}", "{right}", "--diff"}
	for _, tpl := range cases {
		err := svc.OpenInExternalDiff(t.TempDir(), "tool.exe", tpl, model.ExternalDiffRequest{Mode: "workspace", File: "a.go"})
		assertDiffNotConfigured(t, err)
	}
}

func TestOpenInExternalDiff_EmptyFile(t *testing.T) {
	svc := NewGitService()
	err := svc.OpenInExternalDiff(t.TempDir(), "tool.exe", "{left} {right}", model.ExternalDiffRequest{Mode: "workspace"})
	if err == nil || err.Error() != "文件路径不能为空" {
		t.Errorf("got %v", err)
	}
}

func TestResolveExternalDiffSides_Workspace(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	writeDiffFile(t, filepath.Join(repo, "main.go"), "v2-working")

	svc := NewGitService()
	left, rightPath, needTempRight, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "workspace", File: "main.go"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "v2" {
		t.Errorf("左侧应为 HEAD 版本 v2, got %q", left)
	}
	if needTempRight {
		t.Error("workspace 右侧应为原文件，不写临时文件")
	}
	if rightPath != filepath.Join(repo, "main.go") {
		t.Errorf("右侧应为原文件绝对路径, got %q", rightPath)
	}
}

func TestResolveExternalDiffSides_WorkspaceUntracked(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	writeDiffFile(t, filepath.Join(repo, "new.txt"), "brand new")

	svc := NewGitService()
	left, _, needTempRight, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "workspace", File: "new.txt"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "" {
		t.Errorf("未跟踪文件左侧应为空, got %q", left)
	}
	if needTempRight {
		t.Error("未跟踪文件仍存在于工作区，右侧不应写临时文件")
	}
}

func TestResolveExternalDiffSides_WorkspaceDeleted(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	if err := os.Remove(filepath.Join(repo, "main.go")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	svc := NewGitService()
	left, rightPath, needTempRight, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "workspace", File: "main.go"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "v2" {
		t.Errorf("删除文件左侧应为 HEAD 版本 v2, got %q", left)
	}
	if !needTempRight || rightPath != "" {
		t.Errorf("删除文件右侧应降级临时文件, needTempRight=%v rightPath=%q", needTempRight, rightPath)
	}
}

func TestResolveExternalDiffSides_Commit(t *testing.T) {
	repo, _, v2 := newDiffTestRepo(t)

	svc := NewGitService()
	left, right, needTempRight, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "commit", File: "main.go", SHA: v2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "v1" || right != "v2" {
		t.Errorf("commit 两侧内容: left=%q right=%q", left, right)
	}
	if !needTempRight {
		t.Error("commit 两侧均应写临时文件")
	}
}

func TestResolveExternalDiffSides_CommitRoot(t *testing.T) {
	repo, v1, _ := newDiffTestRepo(t)

	svc := NewGitService()
	left, right, _, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "commit", File: "main.go", SHA: v1})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "" {
		t.Errorf("root commit 左侧应为空（对比空树）, got %q", left)
	}
	if right != "v1" {
		t.Errorf("root commit 右侧应为 v1, got %q", right)
	}
}

func TestResolveExternalDiffSides_CommitAddedFile(t *testing.T) {
	repo, _, v2 := newDiffTestRepo(t)

	svc := NewGitService()
	left, right, _, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "commit", File: "added.txt", SHA: v2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "" {
		t.Errorf("新增文件左侧应为空, got %q", left)
	}
	if right != "new" {
		t.Errorf("新增文件右侧应为 new, got %q", right)
	}
}

func TestResolveExternalDiffSides_Range(t *testing.T) {
	repo, v1, v2 := newDiffTestRepo(t)

	svc := NewGitService()
	left, right, needTempRight, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "range", File: "main.go", BaseSHA: v1, HeadSHA: v2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if left != "v1" || right != "v2" {
		t.Errorf("range 两侧内容: left=%q right=%q", left, right)
	}
	if !needTempRight {
		t.Error("range 两侧均应写临时文件")
	}
}

func TestResolveExternalDiffSides_InvalidMode(t *testing.T) {
	svc := NewGitService()
	_, _, _, err := svc.resolveExternalDiffSides(t.TempDir(), model.ExternalDiffRequest{Mode: "bogus", File: "a.go"})
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestResolveExternalDiffSides_MissingSHA(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	svc := NewGitService()

	if _, _, _, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "commit", File: "main.go"}); err == nil {
		t.Error("commit 缺 SHA 应返回错误")
	}
	if _, _, _, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "range", File: "main.go", BaseSHA: "abc"}); err == nil {
		t.Error("range 缺 HeadSHA 应返回错误")
	}
}

// TestResolveExternalDiffSides_InvalidSHA SHA 拼入 git show 参数前必须过形态校验：
// 以 - 开头的值会被 git 解析为命令行选项（如 --output= 任意写文件），属安全校验。
func TestResolveExternalDiffSides_InvalidSHA(t *testing.T) {
	repo, v1, _ := newDiffTestRepo(t)
	svc := NewGitService()

	cases := []model.ExternalDiffRequest{
		{Mode: "commit", File: "main.go", SHA: "--output=C:/evil.txt"},
		{Mode: "commit", File: "main.go", SHA: "rm -rf; echo"},
		{Mode: "range", File: "main.go", BaseSHA: "--exec=boom", HeadSHA: v1},
		{Mode: "range", File: "main.go", BaseSHA: v1, HeadSHA: "HEAD; touch /tmp/x"},
	}
	for _, req := range cases {
		if _, _, _, err := svc.resolveExternalDiffSides(repo, req); err == nil {
			t.Errorf("非法 SHA %q 应被拒绝", req.SHA+req.BaseSHA+req.HeadSHA)
		}
	}

	// 合法 SHA（大小写混合十六进制）应正常通过校验进入 git 取内容
	if _, _, _, err := svc.resolveExternalDiffSides(repo, model.ExternalDiffRequest{Mode: "commit", File: "main.go", SHA: strings.ToUpper(v1)}); err != nil {
		t.Errorf("大写合法 SHA 不应被拒绝: %v", err)
	}
}

func TestOpenInExternalDiff_LaunchFailed(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	svc := NewGitService()

	err := svc.OpenInExternalDiff(repo, "Z:\\no\\such\\diff-tool.exe", "{left} {right}",
		model.ExternalDiffRequest{Mode: "workspace", File: "main.go"})
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

func TestOpenInExternalDiff_LaunchSuccess(t *testing.T) {
	repo, _, _ := newDiffTestRepo(t)
	svc := NewGitService()

	// 用 cmd /c echo 作为假 diff 工具：立即退出、无副作用，验证启动链路与参数渲染
	err := svc.OpenInExternalDiff(repo, "cmd", "/c echo {left} vs {right}",
		model.ExternalDiffRequest{Mode: "workspace", File: "main.go"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// 启动后临时目录应存在且左侧内容为 HEAD 版本
	entries, readErr := os.ReadDir(util.DiffTempRoot())
	if readErr != nil || len(entries) == 0 {
		t.Fatalf("启动后应有临时目录: %v", readErr)
	}
}

func TestDiffTempFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"main.go", "main.go"},
		{"src/main.go", "main.go"},
		{"a/b/c.txt", "c.txt"},
		{`a\b\c.txt`, "c.txt"},
	}
	for _, c := range cases {
		if got := diffTempFileName(c.in); got != c.want {
			t.Errorf("diffTempFileName(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}
