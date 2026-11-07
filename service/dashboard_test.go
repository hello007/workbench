package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/util/testutil"
)

// newTestDashboardService 创建用临时配置路径的 DashboardService。
func newTestDashboardService(t *testing.T) *DashboardService {
	t.Helper()
	return NewDashboardService(filepath.Join(t.TempDir(), "dashboard_pinned.json"))
}

func TestDashboardService_AddPin_PersistsAndNormalizes(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t) // 提供一个真实路径，确保 Abs 可规范

	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("AddPin failed: %v", err)
	}
	// 路径规范化为绝对路径后持久化
	pinned := svc.LoadPinned()
	if len(pinned) != 1 {
		t.Fatalf("expected 1 pinned, got %d", len(pinned))
	}
	abs, _ := filepath.Abs(dir)
	if pinned[0] != abs {
		t.Errorf("expected normalized %q, got %q", abs, pinned[0])
	}
}

func TestDashboardService_AddPin_Idempotent(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)

	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("first AddPin failed: %v", err)
	}
	// 同路径加相对路径变体（应规范化后去重）
	rel, _ := filepath.Rel(t.TempDir(), dir)
	if rel != "" {
		// rel 基于另一个 TempDir，可能无意义；改为直接重复加同路径验证幂等
	}
	if err := svc.AddPin(dir); err != nil {
		t.Fatalf("second AddPin failed: %v", err)
	}
	if got := len(svc.LoadPinned()); got != 1 {
		t.Errorf("expected 1 after duplicate add, got %d", got)
	}
}

func TestDashboardService_RemovePin(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)

	svc.AddPin(dir)
	svc.AddPin(testutil.InitTempRepo(t)) // 第二个仓

	if err := svc.RemovePin(dir); err != nil {
		t.Fatalf("RemovePin failed: %v", err)
	}
	pinned := svc.LoadPinned()
	if len(pinned) != 1 {
		t.Fatalf("expected 1 after remove, got %d", len(pinned))
	}
	abs, _ := filepath.Abs(dir)
	for _, p := range pinned {
		if p == abs {
			t.Errorf("removed path still present: %q", abs)
		}
	}
}

func TestDashboardService_RemovePin_Idempotent(t *testing.T) {
	svc := newTestDashboardService(t)
	// 移除不存在的路径应幂等返回 nil
	if err := svc.RemovePin("/nonexistent/path"); err != nil {
		t.Errorf("RemovePin nonexistent should be idempotent, got %v", err)
	}
}

func TestDashboardService_IsPinned(t *testing.T) {
	svc := newTestDashboardService(t)
	dir := testutil.InitTempRepo(t)
	svc.AddPin(dir)

	if !svc.IsPinned(dir) {
		t.Errorf("IsPinned should be true after AddPin")
	}
	if svc.IsPinned("/some/other/path") {
		t.Errorf("IsPinned should be false for unpinned path")
	}
}

func TestDashboardService_LoadPinned_MissingFileReturnsEmpty(t *testing.T) {
	// 配置文件不存在时应返回空列表，不报错
	svc := NewDashboardService(filepath.Join(t.TempDir(), "absent.json"))
	if got := svc.LoadPinned(); len(got) != 0 {
		t.Errorf("expected empty for missing file, got %v", got)
	}
}

func TestDashboardService_LoadPinned_CorruptFileDegradesGracefully(t *testing.T) {
	// 损坏 JSON 应降级空列表 + 不 panic（RepoMetaService 范式）
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "dashboard_pinned.json")
	if err := os.WriteFile(cfgPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	svc := NewDashboardService(cfgPath)
	if got := svc.LoadPinned(); len(got) != 0 {
		t.Errorf("expected empty for corrupt file, got %v", got)
	}
}

func TestDashboardService_GetStatuses_EmptyPinned(t *testing.T) {
	svc := newTestDashboardService(t)
	got := svc.GetStatuses()
	if len(got) != 0 {
		t.Errorf("expected empty statuses for empty pin list, got %d", len(got))
	}
}

