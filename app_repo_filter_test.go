package main

import (
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/service"
)

// repoFilterTestApp 构建一个用于仓库筛选器测试的 App，注入临时配置的各 service。
func repoFilterTestApp(t *testing.T) (*App, string) {
	t.Helper()
	tmp := t.TempDir()
	app := &App{
		directorySvc: service.NewDirectoryService(filepath.Join(tmp, "directories.json")),
		gitSvc:       service.NewGitServiceWithCache(filepath.Join(tmp, "repo_scan_cache.json")),
		repoMetaSvc:  service.NewRepoMetaService(filepath.Join(tmp, "repo_meta.json")),
	}
	return app, tmp
}

// initRepoForTest 在 path 处初始化一个 git 仓库（含身份配置），可选添加远程。
func initRepoForTest(t *testing.T, path string, withRemote bool) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	runGitIn(t, path, "init")
	runGitIn(t, path, "config", "user.email", "test@test.com")
	runGitIn(t, path, "config", "user.name", "test")
	if withRemote {
		runGitIn(t, path, "remote", "add", "origin", "https://example.com/repo.git")
	}
}

// TestGetRepoFilterList_BasicRepos 扫描两个仓库：有远程/无远程，返回正确的 IsGitRepo/HasRemote。
func TestGetRepoFilterList_BasicRepos(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repoA := filepath.Join(workDir, "repo-a")
	repoB := filepath.Join(workDir, "repo-b")
	initRepoForTest(t, repoA, true)  // 有远程
	initRepoForTest(t, repoB, false) // 无远程

	// 注册工作目录（service.Create 自动 filepath.Abs 规范化路径）
	created, err := app.directorySvc.Create("work", workDir, false)
	if err != nil {
		t.Fatalf("Create workdir: %v", err)
	}

	items := app.GetRepoFilterList(created.ID)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	byName := make(map[string]*model.RepoFilterItem, len(items))
	for _, it := range items {
		byName[it.Name] = it
	}
	if !byName["repo-a"].IsGitRepo {
		t.Error("repo-a should be IsGitRepo=true")
	}
	if !byName["repo-a"].HasRemote {
		t.Error("repo-a should have remote")
	}
	if byName["repo-b"].HasRemote {
		t.Error("repo-b should NOT have remote")
	}
	if byName["repo-a"].Missing {
		t.Error("repo-a should not be missing")
	}
}

// TestGetRepoFilterList_ReadmeSummary README 摘要解析并缓存到 RepoMeta。
func TestGetRepoFilterList_ReadmeSummary(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repo := filepath.Join(workDir, "myrepo")
	initRepoForTest(t, repo, false)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# 我的项目\n\n这是一个测试项目说明。"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	created, _ := app.directorySvc.Create("work", workDir, false)
	items := app.GetRepoFilterList(created.ID)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ReadmeSummary == "" {
		t.Error("expected non-empty ReadmeSummary")
	}
	// README 摘要应缓存到 RepoMeta（下次加载沿用，避免重复读盘）
	meta, _ := app.repoMetaSvc.Load()
	abs, _ := filepath.Abs(repo)
	if m, ok := meta[abs]; !ok || m.ReadmeSummary == "" {
		t.Errorf("expected ReadmeSummary cached in RepoMeta for %q, got: %+v", abs, meta)
	}
}

// TestSaveRepoMeta_LoadsBack SaveRepoMeta 后 GetRepoFilterList 能读回简述与标签。
func TestSaveRepoMeta_LoadsBack(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repo := filepath.Join(workDir, "myrepo")
	initRepoForTest(t, repo, false)

	created, _ := app.directorySvc.Create("work", workDir, false)
	app.GetRepoFilterList(created.ID) // 首次扫描建立元数据

	if err := app.SaveRepoMeta(repo, "我的简述", []string{"tag1", "tag2"}); err != nil {
		t.Fatalf("SaveRepoMeta: %v", err)
	}

	items := app.GetRepoFilterList(created.ID)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Summary != "我的简述" {
		t.Errorf("Summary: got %q, want %q", items[0].Summary, "我的简述")
	}
	if len(items[0].Tags) != 2 || items[0].Tags[0] != "tag1" || items[0].Tags[1] != "tag2" {
		t.Errorf("Tags: got %v, want [tag1 tag2]", items[0].Tags)
	}
}

