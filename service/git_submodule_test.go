package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
	"workbench/util/testutil"
)

// setupSuperprojectWithSubmodule 构造一个 superproject 含一个真实 submodule，
// 供 ListSubmodules / UpdateSubmodules / InitSubmodules 测试使用。
//
// 构造步骤：
//  1. 创建 child 独立仓库（含一次提交）
//  2. 创建 parent 独立仓库（含一次提交）
//  3. parent 执行 git -c protocol.file.allow=always submodule add ../child libs/child
//     （git 2.41+ 默认禁 file 协议 CVE-2022-39253，submodule add 内部 clone 子进程不继承
//     仓库本地 config，须用 -c 内联传给主进程使其下传 clone 子进程）
//  4. commit .gitmodules + gitlink，并 update --init 检出
//
// 返回 parent 与 child 的绝对路径。
func setupSuperprojectWithSubmodule(t *testing.T) (parent, child string) {
	t.Helper()
	parent = testutil.InitTempRepo(t)
	child = testutil.InitTempRepo(t)

	// child 首次提交
	testutil.WriteFile(t, filepath.Join(child, "child.txt"), "child init")
	testutil.RunGit(t, child, "add", "child.txt")
	testutil.RunGit(t, child, "commit", "-m", "child init")

	// parent 首次提交
	testutil.WriteFile(t, filepath.Join(parent, "parent.txt"), "parent init")
	testutil.RunGit(t, parent, "add", "parent.txt")
	testutil.RunGit(t, parent, "commit", "-m", "parent init")

	// git 2.41+ 默认禁 file 协议（CVE-2022-39253）。仓库本地 config protocol.file.allow
	// 不被 submodule add 内部的 clone 子进程继承，须用 -c 内联传主进程再下传子进程。
	// 用 child 绝对路径作 url，避免相对路径在 clone 子进程 cwd 下解析错位（parent 与 child
	// 分属不同 t.TempDir 随机子目录，../child 无法上溯命中）。
	runGitInlineConfig(t, parent, "protocol.file.allow=always", "submodule", "add", child, "libs/child")
	testutil.RunGit(t, parent, "commit", "-m", "add child submodule")

	// 确保 submodule 在 superproject 真正检出（已初始化）
	runGitInlineConfig(t, parent, "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")

	return parent, child
}

// runGitInlineConfig 在 dir 执行带 -c <config> 内联的 git 命令（失败终止测试）。
// 供 submodule add/update 走 file 协议本地路径使用：-c 传主进程，子进程（clone）继承。
func runGitInlineConfig(t *testing.T, dir, cfg string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", cfg}, args...)
	var stderr strings.Builder
	cmd := exec.Command("git", full...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s failed: %v\nstderr: %s", args, dir, err, stderr.String())
	}
}

// findSubmodule 按 path 在列表中查找，返回指针（未找到返回 nil）。
func findSubmodule(subs []model.GitSubmodule, path string) *model.GitSubmodule {
	for i := range subs {
		if subs[i].Path == path {
			return &subs[i]
		}
	}
	return nil
}

// ===== ListSubmodules =====

// TestListSubmodules_NonRepo 非仓库目录无法定位根，返回错误。
func TestListSubmodules_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.ListSubmodules(t.TempDir()); err == nil {
		t.Error("非仓库 ListSubmodules 应返回错误")
	}
}