func TestDashboardService_GetStatuses_MissingPathMarksMissing(t *testing.T) {
	svc := newTestDashboardService(t)
	svc.AddPin(filepath.Join(t.TempDir(), "deleted-repo"))
	got := svc.GetStatuses()
	if len(got) != 1 {
		t.Fatalf("expected 1 status, got %d", len(got))
	}
	if !got[0].Missing {
		t.Errorf("expected Missing=true for nonexistent path, got %+v", got[0])
	}
	if got[0].IsRepo {
		t.Errorf("expected IsRepo=false for nonexistent path")
	}
}

// commitBaseRepo 构造含单次基线提交的仓库（master 分支，a.txt 已跟踪），返回仓库根目录。
func commitBaseRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, dir)
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "base\n")
	testutil.RunGit(t, dir, "add", "a.txt")
	testutil.RunGit(t, dir, "commit", "-m", "base")
	return dir
}

// corruptHeadCommit 删除 HEAD 指向的 loose commit 对象，构造「log/status 失败但非 unborn」
// 的确定性单项失败场景（symbolic-ref/rev-parse/stash list 均不读对象，不受影响）。
func corruptHeadCommit(t *testing.T, repoDir string) {
	t.Helper()
	headData, err := os.ReadFile(filepath.Join(repoDir, ".git", "HEAD"))
	if err != nil {
		t.Fatalf("read .git/HEAD: %v", err)
	}
	ref := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(headData)), "ref:"))
	shaBytes, err := os.ReadFile(filepath.Join(repoDir, ".git", filepath.FromSlash(ref)))
	if err != nil {
		t.Fatalf("read ref %s: %v", ref, err)
	}
	sha := strings.TrimSpace(string(shaBytes))
	objPath := filepath.Join(repoDir, ".git", "objects", sha[:2], sha[2:])
	if err := os.Remove(objPath); err != nil {
		t.Fatalf("remove loose object: %v", err)
	}
}

func TestDashboardService_ComputeRepoStatus_CleanWorktree(t *testing.T) {
	dir := commitBaseRepo(t)
	svc := newTestDashboardService(t)

	st := svc.ComputeRepoStatus(dir)
	if st.Error != "" {
		t.Fatalf("unexpected error: %s", st.Error)
	}
	if st.Dirty || st.DirtyCount != 0 {
		t.Errorf("expected clean worktree, got dirty=%v dirtyCount=%d", st.Dirty, st.DirtyCount)
	}
	if st.LastCommitUnix <= 0 {
		t.Errorf("expected lastCommitUnix > 0, got %d", st.LastCommitUnix)
	}
	if st.StashCount != 0 {
		t.Errorf("expected stashCount 0, got %d", st.StashCount)
	}
}

func TestDashboardService_ComputeRepoStatus_DirtyCountCollapsesUntrackedDir(t *testing.T) {
	dir := commitBaseRepo(t)
	// 1 个已跟踪文件修改 + 1 个含多文件的 untracked 目录（porcelain 折叠为一条 untracked/ 目录）
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "modified\n")
	testutil.WriteFile(t, filepath.Join(dir, "untracked", "x.txt"), "x\n")
	testutil.WriteFile(t, filepath.Join(dir, "untracked", "y.txt"), "y\n")

	svc := newTestDashboardService(t)
	st := svc.ComputeRepoStatus(dir)
	if !st.Dirty {
		t.Errorf("expected dirty=true, got %+v", st)
	}
	// " M a.txt" + "?? untracked/"（目录折叠计 1，不展开内部文件数）
	if st.DirtyCount != 2 {
		t.Errorf("expected dirtyCount 2 (modified file + collapsed untracked dir), got %d", st.DirtyCount)
	}
	if st.LastCommitUnix <= 0 {
		t.Errorf("expected lastCommitUnix > 0 on dirty repo, got %d", st.LastCommitUnix)
	}
}

