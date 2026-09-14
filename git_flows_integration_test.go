//go:build integration

package main

// Git 关键流程集成测试（E2E 任务 PR3，方案 C 后端层）。
//
// 与单测的区别：本文件用 t.TempDir() 构造真实 git 仓库 fixture（含 bare 远程仓库），
// 经 AppServices 组装真实 service 链（无 mock）驱动三条关键用户流程：
//   1. 提交流：GetLocalChanges → StageFiles → CommitFiles → GetLocalChanges 清空 → GetCommitHistory 出现新提交
//   2. 推送流：PushRepo 推到本地 bare 远程 → 直接校验 bare 仓库产物（refs 指向工作仓 HEAD）
//   3. 分支流：GetBranches / CreateBranch / CheckoutBranch / RenameBranch / DeleteBranch 全链
//   4. 合并流：ff 快进、no-ff 合并提交、Rebase 变基、真冲突 + GetConflictState + AbortMerge / ResolveConflict + ContinueMerge
//
// 运行方式：go test -tags=integration ./...（默认标签 go test ./... 不编译本文件，
// 覆盖率门禁 scripts/coverage-check.sh 跑默认标签，不受影响）。
// 前置条件：git 在 PATH（缺失则 skip 不 fail）；远程为本地 bare 仓库，不依赖网络。
// 确定性保障：每个仓库显式 symbolic-ref master（规避 init.defaultBranch 差异）、
// 局部配置 user.name/email、core.autocrlf=false 与 commit.gpgsign=false（规避环境差异）。

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/service"
	"workbench/util/testutil"
)

// itRequireGit 前置检查：git 不在 PATH 时跳过（CI 保证有 git，本地异常环境降级为 skip 不 fail）。
func itRequireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 不在 PATH，跳过集成测试")
	}
}

// itInitRepo 在临时目录初始化确定性 git 仓库：
// 显式 master 初始分支 + 局部 user 配置 + 关闭 autocrlf/gpgsign，规避环境差异导致的 flaky。
func itInitRepo(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "work-repo")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	testutil.RunGit(t, dir, "init")
	testutil.RunGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/master")
	testutil.RunGit(t, dir, "config", "user.name", "integration")
	testutil.RunGit(t, dir, "config", "user.email", "integration@test.local")
	testutil.RunGit(t, dir, "config", "core.autocrlf", "false")
	testutil.RunGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// itBareRemote 创建本地 bare 仓库作推送目标（不依赖网络）。
func itBareRemote(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "origin.git")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	testutil.RunGit(t, dir, "init", "--bare")
	return dir
}

