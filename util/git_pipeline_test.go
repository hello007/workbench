package util

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util/testutil"
)

// TestExecutePipeline_PatchID 真实 git 双进程管道冒烟：临时仓库单提交（真实文件变更，
// 保证 log -p 有 patch 输出），`git log -p --format=%H` 管道接入 `git patch-id --stable`，
// 断言输出行为 "<patchId> <commitHash>" 两字段。git 是本项目硬依赖，缺失环境降级 skip。
func TestExecutePipeline_PatchID(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 不在 PATH，跳过管道测试")
	}
	repo := testutil.InitTempRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "hello\n")
	testutil.RunGit(t, repo, "add", "-A")
	testutil.RunGit(t, repo, "commit", "-m", "pipeline smoke")

	g := NewGitCommand()
	// first/second 均为 git 参数（不含程序名，ExecutePipeline 统一前置 git）
	out, err := g.ExecutePipeline(context.Background(), repo,
		[]string{"log", "-p", "--format=%H"},
		[]string{"patch-id", "--stable"})
	if err != nil {
		t.Fatalf("ExecutePipeline: %v", err)
	}
	fields := strings.Fields(strings.TrimSpace(out))
	if len(fields) != 2 {
		t.Fatalf("patch-id 输出应为 '<patchId> <commitHash>' 两字段, got %q", out)
	}
	if len(fields[0]) != 40 || len(fields[1]) != 40 {
		t.Errorf("patchId 与 commitHash 均应为 40 位, got %q", out)
	}
}

// TestExecutePipeline_SecondFailure second 命令失败：错误返回且不永挂（first 收尸路径）。
func TestExecutePipeline_SecondFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 不在 PATH，跳过管道测试")
	}
	repo := testutil.InitTempRepo(t)
	g := NewGitCommand()

	// first 用恒成功的 git --version（空仓库 rev-parse 会先失败混淆断言对象）；
	// second 为不存在的 git 子命令：立即退出非零，first 不阻塞
	if _, err := g.ExecutePipeline(context.Background(), repo,
		[]string{"--version"},
		[]string{"definitely-not-a-subcommand"}); err == nil {
		t.Fatal("second 失败应返回错误")
	}
}

// TestExecutePipeline_FirstFailure first 命令失败（rev 不存在）：second 收到空输入正常退出，
// 管道以 first 的错误收口。
func TestExecutePipeline_FirstFailure(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 不在 PATH，跳过管道测试")
	}
	repo := testutil.InitTempRepo(t)
	g := NewGitCommand()

	if _, err := g.ExecutePipeline(context.Background(), repo,
		[]string{"log", "-p", "--format=%H", "no-such-ref"},
		[]string{"patch-id", "--stable"}); err == nil {
		t.Fatal("first 失败应返回错误")
	}
}
