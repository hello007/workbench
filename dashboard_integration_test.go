//go:build integration

package main

// 全局状态看板集成测试（方案 C 后端层，与 git_flows_integration_test.go 同范式）。
//
// 用真实 git 仓库 fixture + 本地 bare 远程（不依赖网络）校验 ComputeRepoStatus / GetStatuses
// 在 ahead/behind/dirty/no-upstream/detached HEAD 各场景的数值与命令行一致：
//   - ahead：本地领先上游（work 新 commit 未 push）
//   - behind：本地落后上游（远端推新 commit 后 fetch）
//   - dirty：工作区有未提交改动
//   - no-upstream：分支未设跟踪上游
//   - detached：HEAD 处于 detached 状态
//
// 运行：go test -tags=integration ./...（默认标签不编译本文件）。
// 前置：git 在 PATH（itRequireGit 缺失则 skip）。

import (
	"path/filepath"
	"strings"
	"testing"

	"workbench/service"
	"workbench/util/testutil"
)

// newItDashboardService 集成测试用 DashboardService（临时配置路径）。
func newItDashboardService(t *testing.T) *service.DashboardService {
	t.Helper()
	return service.NewDashboardService(filepath.Join(t.TempDir(), "dashboard_pinned.json"))
}

// itRevListAheadBehind 用命令行直接算 ahead/behind 作校验基准（与 ComputeRepoStatus 同命令）。
func itRevListAheadBehind(t *testing.T, dir string) (ahead, behind int) {
	t.Helper()
	out := itGitOut(t, dir, "rev-list", "--left-right", "--count", "@{u}...HEAD")
	parts := strings.Split(out, "\t")
	if len(parts) != 2 {
		t.Fatalf("rev-list output unexpected: %q", out)
	}
	behind = atoiSafe(t, strings.TrimSpace(parts[0]))
	ahead = atoiSafe(t, strings.TrimSpace(parts[1]))
	return ahead, behind
}