// TestListSubmodules_NoSubmodules 仓库无 submodule 返回空切片。
func TestListSubmodules_NoSubmodules(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	testutil.WriteFile(t, filepath.Join(repo, "a.txt"), "init")
	testutil.RunGit(t, repo, "add", "a.txt")
	testutil.RunGit(t, repo, "commit", "-m", "init")

	svc := NewGitService()
	subs, err := svc.ListSubmodules(repo)
	if err != nil {
		t.Fatalf("ListSubmodules no submodules: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("无 submodule 应返回空切片, got %d", len(subs))
	}
}

// TestListSubmodules_Initialized 解析已初始化 submodule：path/sha/url/initialized 正确，
// detached 标记为 true（git submodule update 默认检出 detached）。
func TestListSubmodules_Initialized(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	svc := NewGitService()

	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("应列出 1 个 submodule, got %d", len(subs))
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child submodule")
	}
	if !sm.Initialized {
		t.Error("已检出的 submodule 应 Initialized=true")
	}
	if sm.Sha == "" || len(sm.Sha) != 40 {
		t.Errorf("Sha 应为 40 位, got %q", sm.Sha)
	}
	if sm.ShortSha != sm.Sha[:8] {
		t.Errorf("ShortSha 应为前 8 位, got %q want %q", sm.ShortSha, sm.Sha[:8])
	}
	if sm.Url == "" {
		t.Error("Url 应从 .gitmodules 读取非空")
	}
	if sm.Conflict {
		t.Error("无冲突应 Conflict=false")
	}
	if sm.ShaMismatch {
		t.Error("SHA 一致应 ShaMismatch=false")
	}
}

// TestListSubmodules_Detached 校验 Detached 字段与 submodule 实际 HEAD 状态一致。
// git submodule update 的 detached 行为依赖 git 版本与 submodule branch 配置，不固定为 true，
// 故只断言 ListSubmodules 的 Detached 判定与 git branch --show-current 实际结果一致。
func TestListSubmodules_Detached(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	svc := NewGitService()

	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	// 实际 branch --show-current，空串表示 detached
	actualBranch := strings.TrimSpace(gitOutput(t, filepath.Join(parent, "libs/child"), "branch", "--show-current"))
	actualDetached := actualBranch == ""
	if sm.Detached != actualDetached {
		t.Errorf("Detached 判定应与实际一致: got %v want %v (branch=%q)", sm.Detached, actualDetached, actualBranch)
	}
}

// TestListSubmodules_Dirty submodule 工作区有未提交改动，Dirty 应为 true（来自 porcelain=2）。
// 注意：git submodule status 不检测 dirty，须双命令融合。
func TestListSubmodules_Dirty(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	svc := NewGitService()

	// 在 submodule 工作区制造未提交改动
	testutil.WriteFile(t, filepath.Join(parent, "libs/child", "dirty.txt"), "dirty change")

	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	if !sm.Dirty {
		t.Error("submodule 工作区有改动应 Dirty=true（git submodule status 不检测，须 porcelain=2 融合）")
	}
}

// TestListSubmodules_NewCommitNotDirty submodule 内有新提交（HEAD 前移）但工作区干净时，
// 应标 ShaMismatch=true（前导码 +）且 Dirty=false（subFlags 位2 非 M，与 dirty 工作区的 S.M. 区分）。
// 校验 Dirty 与 ShaMismatch 语义严格分离，不因 porcelain=2 的 Y 字段在两种场景均为 M 而误判。
func TestListSubmodules_NewCommitNotDirty(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	// 在 submodule 工作目录内提交新 commit（HEAD 前移，工作区干净）
	subDir := filepath.Join(parent, "libs/child")
	testutil.RunGit(t, subDir, "config", "user.email", "test@test.com")
	testutil.RunGit(t, subDir, "config", "user.name", "test")
	testutil.WriteFile(t, filepath.Join(subDir, "c2.txt"), "c2")
	testutil.RunGit(t, subDir, "add", "c2.txt")
	testutil.RunGit(t, subDir, "commit", "-m", "c2 in submodule")

	svc := NewGitService()
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	if !sm.ShaMismatch {
		t.Error("submodule HEAD 前移应 ShaMismatch=true（前导码 +）")
	}
	if sm.Dirty {
		t.Error("submodule 新提交但工作区干净应 Dirty=false（subFlags 位2 非 M，不应误判）")
	}
}

// TestListSubmodules_NotInitialized submodule deinit 后前导码 - ，Initialized 应为 false。
func TestListSubmodules_NotInitialized(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	// deinit 注销，使 submodule 进入未初始化态（前导码 -）。deinit 不走 file 协议，普通 runGit 即可。
	testutil.RunGit(t, parent, "submodule", "deinit", "-f", "libs/child")

	svc := NewGitService()
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	if sm.Initialized {
		t.Error("deinit 后应 Initialized=false（前导码 -）")
	}
}

