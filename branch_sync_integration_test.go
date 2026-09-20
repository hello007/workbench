//go:build integration

package main

// 分支同步摘要集成测试（09-20-push 任务，与 dashboard_integration_test.go 同范式）。
//
// 用真实 git 仓库 fixture + 本地 bare 远程（不依赖网络）经 App 层全链校验 GetBranchSyncInfo
// 在 ahead/behind/同步/无上游/detached HEAD 各场景的摘要数值与 refs 位置：
//   - ahead：本地新 commit 未 push（local 指向 HEAD，remote 停在已推送 commit）
//   - behind：远端推新 commit 后本地 fetch（remote 指向远端新 commit）
//   - 同步：push 后双向共标同 commit（local 与 remote sha 相同）
//   - no-upstream：分支未设跟踪上游（仅 local 条目，无 remote 条目）
//   - detached：HEAD detached（仅 head 条目，branch=短 SHA）
//
// ahead/behind 数值与命令行基准 itRevListAheadBehind（CLI 同命令）比对。
// 运行：go test -tags=integration ./...（默认标签不编译本文件）。

import (
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// itFindSyncRef 按 kind 查找 refs 条目，不存在返回 nil。
func itFindSyncRef(info *model.BranchSyncInfo, kind string) *model.CommitRef {
	for i := range info.Refs {
		if info.Refs[i].Kind == kind {
			return &info.Refs[i]
		}
	}
	return nil
}

// TestGetBranchSyncInfo_Ahead 本地新 commit 未 push：ahead=1，local 跟踪 HEAD、remote 停在旧 commit。
func TestGetBranchSyncInfo_Ahead(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	remote := itBareRemote(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")

	// 本地新 commit 未 push → ahead 1
	itWriteFile(t, repo, "b.txt", "new\n")
	itCommitAll(t, repo, "local ahead")
	headSHA := itRevParse(t, repo, "HEAD")

	app := itNewApp()
	info, err := app.GetBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("GetBranchSyncInfo failed: %v", err)
	}
	if info.Branch != "master" {
		t.Errorf("expected branch=master, got %q", info.Branch)
	}
	if !info.HasUpstream || info.Detached {
		t.Errorf("expected hasUpstream=true detached=false, got %+v", info)
	}
	if info.HeadSha != headSHA {
		t.Errorf("expected headSha=%q, got %q", headSHA, info.HeadSha)
	}
	if info.Ahead != 1 || info.Behind != 0 {
		t.Errorf("expected ahead=1 behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	// 与命令行基准一致
	expAhead, expBehind := itRevListAheadBehind(t, repo)
	if info.Ahead != expAhead || info.Behind != expBehind {
		t.Errorf("mismatch with CLI: got ahead=%d behind=%d, CLI ahead=%d behind=%d",
			info.Ahead, info.Behind, expAhead, expBehind)
	}
	// refs：local 跟踪 HEAD，remote 停在已推送 commit
	local := itFindSyncRef(info, model.CommitRefKindLocal)
	remoteRef := itFindSyncRef(info, model.CommitRefKindRemote)
	if local == nil || remoteRef == nil {
		t.Fatalf("expected local+remote refs, got %+v", info.Refs)
	}
	if local.Name != "master" || local.Sha != headSHA {
		t.Errorf("local ref should track HEAD, got %+v", *local)
	}
	if remoteRef.Name != "origin/master" || remoteRef.Sha == headSHA {
		t.Errorf("remote ref should stay at pushed commit, got %+v", *remoteRef)
	}
}

// TestGetBranchSyncInfo_Behind 远端推新 commit 后本地 fetch：behind=1，remote 指向远端新 commit。
func TestGetBranchSyncInfo_Behind(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	remote := itBareRemote(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")

	// 另一 clone 推新 commit（模拟他人在远端推进）
	cloneDir := filepath.Join(t.TempDir(), "other-clone")
	testutil.RunGit(t, t.TempDir(), "clone", remote, cloneDir)
	// clone 仓库不带本测试会话的 git 身份，须显式补 config，否则 CI 干净容器 commit 因缺 user.email 失败
	testutil.RunGit(t, cloneDir, "config", "user.name", "other")
	testutil.RunGit(t, cloneDir, "config", "user.email", "other@test.local")
	itWriteFile(t, cloneDir, "c.txt", "remote new\n")
	itCommitAll(t, cloneDir, "remote only commit")
	testutil.RunGit(t, cloneDir, "push", "origin", "master")
	// 本地 fetch 拿到远端新引用
	testutil.RunGit(t, repo, "fetch", "origin")
	remoteHeadSHA := itRevParse(t, repo, "origin/master")

	app := itNewApp()
	info, err := app.GetBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("GetBranchSyncInfo failed: %v", err)
	}
	if info.Ahead != 0 || info.Behind != 1 {
		t.Errorf("expected ahead=0 behind=1, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	expAhead, expBehind := itRevListAheadBehind(t, repo)
	if info.Ahead != expAhead || info.Behind != expBehind {
		t.Errorf("mismatch with CLI: got ahead=%d behind=%d, CLI ahead=%d behind=%d",
			info.Ahead, info.Behind, expAhead, expBehind)
	}
	// refs：remote 指向远端新 commit（用户可见远程头位置）
	remoteRef := itFindSyncRef(info, model.CommitRefKindRemote)
	if remoteRef == nil {
		t.Fatalf("expected remote ref, got %+v", info.Refs)
	}
	if remoteRef.Sha != remoteHeadSHA {
		t.Errorf("remote ref should point to origin/master %q, got %q", remoteHeadSHA, remoteRef.Sha)
	}
	local := itFindSyncRef(info, model.CommitRefKindLocal)
	if local == nil || local.Sha != info.HeadSha {
		t.Errorf("local ref should track HEAD, got %+v", info.Refs)
	}
}

// TestGetBranchSyncInfo_InSync push 后双向同步：local 与 remote 共标同 commit。
func TestGetBranchSyncInfo_InSync(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	remote := itBareRemote(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "remote", "add", "origin", remote)
	testutil.RunGit(t, repo, "push", "-u", "origin", "master")
	headSHA := itRevParse(t, repo, "HEAD")

	app := itNewApp()
	info, err := app.GetBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("GetBranchSyncInfo failed: %v", err)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("expected ahead=0 behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	// 远程头与本地头重合：local 与 remote 同 sha（提交行同行共标）
	local := itFindSyncRef(info, model.CommitRefKindLocal)
	remoteRef := itFindSyncRef(info, model.CommitRefKindRemote)
	if local == nil || remoteRef == nil {
		t.Fatalf("expected local+remote refs, got %+v", info.Refs)
	}
	if local.Sha != headSHA || remoteRef.Sha != headSHA {
		t.Errorf("in-sync refs should share HEAD sha %q, got local=%q remote=%q",
			headSHA, local.Sha, remoteRef.Sha)
	}
}

// TestGetBranchSyncInfo_NoUpstream 分支未设跟踪上游：hasUpstream=false，仅 local 条目。
func TestGetBranchSyncInfo_NoUpstream(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	// 新建分支不设上游
	testutil.RunGit(t, repo, "checkout", "-b", "feature")

	app := itNewApp()
	info, err := app.GetBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("GetBranchSyncInfo failed: %v", err)
	}
	if info.HasUpstream {
		t.Errorf("expected hasUpstream=false, got %+v", info)
	}
	if info.Ahead != 0 || info.Behind != 0 {
		t.Errorf("expected ahead/behind=0, got ahead=%d behind=%d", info.Ahead, info.Behind)
	}
	if info.Branch != "feature" {
		t.Errorf("expected branch=feature, got %q", info.Branch)
	}
	if itFindSyncRef(info, model.CommitRefKindLocal) == nil {
		t.Errorf("expected local ref, got %+v", info.Refs)
	}
	if itFindSyncRef(info, model.CommitRefKindRemote) != nil {
		t.Errorf("expected no remote ref without upstream, got %+v", info.Refs)
	}
}

// TestGetBranchSyncInfo_Detached detached HEAD：detached=true，仅 head 条目，branch=短 SHA。
func TestGetBranchSyncInfo_Detached(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	sha := itRevParse(t, repo, "HEAD")
	testutil.RunGit(t, repo, "checkout", sha)

	app := itNewApp()
	info, err := app.GetBranchSyncInfo(repo)
	if err != nil {
		t.Fatalf("GetBranchSyncInfo failed: %v", err)
	}
	if !info.Detached {
		t.Errorf("expected detached=true, got %+v", info)
	}
	if info.Branch != sha[:8] {
		t.Errorf("expected branch=short SHA %q, got %q", sha[:8], info.Branch)
	}
	if info.HeadSha != sha {
		t.Errorf("expected headSha=%q, got %q", sha, info.HeadSha)
	}
	if len(info.Refs) != 1 {
		t.Fatalf("expected 1 ref, got %+v", info.Refs)
	}
	if info.Refs[0].Kind != model.CommitRefKindHead || info.Refs[0].Name != "HEAD" || info.Refs[0].Sha != sha {
		t.Errorf("expected HEAD ref entry, got %+v", info.Refs[0])
	}
}

// TestGetBranchSyncInfo_EmptyPath 空路径报错不 panic。
func TestGetBranchSyncInfo_EmptyPath(t *testing.T) {
	itRequireGit(t)
	app := itNewApp()
	if _, err := app.GetBranchSyncInfo(""); err == nil {
		t.Error("expected error for empty path")
	}
}
