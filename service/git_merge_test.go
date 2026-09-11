package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// svcMergeWriteFile 写入测试文件，失败即终止。
func svcMergeWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// svcSetupMasterBranch 显式以 master 作为初始分支，避免 git 新版默认 main 歧义。
func svcSetupMasterBranch(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/master")
}

// svcSetupFFRepo 构造可快进合并仓库：master 基线提交，feature 领先一个提交，切回 master。
func svcSetupFFRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	svcSetupMasterBranch(t, dir)
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "base")
	runGit(t, dir, "checkout", "-b", "feature")
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "feature\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "feature")
	runGit(t, dir, "checkout", "master")
	return dir
}

// svcSetupConflictRepo 构造冲突仓库：master 与 feature 各改 a.txt 同一行 line2 并提交。
func svcSetupConflictRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	svcSetupMasterBranch(t, dir)
	runGit(t, dir, "config", "user.email", "t@t.com")
	runGit(t, dir, "config", "user.name", "t")
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "line1\nline2\nline3\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "base")
	runGit(t, dir, "checkout", "-b", "feature")
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "line1\nfeature-line2\nline3\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "feature change line2")
	runGit(t, dir, "checkout", "master")
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "line1\nmaster-line2\nline3\n")
	runGit(t, dir, "add", "a.txt")
	runGit(t, dir, "commit", "-m", "master change line2")
	return dir
}

// TestMerge_NonRepo 非仓库目录 merge 返回错误。
func TestMerge_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.Merge(t.TempDir(), "feature", model.MergeModeFF); err == nil {
		t.Error("非仓库 merge 应返回错误")
	}
}

// TestMerge_EmptyBranch 目标分支空时拒绝，且优先于工作区校验。
func TestMerge_EmptyBranch(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.Merge(dir, "  ", model.MergeModeFF); err == nil {
		t.Error("空分支名应返回错误")
	}
}

// TestMerge_DirtyWorkspace 工作区有未提交变更时拒绝 merge。
func TestMerge_DirtyWorkspace(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svcMergeWriteFile(t, filepath.Join(dir, "dirty.txt"), "dirty\n")
	svc := NewGitService()
	_, err := svc.Merge(dir, "feature", model.MergeModeFF)
	if err == nil {
		t.Fatal("工作区脏应拒绝 merge")
	}
	if !strings.Contains(err.Error(), "工作区不干净") {
		t.Errorf("错误应提示工作区不干净, got %v", err)
	}
}

// TestMerge_DetachedHead 分离头指针状态下禁止 merge。
func TestMerge_DetachedHead(t *testing.T) {
	dir := svcSetupFFRepo(t)
	runGit(t, dir, "checkout", "--detach")
	svc := NewGitService()
	_, err := svc.Merge(dir, "feature", model.MergeModeFF)
	if err == nil {
		t.Fatal("detached HEAD 应拒绝 merge")
	}
	if !strings.Contains(err.Error(), "分离头指针") {
		t.Errorf("错误应提示分离头指针, got %v", err)
	}
}

// TestMerge_Success_FF 可快进场景 merge ff 成功，无冲突态。
func TestMerge_Success_FF(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.Merge(dir, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("Merge ff: %v", err)
	}
	state, err := svc.GetConflictState(dir)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeNone {
		t.Errorf("ff 合并后应无冲突态, got %s", state.Type)
	}
}

// TestMerge_Conflict_EntersState 冲突 merge 进入冲突态，类型为 merge 且含冲突文件。
func TestMerge_Conflict_EntersState(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	if _, err := svc.Merge(dir, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("冲突应被接受为正常返回: %v", err)
	}
	state, err := svc.GetConflictState(dir)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeMerge {
		t.Errorf("冲突态类型应为 merge, got %s", state.Type)
	}
	found := false
	for _, f := range state.Files {
		if strings.TrimSpace(f) == "a.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("冲突文件应含 a.txt, got %v", state.Files)
	}
}

// TestGetConflictState_None 干净仓库冲突态为 none、文件列表空。
func TestGetConflictState_None(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	state, err := svc.GetConflictState(dir)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeNone {
		t.Errorf("应无冲突态, got %s", state.Type)
	}
	if len(state.Files) != 0 {
		t.Errorf("无冲突应文件列表空, got %v", state.Files)
	}
}

// TestResolveConflict 标记冲突文件已解决后，冲突列表清空。
func TestResolveConflict(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "line1\nresolved\nline3\n")
	if err := svc.ResolveConflict(dir, "a.txt"); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if len(state.Files) != 0 {
		t.Errorf("标记解决后冲突列表应空, got %v", state.Files)
	}
}

// TestResolveConflict_EmptyFile 文件路径空报错。
func TestResolveConflict_EmptyFile(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if err := svc.ResolveConflict(dir, ""); err == nil {
		t.Error("空文件路径应报错")
	}
}

// TestContinueMerge_NotInProgress 无进行中合并时 continue 报错。
func TestContinueMerge_NotInProgress(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.ContinueMerge(dir); err == nil {
		t.Error("无进行中合并应报错")
	}
}