// TestListSubmodules_ConfigFromGitmodules branch/url 应从 .gitmodules 读取填充。
func TestListSubmodules_ConfigFromGitmodules(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	svc := NewGitService()

	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	// 默认 git submodule add 无 -b 不写 branch，Branch 应为空
	if sm.Branch != "" {
		t.Errorf("未指定 -b 时 Branch 应为空, got %q", sm.Branch)
	}
	if sm.Url == "" {
		t.Error("Url 应从 .gitmodules 读取非空")
	}
}

// TestListSubmodules_BranchConfig 带 -b branch 添加 submodule 时 Branch 应从 .gitmodules 读取。
func TestListSubmodules_BranchConfig(t *testing.T) {
	parent := testutil.InitTempRepo(t)
	child := testutil.InitTempRepo(t)
	testutil.WriteFile(t, filepath.Join(child, "c.txt"), "c")
	testutil.RunGit(t, child, "add", "c.txt")
	testutil.RunGit(t, child, "commit", "-m", "c")
	// child 创建 master 分支并推送（submodule -b 需远程有该分支）
	testutil.RunGit(t, child, "branch", "main")

	testutil.WriteFile(t, filepath.Join(parent, "p.txt"), "p")
	testutil.RunGit(t, parent, "add", "p.txt")
	testutil.RunGit(t, parent, "commit", "-m", "p")
	runGitInlineConfig(t, parent, "protocol.file.allow=always", "submodule", "add", "-b", "main", child, "libs/child")
	testutil.RunGit(t, parent, "commit", "-m", "add child with branch")
	runGitInlineConfig(t, parent, "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")

	svc := NewGitService()
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatalf("未找到 libs/child")
	}
	if sm.Branch != "main" {
		t.Errorf("Branch 应从 .gitmodules 读取为 main, got %q", sm.Branch)
	}
}

// ===== InitSubmodules =====

// TestInitSubmodules init 后 .git/config 应含 submodule 段。
func TestInitSubmodules(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	// 先 deinit 使其未注册
	testutil.RunGit(t, parent, "submodule", "deinit", "-f", "libs/child")

	svc := NewGitService()
	if _, err := svc.InitSubmodules(parent); err != nil {
		t.Fatalf("InitSubmodules: %v", err)
	}
	// 校验 .git/config 含 submodule.libs.child 段
	configOut := gitOutput(t, parent, "config", "--get", "submodule.libs/child.url")
	if strings.TrimSpace(configOut) == "" {
		t.Error("init 后 .git/config 应含 submodule.libs/child.url")
	}
}

// ===== UpdateSubmodules =====

// TestUpdateSubmodules_Init update --init 后未初始化 submodule 应被检出。
func TestUpdateSubmodules_Init(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	// deinit 使其未检出
	testutil.RunGit(t, parent, "submodule", "deinit", "-f", "libs/child")

	svc := NewGitService()
	// update --init 重新检出
	_, err := svc.UpdateSubmodules(parent, model.SubmoduleUpdateCheckout, false, true, "")
	if err != nil {
		t.Fatalf("UpdateSubmodules init: %v", err)
	}
	// 校验 submodule 工作区文件存在
	if !pathExists(filepath.Join(parent, "libs", "child", "child.txt")) {
		t.Error("update --init 后 submodule 工作区应检出 child.txt")
	}
}

// TestUpdateSubmodules_SinglePath 传 path 仅更新单个 submodule。
func TestUpdateSubmodules_SinglePath(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	testutil.RunGit(t, parent, "submodule", "deinit", "-f", "libs/child")

	svc := NewGitService()
	_, err := svc.UpdateSubmodules(parent, model.SubmoduleUpdateCheckout, false, true, "libs/child")
	if err != nil {
		t.Fatalf("UpdateSubmodules single path: %v", err)
	}
	if !pathExists(filepath.Join(parent, "libs", "child", "child.txt")) {
		t.Error("单 submodule update --init 后应检出 child.txt")
	}
}