// itWriteFile 在仓库内写文件（rel 为仓库相对路径，跨平台经 filepath.Join 拼接）。
func itWriteFile(t *testing.T, repoPath, rel, content string) {
	t.Helper()
	full := filepath.Join(repoPath, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
}

// itCommitAll 暂存全部变更并提交（fixture 造数用，被测行为走 App 链）。
func itCommitAll(t *testing.T, repoPath, message string) {
	t.Helper()
	testutil.RunGit(t, repoPath, "add", "-A")
	testutil.RunGit(t, repoPath, "commit", "-m", message)
}

// itGitOut 执行 git 命令并返回 trim 后输出（rev-parse 等需要读取结果的校验用）。
func itGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// itRevParse 解析指定引用的完整 SHA。
func itRevParse(t *testing.T, repoPath, ref string) string {
	t.Helper()
	sha := itGitOut(t, repoPath, "rev-parse", ref)
	if len(sha) != 40 {
		t.Fatalf("rev-parse %s 应返回 40 位 SHA, got %q", ref, sha)
	}
	return sha
}

// itNewApp 按生产装配范式构造 App（对照 NewAppServices）：真实 GitService + 提交历史缓存。
// 与生产的唯一差异：gitSvc 不注入扫描缓存（NewGitService 而非 NewGitServiceWithCache）——
// scanCache 仅服务多仓库扫描预筛（ScanGitRepos），与被测单仓库 Git 操作链无关，
// 且避免集成测试写 repo_scan_cache.json 落盘产物。
func itNewApp() *App {
	return &App{AppServices: &AppServices{
		gitSvc:             service.NewGitService(),
		commitHistoryCache: service.NewCommitHistoryCache(),
	}}
}

// itFindBranch 在分支列表中查找指定名称的本地分支，不存在返回 nil。
func itFindBranch(t *testing.T, list *model.BranchList, name string) *model.BranchInfo {
	t.Helper()
	for i := range list.Branches {
		b := &list.Branches[i]
		if !b.IsRemote && b.Name == name {
			return b
		}
	}
	return nil
}

// itNormalize 统一换行为 LF，规避个别环境 autocrlf 残留对内容断言的干扰。
func itNormalize(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// itReadFile 读取仓库内文件并统一换行后返回。
func itReadFile(t *testing.T, repoPath, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoPath, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return itNormalize(string(data))
}

// itSetupForkRepo 构造可快进分叉仓库：master 基线提交，feature 领先一个提交，切回 master。
func itSetupForkRepo(t *testing.T) string {
	t.Helper()
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "checkout", "-b", "feature")
	itWriteFile(t, repo, "a.txt", "feature\n")
	itCommitAll(t, repo, "feature change")
	testutil.RunGit(t, repo, "checkout", "master")
	return repo
}

// itSetupConflictRepo 构造真冲突仓库：master 与 feature 各改 a.txt 同一行（line2）并提交。
func itSetupConflictRepo(t *testing.T) string {
	t.Helper()
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "line1\nline2\nline3\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "checkout", "-b", "feature")
	itWriteFile(t, repo, "a.txt", "line1\nfeature-line2\nline3\n")
	itCommitAll(t, repo, "feature change line2")
	testutil.RunGit(t, repo, "checkout", "master")
	itWriteFile(t, repo, "a.txt", "line1\nmaster-line2\nline3\n")
	itCommitAll(t, repo, "master change line2")
	return repo
}

// TestIntegration_CommitFlow 提交流全链：GetLocalChanges → StageFiles → CommitFiles →
// 提交后 GetLocalChanges 清空 → GetCommitHistory 出现新提交（含缓存增量路径）。
func TestIntegration_CommitFlow(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	app := itNewApp()

	// 基线：1 条提交
	commits, err := app.GetCommitHistory(repo, 20, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory baseline: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("基线应为 1 条提交, got %d", len(commits))
	}

	// 造变更：修改已跟踪文件 + 新增未跟踪文件
	itWriteFile(t, repo, "a.txt", "modified\n")
	itWriteFile(t, repo, "new.txt", "new file\n")

	changes, err := app.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("应有 2 条本地变更, got %d: %+v", len(changes), changes)
	}

	// 暂存 a.txt：Staged 翻转，new.txt 保持未跟踪
	if err := app.StageFiles(repo, []string{"a.txt"}); err != nil {
		t.Fatalf("StageFiles: %v", err)
	}
	changes, err = app.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges after stage: %v", err)
	}
	stagedA := itFindChange(changes, "a.txt")
	if stagedA == nil || !stagedA.Staged {
		t.Errorf("a.txt 暂存后 Staged 应为 true, got %+v", stagedA)
	}
	newChange := itFindChange(changes, "new.txt")
	if newChange == nil || newChange.Staged {
		t.Errorf("new.txt 应未暂存, got %+v", newChange)
	}

	// 选择性提交 a.txt（pathspec 语义）：new.txt 不受影响
	if err := app.CommitFiles(repo, "feat: modify a", []string{"a.txt"}); err != nil {
		t.Fatalf("CommitFiles a.txt: %v", err)
	}
	changes, err = app.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges after partial commit: %v", err)
	}
	if len(changes) != 1 || changes[0].Path != "new.txt" {
		t.Errorf("选择性提交后应仅剩 new.txt, got %+v", changes)
	}

	// 提交 new.txt 后本地变更清空
	if err := app.StageFiles(repo, []string{"new.txt"}); err != nil {
		t.Fatalf("StageFiles new.txt: %v", err)
	}
	if err := app.CommitFiles(repo, "feat: add new", []string{"new.txt"}); err != nil {
		t.Fatalf("CommitFiles new.txt: %v", err)
	}
	changes, err = app.GetLocalChanges(repo)
	if err != nil {
		t.Fatalf("GetLocalChanges after full commit: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("全部提交后本地变更应为空, got %+v", changes)
	}

	// 提交历史出现新提交（第二次调用同时覆盖缓存增量 prepend 路径）
	commits, err = app.GetCommitHistory(repo, 20, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory after commits: %v", err)
	}
	if len(commits) != 3 {
		t.Fatalf("提交后历史应为 3 条, got %d", len(commits))
	}
	if got := strings.TrimSpace(commits[0].Message); got != "feat: add new" {
		t.Errorf("最新提交应为 feat: add new, got %q", got)
	}
	if got := strings.TrimSpace(commits[1].Message); got != "feat: modify a" {
		t.Errorf("第二条提交应为 feat: modify a, got %q", got)
	}
}

