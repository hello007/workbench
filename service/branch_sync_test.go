package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util"
	"workbench/util/testutil"
)

// newBareRemote 创建本地 bare 仓库作推送目标（不依赖网络），返回远程路径。
func newBareRemote(t *testing.T) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "origin.git")
	// git init 不创建父目录，须先建目录（对齐集成测试 itBareRemote）
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatalf("mkdir bare remote: %v", err)
	}
	testutil.RunGit(t, remote, "init", "--bare")
	return remote
}

// setupSyncRepo 构造带 bare 远程且已 push -u 设上游的基础仓库（master 一条提交），
// 返回工作仓库与远程路径。
func setupSyncRepo(t *testing.T) (repo, remote string) {
	t.Helper()
	repo = testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	remote = newBareRemote(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")
	return repo, remote
}

// gitRevParseFull 解析指定引用的完整 SHA（trim 后）。
func gitRevParseFull(t *testing.T, repo, ref string) string {
	t.Helper()
	out, err := util.NewGitCommand().Execute(repo, "rev-parse", ref)
	if err != nil {
		t.Fatalf("rev-parse %s failed: %v", ref, err)
	}
	return strings.TrimSpace(out)
}

// findRef 按 kind 查找 refs 条目，不存在返回 nil。
func findRef(info *model.BranchSyncInfo, kind string) *model.CommitRef {
	for i := range info.Refs {
		if info.Refs[i].Kind == kind {
			return &info.Refs[i]
		}
	}
	return nil
}

func TestComputeAheadBehind_NoUpstream(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "base\n")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "base")

	ahead, behind, hasUpstream, err := ComputeAheadBehind(repo)
	if err != nil {
		t.Fatalf("无上游应降级不报错, got %v", err)
	}
	if hasUpstream {
		t.Error("expected hasUpstream=false")
	}
	if ahead != 0 || behind != 0 {
		t.Errorf("expected ahead/behind=0, got ahead=%d behind=%d", ahead, behind)
	}
}

func TestComputeAheadBehind_AheadOne(t *testing.T) {
	repo, _ := setupSyncRepo(t)
	// 本地新 commit 未 push → ahead=1
	testutil.WriteFile(t, filepath.Join(repo, "b.txt"), "new\n")
	testutil.RunGit(t, repo, "add", "b.txt")
	testutil.RunGit(t, repo, "commit", "-m", "local ahead")

	ahead, behind, hasUpstream, err := ComputeAheadBehind(repo)
	if err != nil {
		t.Fatalf("ComputeAheadBehind failed: %v", err)
	}
	if !hasUpstream {
		t.Error("expected hasUpstream=true")
	}
	if ahead != 1 || behind != 0 {
		t.Errorf("expected ahead=1 behind=0, got ahead=%d behind=%d", ahead, behind)
	}
}

func TestComputeAheadBehind_BehindOne(t *testing.T) {
	repo, remote := setupSyncRepo(t)
	// 另一 clone 推新 commit 到 remote，本地 fetch 后 → behind=1
	cloneDir := filepath.Join(t.TempDir(), "other-clone")
	testutil.RunGit(t, t.TempDir(), "clone", remote, cloneDir)
	testutil.RunGit(t, cloneDir, "config", "user.name", "other")
	testutil.RunGit(t, cloneDir, "config", "user.email", "other@test.local")
	testutil.WriteFile(t, filepath.Join(cloneDir, "c.txt"), "remote new\n")
	testutil.RunGit(t, cloneDir, "add", "c.txt")
	testutil.RunGit(t, cloneDir, "commit", "-m", "remote only")
	testutil.RunGit(t, cloneDir, "push", "origin", "master")
	testutil.RunGit(t, repo, "fetch", "origin")

	ahead, behind, hasUpstream, err := ComputeAheadBehind(repo)
	if err != nil {
		t.Fatalf("ComputeAheadBehind failed: %v", err)
	}
	if !hasUpstream {
		t.Error("expected hasUpstream=true")
	}
	if ahead != 0 || behind != 1 {
		t.Errorf("expected ahead=0 behind=1, got ahead=%d behind=%d", ahead, behind)
	}
}