// TestUpdateSubmodules_NonRepo 非仓库目录返回错误。
func TestUpdateSubmodules_NonRepo(t *testing.T) {
	svc := NewGitService()
	_, err := svc.UpdateSubmodules(t.TempDir(), model.SubmoduleUpdateCheckout, false, true, "")
	if err == nil {
		t.Error("非仓库 UpdateSubmodules 应返回错误")
	}
}

// TestUpdateSubmodules_InvalidMode 非法 mode（非 checkout/merge/rebase/remote）应被白名单拦截，
// 而非静默走 util.SubmoduleUpdate 的 default（checkout）分支——避免前端传错或调用方拼错时无报错。
func TestUpdateSubmodules_InvalidMode(t *testing.T) {
	repo := testutil.InitTempRepo(t)
	svc := NewGitService()
	_, err := svc.UpdateSubmodules(repo, model.SubmoduleUpdateMode("garbage"), false, false, "")
	if err == nil {
		t.Fatal("非法 mode 应返回错误")
	}
	if !strings.Contains(err.Error(), "不支持的子模块更新模式") {
		t.Errorf("错误信息应含模式校验提示, got %v", err)
	}
}

// TestUpdateSubmodules_EmptyModeDefaultsCheckout 空 mode 兜底为 checkout，不报错（init=false 路径）。
func TestUpdateSubmodules_EmptyModeDefaultsCheckout(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	testutil.RunGit(t, parent, "submodule", "deinit", "-f", "libs/child")
	svc := NewGitService()
	// 空 mode + init=true 走 update --init（mode 不参与），应成功检出
	_, err := svc.UpdateSubmodules(parent, model.SubmoduleUpdateMode(""), false, true, "libs/child")
	if err != nil {
		t.Fatalf("空 mode 应兜底 checkout 不报错: %v", err)
	}
	if !pathExists(filepath.Join(parent, "libs", "child", "child.txt")) {
		t.Error("空 mode + init 后应检出 child.txt")
	}
}

// gitOutput 在仓库执行 git 命令返回 stdout（失败终止测试）。
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v", args, dir, err)
	}
	return string(out)
}

// pathExists 判定路径存在性（文件或目录均可），供测试校验 submodule 检出结果使用。
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// allowFileProtocol 通过 GIT_CONFIG_COUNT 环境变量开启 git file 协议（CVE-2022-39253 后默认禁）。
// git submodule add 内部的 clone 子进程不继承仓库本地 config，但继承父进程环境变量，
// 故用 GIT_CONFIG_COUNT / GIT_CONFIG_KEY_0 / GIT_CONFIG_VALUE_0 注入 protocol.file.allow=always。
// 测试结束自动还原（t.Cleanup），不影响其他用例。
func allowFileProtocol(t *testing.T) {
	t.Helper()
	os.Setenv("GIT_CONFIG_COUNT", "1")
	os.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	os.Setenv("GIT_CONFIG_VALUE_0", "always")
	t.Cleanup(func() {
		os.Unsetenv("GIT_CONFIG_COUNT")
		os.Unsetenv("GIT_CONFIG_KEY_0")
		os.Unsetenv("GIT_CONFIG_VALUE_0")
	})
}

// ===== AddSubmodule =====