// TestContinueMerge 冲突解决后 continue 完成合并，退出冲突态。
func TestContinueMerge(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	svcMergeWriteFile(t, filepath.Join(dir, "a.txt"), "line1\nresolved\nline3\n")
	svc.ResolveConflict(dir, "a.txt")
	if _, err := svc.ContinueMerge(dir); err != nil {
		t.Fatalf("ContinueMerge: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("continue 后应退出冲突态, got %s", state.Type)
	}
}

// TestAbortMerge 冲突后 abort 清理冲突态。
func TestAbortMerge(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	svc.Merge(dir, "feature", model.MergeModeFF)
	if err := svc.AbortMerge(dir); err != nil {
		t.Fatalf("AbortMerge: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("abort 后应无冲突态, got %s", state.Type)
	}
}

// TestAbortMerge_NotInProgress 无进行中合并时 abort 报错。
func TestAbortMerge_NotInProgress(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if err := svc.AbortMerge(dir); err == nil {
		t.Error("无进行中合并 abort 应报错")
	}
}

// TestRebase_EmptyBranch 目标分支空时拒绝。
func TestRebase_EmptyBranch(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.Rebase(dir, ""); err == nil {
		t.Error("空分支名应返回错误")
	}
}

// TestRebase_DirtyWorkspace 工作区脏拒绝 rebase（验证 precheck 共用）。
func TestRebase_DirtyWorkspace(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svcMergeWriteFile(t, filepath.Join(dir, "dirty.txt"), "dirty\n")
	svc := NewGitService()
	_, err := svc.Rebase(dir, "feature")
	if err == nil {
		t.Fatal("工作区脏应拒绝 rebase")
	}
	if !strings.Contains(err.Error(), "工作区不干净") {
		t.Errorf("错误应提示工作区不干净, got %v", err)
	}
}

// TestRebase_Conflict_EntersState rebase 冲突进入 rebase 态。
func TestRebase_Conflict_EntersState(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	if _, err := svc.Rebase(dir, "feature"); err != nil {
		t.Fatalf("rebase 冲突应被接受: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeRebase {
		t.Errorf("冲突态应为 rebase, got %s", state.Type)
	}
}

// TestAbortRebase 变基冲突后 abort 清理。
func TestAbortRebase(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	svc.Rebase(dir, "feature")
	if err := svc.AbortRebase(dir); err != nil {
		t.Fatalf("AbortRebase: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("abort 后应无冲突态, got %s", state.Type)
	}
}

// TestSkipRebase_NotRebase 无变基进行时 skip 报错。
func TestSkipRebase_NotRebase(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.SkipRebase(dir); err == nil {
		t.Error("无进行中变基 skip 应报错")
	}
}

// TestCherryPick_EmptySHA SHA 空拒绝。
func TestCherryPick_EmptySHA(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	if _, err := svc.CherryPick(dir, "  "); err == nil {
		t.Error("空 SHA 应返回错误")
	}
}

// TestCherryPick_Success 无冲突拣选成功。
func TestCherryPick_Success(t *testing.T) {
	dir := svcSetupFFRepo(t)
	svc := NewGitService()
	shaOut, err := svc.gitCmd.Execute(dir, "rev-parse", "feature")
	if err != nil {
		t.Fatalf("rev-parse feature: %v", err)
	}
	sha := strings.TrimSpace(shaOut)
	if _, err := svc.CherryPick(dir, sha); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("无冲突拣选应无冲突态, got %s", state.Type)
	}
}

// TestCherryPick_Conflict_EntersState 冲突拣选进入 cherry-pick 态。
func TestCherryPick_Conflict_EntersState(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	shaOut, _ := svc.gitCmd.Execute(dir, "rev-parse", "feature")
	sha := strings.TrimSpace(shaOut)
	if _, err := svc.CherryPick(dir, sha); err != nil {
		t.Fatalf("cherry-pick 冲突应被接受: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeCherryPick {
		t.Errorf("冲突态应为 cherry-pick, got %s", state.Type)
	}
}

// TestAbortCherryPick 拣选冲突后 abort 清理。
func TestAbortCherryPick(t *testing.T) {
	dir := svcSetupConflictRepo(t)
	svc := NewGitService()
	shaOut, _ := svc.gitCmd.Execute(dir, "rev-parse", "feature")
	sha := strings.TrimSpace(shaOut)
	svc.CherryPick(dir, sha)
	if err := svc.AbortCherryPick(dir); err != nil {
		t.Fatalf("AbortCherryPick: %v", err)
	}
	state, _ := svc.GetConflictState(dir)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("abort 后应无冲突态, got %s", state.Type)
	}
}

// TestPull_NonRepo_Rebase useRebase=true 时非仓库同样报错（验证 rebase 分支入口校验）。
func TestPull_NonRepo_Rebase(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.Pull(t.TempDir(), true); err == nil {
		t.Error("非仓库 pull --rebase 应返回错误")
	}
}

// TestResolveConflict_NonRepo 非仓库目录 resolve 报错。
func TestResolveConflict_NonRepo(t *testing.T) {
	svc := NewGitService()
	if err := svc.ResolveConflict(t.TempDir(), "a.txt"); err == nil {
		t.Error("非仓库 resolve 应报错")
	}
}