func TestComputeBranchSyncInfo_InSyncRefsShareSha(t *testing.T) {
	repo, _ := setupSyncRepo(t)

	info, err := ComputeBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("ComputeBranchSyncInfo failed: %v", err)
	}
	if info.Branch != "master" {
		t.Errorf("expected branch=master, got %q", info.Branch)
	}
	if !info.HasUpstream {
		t.Error("expected hasUpstream=true")
	}
	if info.Detached {
		t.Error("expected detached=false")
	}
	if info.HeadSha == "" {
		t.Error("expected headSha non-empty")
	}
	// 同步态：local 与 remote 共标同 commit
	local := findRef(info, model.CommitRefKindLocal)
	remoteRef := findRef(info, model.CommitRefKindRemote)
	if local == nil || remoteRef == nil {
		t.Fatalf("expected local+remote refs, got %+v", info.Refs)
	}
	if local.Name != "master" {
		t.Errorf("expected local name=master, got %q", local.Name)
	}
	if remoteRef.Name != "origin/master" {
		t.Errorf("expected remote name=origin/master, got %q", remoteRef.Name)
	}
	if local.Sha != info.HeadSha {
		t.Errorf("local sha should equal headSha: %q vs %q", local.Sha, info.HeadSha)
	}
	if remoteRef.Sha != info.HeadSha {
		t.Errorf("in-sync remote sha should equal headSha: %q vs %q", remoteRef.Sha, info.HeadSha)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("expected ahead/behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
}

func TestComputeBranchSyncInfo_AheadRemoteStaysBehind(t *testing.T) {
	repo, _ := setupSyncRepo(t)
	// 本地新 commit 未 push → local 在新 commit，remote 停在旧 commit
	testutil.WriteFile(t, filepath.Join(repo, "b.txt"), "new\n")
	testutil.RunGit(t, repo, "add", "b.txt")
	testutil.RunGit(t, repo, "commit", "-m", "local ahead")
	headSha := gitRevParseFull(t, repo, "HEAD")

	info, err := ComputeBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("ComputeBranchSyncInfo failed: %v", err)
	}
	if info.Ahead != 1 || info.Behind != 0 {
		t.Errorf("expected ahead=1 behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	local := findRef(info, model.CommitRefKindLocal)
	remoteRef := findRef(info, model.CommitRefKindRemote)
	if local == nil || remoteRef == nil {
		t.Fatalf("expected local+remote refs, got %+v", info.Refs)
	}
	if local.Sha != headSha {
		t.Errorf("local sha should track HEAD, got %q want %q", local.Sha, headSha)
	}
	if remoteRef.Sha == headSha {
		t.Error("remote sha should stay at pushed commit, not local new HEAD")
	}
}

func TestComputeBranchSyncInfo_NoUpstream(t *testing.T) {
	repo, _ := setupSyncRepo(t)
	// 新建分支不设上游
	testutil.RunGit(t, repo, "checkout", "-b", "feature")

	info, err := ComputeBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("ComputeBranchSyncInfo failed: %v", err)
	}
	if info.HasUpstream {
		t.Error("expected hasUpstream=false")
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("expected ahead/behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	// 无上游：仅 local 条目，无 remote 条目
	if findRef(info, model.CommitRefKindLocal) == nil {
		t.Errorf("expected local ref, got %+v", info.Refs)
	}
	if findRef(info, model.CommitRefKindRemote) != nil {
		t.Errorf("expected no remote ref without upstream, got %+v", info.Refs)
	}
}

func TestComputeBranchSyncInfo_Detached(t *testing.T) {
	repo, _ := setupSyncRepo(t)
	sha := gitRevParseFull(t, repo, "HEAD")
	testutil.RunGit(t, repo, "checkout", sha)

	info, err := ComputeBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("ComputeBranchSyncInfo failed: %v", err)
	}
	if !info.Detached {
		t.Error("expected detached=true")
	}
	if info.Branch != sha[:8] {
		t.Errorf("expected branch=short SHA %q, got %q", sha[:8], info.Branch)
	}
	if info.HeadSha != sha {
		t.Errorf("expected headSha=%q, got %q", sha, info.HeadSha)
	}
	// detached：仅 HEAD 条目
	if len(info.Refs) != 1 {
		t.Fatalf("expected 1 ref, got %+v", info.Refs)
	}
	headRef := info.Refs[0]
	if headRef.Kind != model.CommitRefKindHead || headRef.Name != "HEAD" || headRef.Sha != sha {
		t.Errorf("expected HEAD ref entry, got %+v", headRef)
	}
}

func TestComputeBranchSyncInfo_EmptyRepoDegrades(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.SetupMasterBranch(t, repo) // 有分支名但无 commit（unborn branch）

	info, err := ComputeBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("空仓库应降级不报错, got %v", err)
	}
	if info.Branch != "master" {
		t.Errorf("expected branch=master, got %q", info.Branch)
	}
	if len(info.Refs) != 0 {
		t.Errorf("expected empty refs, got %+v", info.Refs)
	}
	if info.HasUpstream || info.Detached || info.HeadSha != "" {
		t.Errorf("expected zero-value sync info, got %+v", info)
	}
}

func TestComputeBranchSyncInfo_NotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := ComputeBranchSyncInfo(dir); err == nil {
		t.Error("expected error for non-repo path")
	}
}