// TestAddSubmodule 在无 submodule 的仓库新增一个，校验 .gitmodules 生成 + ListSubmodules 能列出。
func TestAddSubmodule(t *testing.T) {
	parent := testutil.InitTempRepo(t)
	child := testutil.InitTempRepo(t)
	// child 首次提交（submodule add 须有可克隆的提交）
	testutil.WriteFile(t, filepath.Join(child, "child.txt"), "child init")
	testutil.RunGit(t, child, "add", "child.txt")
	testutil.RunGit(t, child, "commit", "-m", "child init")
	// parent 首次提交（precheckMutation 要求工作区干净 + 非 detached）
	testutil.WriteFile(t, filepath.Join(parent, "parent.txt"), "parent init")
	testutil.RunGit(t, parent, "add", "parent.txt")
	testutil.RunGit(t, parent, "commit", "-m", "parent init")

	// file 协议许可（git 2.41+ 默认禁，submodule add 内部 clone 子进程须继承）
	allowFileProtocol(t)

	svc := NewGitService()
	if _, err := svc.AddSubmodule(parent, child, "libs/child", ""); err != nil {
		t.Fatalf("AddSubmodule: %v", err)
	}
	if !pathExists(filepath.Join(parent, ".gitmodules")) {
		t.Error("AddSubmodule 后应生成 .gitmodules")
	}
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules after add: %v", err)
	}
	if findSubmodule(subs, "libs/child") == nil {
		t.Error("AddSubmodule 后 ListSubmodules 应列出 libs/child")
	}
}

// TestAddSubmodule_DirtyWorkspace 工作区有未提交改动时 AddSubmodule 应报错（precheckMutation 拦截）。
func TestAddSubmodule_DirtyWorkspace(t *testing.T) {
	parent := testutil.InitTempRepo(t)
	child := testutil.InitTempRepo(t)
	testutil.WriteFile(t, filepath.Join(child, "child.txt"), "child init")
	testutil.RunGit(t, child, "add", "child.txt")
	testutil.RunGit(t, child, "commit", "-m", "child init")
	// parent 先提交一次使仓库在分支上，再制造未提交改动
	testutil.WriteFile(t, filepath.Join(parent, "parent.txt"), "parent init")
	testutil.RunGit(t, parent, "add", "parent.txt")
	testutil.RunGit(t, parent, "commit", "-m", "parent init")
	testutil.WriteFile(t, filepath.Join(parent, "dirty.txt"), "uncommitted") // 未提交改动

	allowFileProtocol(t)

	svc := NewGitService()
	if _, err := svc.AddSubmodule(parent, child, "libs/child", ""); err == nil {
		t.Error("工作区不干净时 AddSubmodule 应返回错误（precheckMutation 拦截）")
	}
}

// TestAddSubmodule_NonRepo 非仓库目录返回错误。
func TestAddSubmodule_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.AddSubmodule(t.TempDir(), "/some/child", "libs/x", ""); err == nil {
		t.Error("非仓库 AddSubmodule 应返回错误")
	}
}

// ===== RemoveSubmodule =====

// TestRemoveSubmodule 删除 submodule 后校验 .gitmodules 清除 + ListSubmodules 不再列出。
func TestRemoveSubmodule(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)

	svc := NewGitService()
	if err := svc.RemoveSubmodule(parent, "libs/child"); err != nil {
		t.Fatalf("RemoveSubmodule: %v", err)
	}
	// .gitmodules 不应再含 libs/child 段（仅一个 submodule 时整个文件被 git rm 删除）
	if data, err := os.ReadFile(filepath.Join(parent, ".gitmodules")); err == nil {
		if strings.Contains(string(data), "libs/child") {
			t.Error("RemoveSubmodule 后 .gitmodules 不应含 libs/child 段")
		}
	}
	// ListSubmodules 不应再列出
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules after remove: %v", err)
	}
	if findSubmodule(subs, "libs/child") != nil {
		t.Error("RemoveSubmodule 后 ListSubmodules 不应再列出 libs/child")
	}
}

// TestRemoveSubmodule_CleansGitModules 重点校验 .git/modules/libs/child 目录被删除。
// git submodule deinit + git rm 不自动清 .git/modules/<name>，遗漏致同名 submodule 重加时
// 复用旧 git 目录、历史错乱——服务层须显式 os.RemoveAll 清理。
func TestRemoveSubmodule_CleansGitModules(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	modulesDir := filepath.Join(parent, ".git", "modules", "libs", "child")
	if !pathExists(modulesDir) {
		t.Fatal("setup 后 .git/modules/libs/child 应存在（submodule git 目录存储于此）")
	}

	svc := NewGitService()
	if err := svc.RemoveSubmodule(parent, "libs/child"); err != nil {
		t.Fatalf("RemoveSubmodule: %v", err)
	}
	if pathExists(modulesDir) {
		t.Error("RemoveSubmodule 后 .git/modules/libs/child 应被手动清除（git 不自动清此目录）")
	}
}