// itFindChange 按路径查找本地变更项，不存在返回 nil。
func itFindChange(changes []model.FileChange, path string) *model.FileChange {
	for i := range changes {
		if changes[i].Path == path {
			return &changes[i]
		}
	}
	return nil
}

// TestIntegration_PushFlow 推送流：AddRemote 指向本地 bare 仓库 → PushRepo --set-upstream →
// 直接校验 bare 仓库产物（master refs 与工作仓 HEAD 一致）+ HasUpstream 生效。
func TestIntegration_PushFlow(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	bare := itBareRemote(t)
	app := itNewApp()

	// 走 service 链配置远程（替代 fixture 裸 git remote add）
	if err := app.AddRemote(repo, "origin", bare); err != nil {
		t.Fatalf("AddRemote: %v", err)
	}

	// 推送前再追加一个提交，确保推送的是真实增量
	itWriteFile(t, repo, "b.txt", "to be pushed\n")
	itCommitAll(t, repo, "push me")

	// 推送前不应有上游分支（与 set-upstream 推送后的断言形成对照）
	hasUpstream, err := app.HasUpstream(repo)
	if err != nil {
		t.Fatalf("HasUpstream before push: %v", err)
	}
	if hasUpstream {
		t.Error("推送前不应有上游分支")
	}

	if _, err := app.PushRepo(repo, true); err != nil {
		t.Fatalf("PushRepo setUpstream: %v", err)
	}

	// 直接校验 bare 仓库产物：refs/heads/master 指向工作仓 HEAD
	headSHA := itRevParse(t, repo, "HEAD")
	bareSHA := itRevParse(t, bare, "master")
	if bareSHA != headSHA {
		t.Errorf("bare 仓库 master 应等于工作仓 HEAD: bare=%s head=%s", bareSHA, headSHA)
	}

	hasUpstream, err = app.HasUpstream(repo)
	if err != nil {
		t.Fatalf("HasUpstream after push: %v", err)
	}
	if !hasUpstream {
		t.Error("set-upstream 推送后应有上游分支")
	}
}