func atoiSafe(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("atoi unexpected char in %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// TestComputeRepoStatus_Ahead 本地新 commit 未 push，ahead=1 behind=0，HasUpstream=true。
func TestComputeRepoStatus_Ahead(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	remote := itBareRemote(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	// 推送到 bare 远程并设上游
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")

	// 本地新 commit 未 push → ahead 1
	itWriteFile(t, repo, "b.txt", "new\n")
	itCommitAll(t, repo, "local ahead")

	svc := newItDashboardService(t)
	status := svc.ComputeRepoStatus(repo)

	if !status.IsRepo {
		t.Fatalf("expected IsRepo=true")
	}
	if status.Branch != "master" {
		t.Errorf("expected branch master, got %q", status.Branch)
	}
	if !status.HasUpstream {
		t.Errorf("expected HasUpstream=true")
	}
	if status.Ahead != 1 {
		t.Errorf("expected ahead=1, got %d", status.Ahead)
	}
	if status.Behind != 0 {
		t.Errorf("expected behind=0, got %d", status.Behind)
	}
	// 新 commit 已提交，工作区干净 → dirty=false
	if status.Dirty {
		t.Errorf("expected dirty=false after commit")
	}

	// 与命令行基准一致
	expAhead, expBehind := itRevListAheadBehind(t, repo)
	if status.Ahead != expAhead || status.Behind != expBehind {
		t.Errorf("mismatch with CLI: got ahead=%d behind=%d, CLI ahead=%d behind=%d",
			status.Ahead, status.Behind, expAhead, expBehind)
	}
}

// TestComputeRepoStatus_Behind 远端推新 commit 后本地 fetch，behind=1 ahead=0。
func TestComputeRepoStatus_Behind(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	remote := itBareRemote(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")

	// 另一 clone 推新 commit 到 remote（模拟他人在远端推进）
	cloneDir := filepath.Join(t.TempDir(), "other-clone")
	testutil.RunGit(t, t.TempDir(), "clone", remote, cloneDir)
	testutil.RunGit(t, cloneDir, "config", "user.name", "other")
	testutil.RunGit(t, cloneDir, "config", "user.email", "other@test.local")
	itWriteFile(t, cloneDir, "c.txt", "remote new\n")
	itCommitAll(t, cloneDir, "remote only commit")
	testutil.RunGit(t, cloneDir, "push", "origin", "master")

	// 本地 fetch 拿到远端新引用（看板不主动 fetch，需用户已 fetch 过）
	testutil.RunGit(t, repo, "fetch", "origin")

	svc := newItDashboardService(t)
	status := svc.ComputeRepoStatus(repo)

	if !status.HasUpstream {
		t.Errorf("expected HasUpstream=true")
	}
	if status.Behind != 1 {
		t.Errorf("expected behind=1, got %d", status.Behind)
	}
	if status.Ahead != 0 {
		t.Errorf("expected ahead=0, got %d", status.Ahead)
	}

	expAhead, expBehind := itRevListAheadBehind(t, repo)
	if status.Ahead != expAhead || status.Behind != expBehind {
		t.Errorf("mismatch with CLI: got ahead=%d behind=%d, CLI ahead=%d behind=%d",
			status.Ahead, status.Behind, expAhead, expBehind)
	}
}

// TestComputeRepoStatus_Dirty 工作区有未提交改动，dirty=true。
func TestComputeRepoStatus_Dirty(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")

	// 改文件不提交 → dirty
	itWriteFile(t, repo, "a.txt", "modified\n")

	svc := newItDashboardService(t)
	status := svc.ComputeRepoStatus(repo)

	if !status.Dirty {
		t.Errorf("expected dirty=true for uncommitted change")
	}
}

// TestComputeRepoStatus_NoUpstream 分支未设跟踪上游，HasUpstream=false 且 ahead/behind=0 不报错。
func TestComputeRepoStatus_NoUpstream(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	// 新建分支不设上游
	testutil.RunGit(t, repo, "checkout", "-b", "feature")

	svc := newItDashboardService(t)
	status := svc.ComputeRepoStatus(repo)

	if status.HasUpstream {
		t.Errorf("expected HasUpstream=false for branch without upstream")
	}
	if status.Ahead != 0 || status.Behind != 0 {
		t.Errorf("expected ahead/behind=0 without upstream, got ahead=%d behind=%d",
			status.Ahead, status.Behind)
	}
	if status.Error != "" {
		t.Errorf("expected no error without upstream (degrade), got %q", status.Error)
	}
}

// TestComputeRepoStatus_Detached HEAD detached 时标 Detached=true，branch 取短 SHA。
func TestComputeRepoStatus_Detached(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	// checkout 到具体 commit 进入 detached
	sha := itGitOut(t, repo, "rev-parse", "HEAD")
	testutil.RunGit(t, repo, "checkout", sha)

	svc := newItDashboardService(t)
	status := svc.ComputeRepoStatus(repo)

	if !status.Detached {
		t.Errorf("expected Detached=true")
	}
	// branch 应展示短 SHA（非空）
	if status.Branch == "" {
		t.Errorf("expected branch=short SHA for detached, got empty")
	}
}

// TestGetStatuses_MultiRepoConcurrent 多 pin 仓批量并发，结果顺序与 pin 列表一致。
func TestGetStatuses_MultiRepoConcurrent(t *testing.T) {
	itRequireGit(t)
	repo1 := itInitRepo(t)
	itWriteFile(t, repo1, "a.txt", "base\n")
	itCommitAll(t, repo1, "base")

	repo2 := itInitRepo(t)
	itWriteFile(t, repo2, "a.txt", "base\n")
	itCommitAll(t, repo2, "base")
	// repo2 dirty
	itWriteFile(t, repo2, "a.txt", "modified\n")

	svc := newItDashboardService(t)
	svc.AddPin(repo1)
	svc.AddPin(repo2)

	statuses := svc.GetStatuses()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	// 顺序与 pin 列表一致
	if statuses[0].Path != repo1 {
		t.Errorf("expected statuses[0].Path=%q, got %q", repo1, statuses[0].Path)
	}
	if statuses[1].Path != repo2 {
		t.Errorf("expected statuses[1].Path=%q, got %q", repo2, statuses[1].Path)
	}
	if statuses[0].Dirty {
		t.Errorf("repo1 should be clean")
	}
	if !statuses[1].Dirty {
		t.Errorf("repo2 should be dirty")
	}
}