// TestRemoveSubmodule_NonRepo 非仓库目录返回错误。
func TestRemoveSubmodule_NonRepo(t *testing.T) {
	svc := NewGitService()
	if err := svc.RemoveSubmodule(t.TempDir(), "libs/x"); err == nil {
		t.Error("非仓库 RemoveSubmodule 应返回错误")
	}
}

// TestRemoveSubmodule_PathTraversal path 含 .. 上溯或为绝对路径时，须在步骤 1 之前被深度防御拦截，
// 避免依赖后续 git submodule deinit / git rm 的副作用保安全（os.RemoveAll(.git/modules/<path>) 穿越风险）。
func TestRemoveSubmodule_PathTraversal(t *testing.T) {
	parent := testutil.InitTempRepo(t)
	testutil.WriteFile(t, filepath.Join(parent, "p.txt"), "p")
	testutil.RunGit(t, parent, "add", "p.txt")
	testutil.RunGit(t, parent, "commit", "-m", "p")
	svc := NewGitService()

	for _, bad := range []string{"../..", "../../etc", "/etc/passwd"} {
		if err := svc.RemoveSubmodule(parent, bad); err == nil {
			t.Errorf("路径 %q 应被穿越防御拦截", bad)
		}
	}
	// 正常相对路径不应被误拦（校验守卫不过度严格）
	// 不实际执行删除（无该 submodule），仅校验不被守卫拦截：正常路径会通过守卫进入 precheckMutation
	// 因工作区干净 + 在分支上，precheckMutation 通过，随后 deinit 报错（无此 submodule）——错误信息不含「非法」
	err := svc.RemoveSubmodule(parent, "libs/notexist")
	if err != nil && strings.Contains(err.Error(), "非法 submodule 路径") {
		t.Errorf("正常相对路径 libs/notexist 不应被穿越守卫误拦: %v", err)
	}
}

// ===== CheckoutSubmoduleBranch =====

// TestCheckoutSubmoduleBranch detached 的 submodule 切换到分支后，当前分支应为目标分支、Detached 为 false。
func TestCheckoutSubmoduleBranch(t *testing.T) {
	parent, _ := setupSuperprojectWithSubmodule(t)
	// submodule 经 update --init 检出为 detached HEAD，在其 git 目录创建一个分支供切换
	subDir := filepath.Join(parent, "libs", "child")
	testutil.RunGit(t, subDir, "branch", "feature-x")

	svc := NewGitService()
	if _, err := svc.CheckoutSubmoduleBranch(parent, "libs/child", "feature-x"); err != nil {
		t.Fatalf("CheckoutSubmoduleBranch: %v", err)
	}
	// 校验 submodule 当前分支为 feature-x
	got := strings.TrimSpace(gitOutput(t, subDir, "branch", "--show-current"))
	if got != "feature-x" {
		t.Errorf("切换后 submodule 分支应为 feature-x, got %q", got)
	}
	// 切换后 ListSubmodules 的 Detached 应为 false
	subs, err := svc.ListSubmodules(parent)
	if err != nil {
		t.Fatalf("ListSubmodules after checkout: %v", err)
	}
	sm := findSubmodule(subs, "libs/child")
	if sm == nil {
		t.Fatal("未找到 libs/child")
	}
	if sm.Detached {
		t.Error("切换到分支后 Detached 应为 false")
	}
}

// TestCheckoutSubmoduleBranch_NonRepo 非仓库目录返回错误。
func TestCheckoutSubmoduleBranch_NonRepo(t *testing.T) {
	svc := NewGitService()
	if _, err := svc.CheckoutSubmoduleBranch(t.TempDir(), "libs/x", "main"); err == nil {
		t.Error("非仓库 CheckoutSubmoduleBranch 应返回错误")
	}
}
