//go:build integration

// 子模块管理流程集成测试（真实 git 仓库 + 真实 submodule clone）。
//
// 与单测（service/git_submodule_test.go）的区别：本文件用 t.TempDir() 构造
// 父仓库 + 子仓库，走真实 `git submodule add` 产生标准 .git/modules/<path>
// clone 结构（无任何 fake），验证 App 链（GetSubmodules / UpdateSubmodules /
// AddSubmodule / RemoveSubmodule）在真实 submodule 布局下的业务正确性。
//
// 运行方式：go test -tags=integration ./...
package main

import (
	"path/filepath"
	"strings"
	"testing"

	"workbench/model"
)

// itSetupSubmoduleRepo 构造带一个已初始化 submodule 的父仓库。
//
// 结构：parent（work-repo）持有 submodule libs/sub，指向本地子仓库 sub-repo；
// submodule add 使用本地路径（经 filepath.ToSlash 规避 Windows 反斜杠差异），
// 不依赖网络。父仓库与子仓库均沿用 itInitRepo 的确定性配置。
func itSetupSubmoduleRepo(t *testing.T) string {
	t.Helper()

	// git ≥2.38.1 默认禁 file transport（CVE-2022-22907 相关修复）；submodule clone
	// 子进程不读父仓库局部 config（安全设计），须环境变量注入 config——测试进程
	// t.Setenv 后所有 git 子进程（fixture 命令 + 被测 App 链内部调用）统一继承放行
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")

	parent := itInitRepo(t)
	itWriteFile(t, parent, "main.txt", "parent content")
	itCommitAll(t, parent, "init parent")

	sub := itInitRepo(t)
	itWriteFile(t, sub, "sub.txt", "sub content")
	itCommitAll(t, sub, "init sub")

	// git ≥2.38.1 默认禁 file transport（CVE-2022-22907 相关修复）；环境变量已在
	// 函数头统一注入，submodule add 直接使用本地路径
	itGitOut(t, parent, "submodule", "add", filepath.ToSlash(sub), "libs/sub")
	itCommitAll(t, parent, "add submodule libs/sub")

	return parent
}

// itFindSubmodule 按路径查找子模块（找不到即 fail）。
func itFindSubmodule(t *testing.T, subs []model.GitSubmodule, path string) *model.GitSubmodule {
	t.Helper()
	for i := range subs {
		if subs[i].Path == path {
			return &subs[i]
		}
	}
	t.Fatalf("submodule %q 不在列表中: %+v", path, subs)
	return nil
}

func TestIntegration_SubmoduleFlow(t *testing.T) {
	itRequireGit(t)

	repoPath := itSetupSubmoduleRepo(t)
	app := itNewApp()

	// 1. List：真实 submodule 布局解析出 path/url/initialized
	subs, err := app.GetSubmodules(repoPath)
	if err != nil {
		t.Fatalf("GetSubmodules: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("应解析出 1 个 submodule, got %d: %+v", len(subs), subs)
	}
	sm := itFindSubmodule(t, subs, "libs/sub")
	if !sm.Initialized {
		t.Errorf("libs/sub 应为已初始化, got %+v", sm)
	}
	if sm.Url == "" || !strings.Contains(sm.Url, "work-repo") {
		t.Errorf("libs/sub url 应来自 .gitmodules 且含子仓库路径, got %q", sm.Url)
	}
	if len(sm.Sha) != 40 {
		t.Errorf("libs/sub sha 应为 40 位, got %q", sm.Sha)
	}

	// 2. Update：checkout 模式更新指定子模块，命令应成功
	if _, err := app.UpdateSubmodules(repoPath, model.SubmoduleUpdateCheckout, false, false, "libs/sub"); err != nil {
		t.Fatalf("UpdateSubmodules(checkout): %v", err)
	}

	// 3. Add：走 App 链添加第二个 submodule（本地路径 URL）
	sub2 := itInitRepo(t)
	itWriteFile(t, sub2, "sub2.txt", "sub2 content")
	itCommitAll(t, sub2, "init sub2")
	if _, err := app.AddSubmodule(repoPath, filepath.ToSlash(sub2), "libs/sub2", ""); err != nil {
		t.Fatalf("AddSubmodule: %v", err)
	}

	// RemoveSubmodule 前置校验工作区干净，提交 Add 引入的 .gitmodules/新增目录变更
	itCommitAll(t, repoPath, "add submodule libs/sub2")

	subs, err = app.GetSubmodules(repoPath)
	if err != nil {
		t.Fatalf("GetSubmodules after add: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("添加后应有 2 个 submodule, got %d: %+v", len(subs), subs)
	}
	itFindSubmodule(t, subs, "libs/sub2")

	// 4. Remove：deinit + git rm + 清理 .git/modules，列表回落到 1 个
	if err := app.RemoveSubmodule(repoPath, "libs/sub2"); err != nil {
		t.Fatalf("RemoveSubmodule: %v", err)
	}

	subs, err = app.GetSubmodules(repoPath)
	if err != nil {
		t.Fatalf("GetSubmodules after remove: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("删除后应剩 1 个 submodule, got %d: %+v", len(subs), subs)
	}
	itFindSubmodule(t, subs, "libs/sub")
}

func TestIntegration_SubmoduleInitFlow(t *testing.T) {
	itRequireGit(t)

	repoPath := itSetupSubmoduleRepo(t)
	app := itNewApp()

	// 手工 deinit 模拟未初始化态（前导码 -），走 App 链全量初始化恢复
	itGitOut(t, repoPath, "submodule", "deinit", "-f", "libs/sub")

	subs, err := app.GetSubmodules(repoPath)
	if err != nil {
		t.Fatalf("GetSubmodules: %v", err)
	}
	sm := itFindSubmodule(t, subs, "libs/sub")
	if sm.Initialized {
		t.Fatalf("deinit 后 libs/sub 应为未初始化, got %+v", sm)
	}

	// 全量初始化（checkout + recursive + init，对齐前端「初始化」按钮链路）
	if _, err := app.UpdateSubmodules(repoPath, model.SubmoduleUpdateCheckout, true, true, ""); err != nil {
		t.Fatalf("UpdateSubmodules(init): %v", err)
	}

	subs, err = app.GetSubmodules(repoPath)
	if err != nil {
		t.Fatalf("GetSubmodules after init: %v", err)
	}
	sm = itFindSubmodule(t, subs, "libs/sub")
	if !sm.Initialized {
		t.Errorf("初始化后 libs/sub 应恢复已初始化, got %+v", sm)
	}
	if _, err := app.UpdateSubmodules(repoPath, model.SubmoduleUpdateCheckout, true, true, ""); err != nil {
		t.Fatalf("UpdateSubmodules(init) retry: %v", err)
	}
}