// TestGetRepoFilterList_MissingRepo 仓库被删除后扫描标记 Missing=true（灰显）。
func TestGetRepoFilterList_MissingRepo(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repoA := filepath.Join(workDir, "repo-a")
	repoB := filepath.Join(workDir, "repo-b")
	initRepoForTest(t, repoA, false)
	initRepoForTest(t, repoB, false)

	created, _ := app.directorySvc.Create("work", workDir, false)
	app.GetRepoFilterList(created.ID) // 首次扫描建立元数据

	if err := os.RemoveAll(repoB); err != nil {
		t.Fatalf("remove repoB: %v", err)
	}
	// 强制刷新扫描（绕过缓存重新判定）
	items := app.RefreshRepoFilterList(created.ID)

	byName := make(map[string]*model.RepoFilterItem, len(items))
	for _, it := range items {
		byName[it.Name] = it
	}
	if _, ok := byName["repo-a"]; !ok {
		t.Error("repo-a should still be present")
	}
	missingB, ok := byName["repo-b"]
	if !ok {
		t.Fatal("repo-b should still appear (as missing)")
	}
	if !missingB.Missing {
		t.Error("repo-b should be marked Missing=true after deletion")
	}
}

// TestCleanMissingRepoMeta 清理失效记录：删除已不存在的仓库元数据。
func TestCleanMissingRepoMeta(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repoA := filepath.Join(workDir, "repo-a")
	repoB := filepath.Join(workDir, "repo-b")
	initRepoForTest(t, repoA, false)
	initRepoForTest(t, repoB, false)

	created, _ := app.directorySvc.Create("work", workDir, false)
	app.GetRepoFilterList(created.ID)

	os.RemoveAll(repoB)
	app.RefreshRepoFilterList(created.ID) // 标记 repoB 失效

	removed, err := app.CleanMissingRepoMeta()
	if err != nil {
		t.Fatalf("CleanMissingRepoMeta: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed count: got %d, want 1", removed)
	}

	// 再次扫描：repoB 不应再出现（元数据已清理）
	items := app.RefreshRepoFilterList(created.ID)
	for _, it := range items {
		if it.Name == "repo-b" {
			t.Errorf("repo-b should be removed after CleanMissingRepoMeta, still got: %+v", it)
		}
	}
}

// TestGetRepoFilterList_UnknownDirId 未知 dirId 返回空切片不报错。
func TestGetRepoFilterList_UnknownDirId(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	items := app.GetRepoFilterList("nonexistent-id")
	if len(items) != 0 {
		t.Errorf("expected empty for unknown dirId, got %d items", len(items))
	}
}

// TestGetRepoFilterList_NoRepos 工作目录下无仓库时返回空列表。
func TestGetRepoFilterList_NoRepos(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	created, _ := app.directorySvc.Create("work", workDir, false)
	items := app.GetRepoFilterList(created.ID)
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty workdir, got %d", len(items))
	}
}

// TestGetRepoFilterList_ReadmeCache 常规重复扫描沿用缓存的 README 摘要（不重读盘，PRD F17），
// 仅手动刷新（RefreshRepoFilterList）才重新解析。修改 README 后常规扫描应返回旧缓存，刷新返回新内容。
func TestGetRepoFilterList_ReadmeCache(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repo := filepath.Join(workDir, "myrepo")
	initRepoForTest(t, repo, false)
	readmePath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readmePath, []byte("原始README内容"), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	created, _ := app.directorySvc.Create("work", workDir, false)
	// 首次扫描：解析并缓存 README
	items := app.GetRepoFilterList(created.ID)
	if len(items) != 1 || !stringsContains(items[0].ReadmeSummary, "原始README内容") {
		t.Fatalf("first scan summary: got %+v", items)
	}

	// 修改 README 内容
	if err := os.WriteFile(readmePath, []byte("全新README内容"), 0644); err != nil {
		t.Fatalf("rewrite readme: %v", err)
	}

	// 常规扫描（非强制）：应沿用缓存，返回旧内容（F17 缓存语义）
	items = app.GetRepoFilterList(created.ID)
	if len(items) != 1 {
		t.Fatalf("routine scan: expected 1 item, got %d", len(items))
	}
	if !stringsContains(items[0].ReadmeSummary, "原始README内容") {
		t.Errorf("routine scan should use cached README, got: %q", items[0].ReadmeSummary)
	}

	// 手动刷新（强制）：应重新解析，返回新内容
	items = app.RefreshRepoFilterList(created.ID)
	if len(items) != 1 {
		t.Fatalf("refresh scan: expected 1 item, got %d", len(items))
	}
	if !stringsContains(items[0].ReadmeSummary, "全新README内容") {
		t.Errorf("refresh scan should re-parse README, got: %q", items[0].ReadmeSummary)
	}
}

// stringsContains 避免 main 包测试引入 strings 包的别名冲突（app_test.go 已有 strings 引用）。
func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && indexOfSub(s, sub) >= 0
}

// indexOfSub 子串查找，未找到返回 -1。
func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