func TestDashboardService_ComputeRepoStatus_StashCount(t *testing.T) {
	dir := commitBaseRepo(t)
	// 两次真实改动分别入 stash（stash push 需有已跟踪改动；入栈后工作区恢复干净）
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "change1\n")
	testutil.RunGit(t, dir, "stash", "push", "-m", "first")
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "change2\n")
	testutil.RunGit(t, dir, "stash", "push", "-m", "second")

	svc := newTestDashboardService(t)
	st := svc.ComputeRepoStatus(dir)
	if st.StashCount != 2 {
		t.Errorf("expected stashCount 2, got %d", st.StashCount)
	}
	if st.Dirty || st.DirtyCount != 0 {
		t.Errorf("expected clean worktree after stash push, got dirty=%v dirtyCount=%d", st.Dirty, st.DirtyCount)
	}
	if st.LastCommitUnix <= 0 {
		t.Errorf("expected lastCommitUnix > 0, got %d", st.LastCommitUnix)
	}
}

func TestDashboardService_ComputeRepoStatus_UnbornRepoDegrades(t *testing.T) {
	dir := testutil.InitTempRepo(t) // git init + 身份配置，无任何 commit
	svc := newTestDashboardService(t)

	st := svc.ComputeRepoStatus(dir)
	if st.LastCommitUnix != 0 {
		t.Errorf("expected lastCommitUnix 0 for unborn repo, got %d", st.LastCommitUnix)
	}
	// unborn 是正常状态：不记 Error（log 失败经 ResolveHead 判 unborn 静默置 0）
	if st.Error != "" {
		t.Errorf("unborn repo should not record error, got %q", st.Error)
	}
	if st.Dirty || st.DirtyCount != 0 {
		t.Errorf("expected clean zero-count worktree, got dirty=%v dirtyCount=%d", st.Dirty, st.DirtyCount)
	}
	if st.StashCount != 0 {
		t.Errorf("expected stashCount 0, got %d", st.StashCount)
	}
}

func TestDashboardService_ComputeRepoStatus_SingleItemFailureDegrades(t *testing.T) {
	dir := commitBaseRepo(t)
	corruptHeadCommit(t, dir) // log -1 与 status --porcelain 失败，stash list 等其余只读调用正常

	svc := newTestDashboardService(t)
	st := svc.ComputeRepoStatus(dir)
	if !st.IsRepo {
		t.Fatalf("expected IsRepo=true, got %+v", st)
	}
	// 单项失败置零值 + Error 记录（既有非致命降级模式）
	if st.Dirty || st.DirtyCount != 0 {
		t.Errorf("expected dirty item degraded to zero, got dirty=%v dirtyCount=%d", st.Dirty, st.DirtyCount)
	}
	if st.LastCommitUnix != 0 {
		t.Errorf("expected lastCommitUnix 0 on failure, got %d", st.LastCommitUnix)
	}
	if st.Error == "" {
		t.Errorf("expected non-empty Error for failed status/log calls")
	}
	// 不阻塞整行：stash 项仍正常计算（IsRepo 保持 true，行不因单项失败中断）
	if st.StashCount != 0 {
		t.Errorf("expected stashCount still computed (0), got %d", st.StashCount)
	}
}

func TestDashboardService_ComputeRepoStatus_DetachedShowsLastCommit(t *testing.T) {
	dir := commitBaseRepo(t)
	testutil.RunGit(t, dir, "checkout", "--detach")

	svc := newTestDashboardService(t)
	st := svc.ComputeRepoStatus(dir)
	if !st.Detached {
		t.Errorf("expected detached=true, got %+v", st)
	}
	// detached HEAD 有对应 commit：最后提交时间正常显示
	if st.LastCommitUnix <= 0 {
		t.Errorf("expected lastCommitUnix > 0 on detached HEAD, got %d", st.LastCommitUnix)
	}
	if st.Error != "" {
		t.Errorf("unexpected error: %s", st.Error)
	}
}