// TestIntegration_BranchFlow 分支管理全链：GetBranches / CreateBranch / CheckoutBranch /
// RenameBranch / DeleteBranch（含 -d 安全删除拒绝未合并分支、-D 强删）。
func TestIntegration_BranchFlow(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	app := itNewApp()

	// 初始：仅 master 且为当前分支
	list, err := app.GetBranches(repo)
	if err != nil {
		t.Fatalf("GetBranches: %v", err)
	}
	master := itFindBranch(t, list, "master")
	if master == nil || !master.IsCurrent {
		t.Fatalf("初始应仅 master 且为当前分支, got %+v", list.Branches)
	}

	// 创建分支：不切换当前分支
	if err := app.CreateBranch(repo, "feature"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	list, err = app.GetBranches(repo)
	if err != nil {
		t.Fatalf("GetBranches after create: %v", err)
	}
	feature := itFindBranch(t, list, "feature")
	if feature == nil || feature.IsCurrent {
		t.Errorf("feature 应存在且非当前分支, got %+v", feature)
	}

	// 切换到 feature 并提交独有文件
	if err := app.CheckoutBranch(repo, "feature", false); err != nil {
		t.Fatalf("CheckoutBranch feature: %v", err)
	}
	list, _ = app.GetBranches(repo)
	if b := itFindBranch(t, list, "feature"); b == nil || !b.IsCurrent {
		t.Errorf("切换后 feature 应为当前分支, got %+v", b)
	}
	itWriteFile(t, repo, "feature.txt", "feature only\n")
	itCommitAll(t, repo, "feature commit")

	// 切回 master：feature 独有文件从工作区消失（验证真实检出）
	if err := app.CheckoutBranch(repo, "master", false); err != nil {
		t.Fatalf("CheckoutBranch master: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); !os.IsNotExist(err) {
		t.Errorf("切回 master 后 feature.txt 应从工作区消失, stat err=%v", err)
	}

	// 重命名 feature → feature-v2
	if err := app.RenameBranch(repo, "feature", "feature-v2"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	list, _ = app.GetBranches(repo)
	if itFindBranch(t, list, "feature") != nil {
		t.Error("重命名后旧名 feature 不应存在")
	}
	if itFindBranch(t, list, "feature-v2") == nil {
		t.Error("重命名后 feature-v2 应存在")
	}

	// 安全删除未合并分支被拒绝（-d 语义），且拒绝原因须为「未合并」而非环境异常；
	// 文案兼容 git l10n（英文 not fully merged / 简体中文「没有完全合并」）
	err = app.DeleteBranch(repo, "feature-v2", false)
	if err == nil {
		t.Error("未合并分支安全删除应被拒绝")
	} else if !strings.Contains(err.Error(), "not fully merged") && !strings.Contains(err.Error(), "没有完全合并") {
		t.Errorf("-d 拒绝原因应为未合并, got %v", err)
	}
	if err := app.DeleteBranch(repo, "feature-v2", true); err != nil {
		t.Fatalf("DeleteBranch force: %v", err)
	}
	list, _ = app.GetBranches(repo)
	if itFindBranch(t, list, "feature-v2") != nil {
		t.Error("强删后 feature-v2 不应存在")
	}
}

// TestIntegration_MergeFlow_FF 可快进合并：master 直接前移到 feature 指针，无合并提交。
func TestIntegration_MergeFlow_FF(t *testing.T) {
	itRequireGit(t)
	repo := itSetupForkRepo(t)
	app := itNewApp()

	featureSHA := itRevParse(t, repo, "feature")

	if _, err := app.Merge(repo, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("Merge ff: %v", err)
	}

	if after := itRevParse(t, repo, "master"); after != featureSHA {
		t.Errorf("ff 合并后 master 应前移到 feature: want %s got %s", featureSHA, after)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "feature\n" {
		t.Errorf("ff 合并后 a.txt 内容应为 feature 版本, got %q", got)
	}
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeNone {
		t.Errorf("ff 合并后应无冲突态, got %s", state.Type)
	}
}

// TestIntegration_MergeFlow_NoFF 强制合并提交：产生双父合并节点，保留分支拓扑。
func TestIntegration_MergeFlow_NoFF(t *testing.T) {
	itRequireGit(t)
	repo := itSetupForkRepo(t)
	app := itNewApp()

	featureSHA := itRevParse(t, repo, "feature")
	beforeSHA := itRevParse(t, repo, "master")

	if _, err := app.Merge(repo, "feature", model.MergeModeNoFF); err != nil {
		t.Fatalf("Merge no-ff: %v", err)
	}

	afterSHA := itRevParse(t, repo, "master")
	if afterSHA == featureSHA {
		t.Error("no-ff 合并应产生新合并提交, master 不应等于 feature SHA")
	}
	// 合并提交双父：第一父为合并前 master，第二父为 feature
	if p1 := itRevParse(t, repo, "master^1"); p1 != beforeSHA {
		t.Errorf("合并提交第一父应为合并前 master: want %s got %s", beforeSHA, p1)
	}
	if p2 := itRevParse(t, repo, "master^2"); p2 != featureSHA {
		t.Errorf("合并提交第二父应为 feature: want %s got %s", featureSHA, p2)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "feature\n" {
		t.Errorf("no-ff 合并后 a.txt 内容应为 feature 版本, got %q", got)
	}
}

// TestIntegration_RebaseFlow 变基流：feature 的提交重放到 master 最新提交之上。
func TestIntegration_RebaseFlow(t *testing.T) {
	itRequireGit(t)
	repo := itInitRepo(t)
	itWriteFile(t, repo, "a.txt", "base\n")
	itCommitAll(t, repo, "base")
	testutil.RunGit(t, repo, "checkout", "-b", "feature")
	itWriteFile(t, repo, "b.txt", "feature only\n")
	itCommitAll(t, repo, "feature commit")
	testutil.RunGit(t, repo, "checkout", "master")
	itWriteFile(t, repo, "c.txt", "master only\n")
	itCommitAll(t, repo, "master commit")
	testutil.RunGit(t, repo, "checkout", "feature")
	app := itNewApp()

	if _, err := app.Rebase(repo, "master"); err != nil {
		t.Fatalf("Rebase: %v", err)
	}

	// feature 的父提交应为 master 最新（重放到其上）
	masterSHA := itRevParse(t, repo, "master")
	featureParent := itRevParse(t, repo, "feature~1")
	if featureParent != masterSHA {
		t.Errorf("变基后 feature~1 应等于 master: want %s got %s", masterSHA, featureParent)
	}
	// master 的提交与 feature 的提交同时存在
	if got := itReadFile(t, repo, "c.txt"); got != "master only\n" {
		t.Errorf("变基后 feature 应含 master 的 c.txt, got %q", got)
	}
	if got := itReadFile(t, repo, "b.txt"); got != "feature only\n" {
		t.Errorf("变基后 feature 应保留自身 b.txt, got %q", got)
	}
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeNone {
		t.Errorf("无冲突变基后应无冲突态, got %s", state.Type)
	}
}

// TestIntegration_MergeConflict_AbortMerge 真冲突场景：merge 进入冲突态（冲突不算错误）→
// GetConflictState 报告 merge 类型与冲突文件 → AbortMerge 回滚到合并前状态。
func TestIntegration_MergeConflict_AbortMerge(t *testing.T) {
	itRequireGit(t)
	repo := itSetupConflictRepo(t)
	app := itNewApp()

	beforeSHA := itRevParse(t, repo, "master")

	// 冲突以 exit 1 正常返回，service 层不当作错误（util ExecuteWithCodes 契约）
	if _, err := app.Merge(repo, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("冲突 merge 应被接受为正常返回: %v", err)
	}

	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState: %v", err)
	}
	if state.Type != model.ConflictTypeMerge {
		t.Fatalf("冲突态类型应为 merge, got %s", state.Type)
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
	content := itReadFile(t, repo, "a.txt")
	if !strings.Contains(content, "<<<<<<<") {
		t.Errorf("冲突文件应含冲突标记, got %q", content)
	}

	// 中止合并：回滚到合并前状态
	if err := app.AbortMerge(repo); err != nil {
		t.Fatalf("AbortMerge: %v", err)
	}
	state, _ = app.GetConflictState(repo)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("abort 后应无冲突态, got %s", state.Type)
	}
	if after := itRevParse(t, repo, "master"); after != beforeSHA {
		t.Errorf("abort 后 master 应回滚到合并前: want %s got %s", beforeSHA, after)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "line1\nmaster-line2\nline3\n" {
		t.Errorf("abort 后 a.txt 应恢复 master 版本, got %q", got)
	}
}

// TestIntegration_MergeConflict_ResolveContinue 真冲突解决闭环：标记解决 → ContinueMerge
// 完成合并提交 → 冲突态退出 → 历史出现合并提交。
func TestIntegration_MergeConflict_ResolveContinue(t *testing.T) {
	itRequireGit(t)
	repo := itSetupConflictRepo(t)
	app := itNewApp()

	if _, err := app.Merge(repo, "feature", model.MergeModeFF); err != nil {
		t.Fatalf("冲突 merge 应被接受: %v", err)
	}

	// 写解决内容并标记已解决（git add）
	itWriteFile(t, repo, "a.txt", "line1\nresolved\nline3\n")
	if err := app.ResolveConflict(repo, "a.txt"); err != nil {
		t.Fatalf("ResolveConflict: %v", err)
	}
	state, err := app.GetConflictState(repo)
	if err != nil {
		t.Fatalf("GetConflictState after resolve: %v", err)
	}
	if len(state.Files) != 0 {
		t.Errorf("标记解决后冲突文件列表应为空, got %v", state.Files)
	}

	// 完成合并（复用 MERGE_MSG 的 --no-edit 提交）
	if _, err := app.ContinueMerge(repo); err != nil {
		t.Fatalf("ContinueMerge: %v", err)
	}
	state, _ = app.GetConflictState(repo)
	if state.Type != model.ConflictTypeNone {
		t.Errorf("continue 后应退出冲突态, got %s", state.Type)
	}

	// 历史顶部为合并提交
	commits, err := app.GetCommitHistory(repo, 5, 0, model.CommitFilter{})
	if err != nil {
		t.Fatalf("GetCommitHistory: %v", err)
	}
	if len(commits) == 0 {
		t.Fatal("历史不应为空")
	}
	if got := strings.TrimSpace(commits[0].Message); !strings.Contains(got, "Merge branch 'feature'") {
		t.Errorf("顶部应为合并提交, got %q", got)
	}
	if got := itReadFile(t, repo, "a.txt"); got != "line1\nresolved\nline3\n" {
		t.Errorf("合并完成后 a.txt 应为解决内容, got %q", got)
	}
}
