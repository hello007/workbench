package main

import (
	"os"
	"path/filepath"
	"strings"
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

// TestGetRepoReadme_FullContent GetRepoReadme 返回仓库根目录下 README 完整文本（不截断）。
func TestGetRepoReadme_FullContent(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	workDir := t.TempDir()
	repo := filepath.Join(workDir, "myrepo")
	initRepoForTest(t, repo, false)
	full := "# 项目名\n\n第一段描述。\n\n第二段描述。"
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(full), 0644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	got := app.GetRepoReadme(repo)
	if got != full {
		t.Errorf("GetRepoReadme: got %q, want %q", got, full)
	}
}

// TestGetRepoReadme_NoReadme 无 README / 路径非目录 均返回空串。
func TestGetRepoReadme_EmptyCases(t *testing.T) {
	app, _ := repoFilterTestApp(t)
	// 无 README 的仓库
	workDir := t.TempDir()
	repo := filepath.Join(workDir, "noreadme")
	initRepoForTest(t, repo, false)
	if got := app.GetRepoReadme(repo); got != "" {
		t.Errorf("expected empty for repo without README, got %q", got)
	}
	// 路径不存在
	if got := app.GetRepoReadme(filepath.Join(workDir, "nonexistent")); got != "" {
		t.Errorf("expected empty for nonexistent path, got %q", got)
	}
	// 路径指向文件而非目录
	filePath := filepath.Join(workDir, "afile.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if got := app.GetRepoReadme(filePath); got != "" {
		t.Errorf("expected empty for non-dir path, got %q", got)
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

// TestBuildRepoFilterList_WorkdirIsolation 验证切换工作目录后，其他工作目录的已编辑仓库
// 不会以失效状态混入当前工作目录列表（repo_meta.json 按 path 全局存储，查询时按 rootPath 范围过滤）。
func TestBuildRepoFilterList_WorkdirIsolation(t *testing.T) {
	app, _ := repoFilterTestApp(t)

	// 两个独立的工作目录
	dirAPath := t.TempDir()
	dirBPath := t.TempDir()
	repoA := filepath.Join(dirAPath, "repo-a")
	repoB := filepath.Join(dirBPath, "repo-b")
	initRepoForTest(t, repoA, false)
	initRepoForTest(t, repoB, false)

	dirA := &model.Directory{ID: "dirA", Name: "dirA", Path: dirAPath}
	dirB := &model.Directory{ID: "dirB", Name: "dirB", Path: dirBPath}

	// 首次扫描 dirA 建立 repoA 元数据，并打标签使其成为"已编辑"仓库
	app.buildRepoFilterList(dirA, false)
	if err := app.SaveRepoMeta(repoA, "repoA 简述", []string{"tag-a"}); err != nil {
		t.Fatalf("SaveRepoMeta repoA: %v", err)
	}

	// 扫描 dirB：repoA 不在 dirB 范围内，不应出现（更不能以 Missing 状态混入）
	itemsB := app.buildRepoFilterList(dirB, false)
	for _, it := range itemsB {
		if it.Name == "repo-a" {
			t.Errorf("repo-a 不应出现在 dirB 的列表中（工作目录隔离），但仍得到: %+v", it)
		}
	}
	// repoB 应出现且非失效
	foundB := false
	for _, it := range itemsB {
		if it.Name == "repo-b" {
			foundB = true
			if it.Missing {
				t.Error("repo-b 不应标记为 Missing")
			}
		}
	}
	if !foundB {
		t.Error("repo-b 应出现在 dirB 的列表中")
	}

	// 扫描 dirA：repoA 应出现、Missing=false（扫描到）且保留用户标签
	itemsA := app.buildRepoFilterList(dirA, false)
	foundA := false
	for _, it := range itemsA {
		if it.Name == "repo-a" {
			foundA = true
			if it.Missing {
				t.Error("repo-a 不应标记为 Missing（扫描到）")
			}
			if len(it.Tags) != 1 || it.Tags[0] != "tag-a" {
				t.Errorf("repo-a 标签: got %v, want [tag-a]", it.Tags)
			}
		}
	}
	if !foundA {
		t.Error("repo-a 应出现在 dirA 的列表中")
	}
}

// TestBuildRepoFilterList_MissingInSameWorkdir 验证当前工作目录范围内的失效记录
// 仍被正确标记 Missing=true（保留失效清理入口，未被工作目录隔离逻辑误伤）。
func TestBuildRepoFilterList_MissingInSameWorkdir(t *testing.T) {
	app, _ := repoFilterTestApp(t)

	dirAPath := t.TempDir()
	repoA := filepath.Join(dirAPath, "repo-a")
	repoGone := filepath.Join(dirAPath, "repo-gone")
	initRepoForTest(t, repoA, false)
	initRepoForTest(t, repoGone, false)

	dirA := &model.Directory{ID: "dirA", Name: "dirA", Path: dirAPath}

	// 首次扫描建立元数据
	app.buildRepoFilterList(dirA, false)

	// 删除 repoGone，使其成为同目录内真正失效的记录
	if err := os.RemoveAll(repoGone); err != nil {
		t.Fatalf("remove repoGone: %v", err)
	}

	// 强制刷新扫描：repoGone 在 dirA 范围内，应标记 Missing=true
	items := app.buildRepoFilterList(dirA, true)
	byName := make(map[string]*model.RepoFilterItem, len(items))
	for _, it := range items {
		byName[it.Name] = it
	}
	gone, ok := byName["repo-gone"]
	if !ok {
		t.Fatal("repo-gone 应以 Missing 状态出现在 dirA 列表中（保留失效清理入口）")
	}
	if !gone.Missing {
		t.Error("repo-gone 应标记 Missing=true")
	}
	a, ok := byName["repo-a"]
	if !ok {
		t.Fatal("repo-a 应出现在 dirA 列表中")
	}
	if a.Missing {
		t.Error("repo-a 不应标记为 Missing")
	}
}

// TestIsPathUnder 直接固化 isPathUnder 的边界行为：相等、大小写、分隔符、兄弟路径不误判。
// 这些边界虽在 Windows 文件系统不敏感场景下风险低，但路径主键规范化依赖其正确性，
// 需独立测试防止未来回归（跨平台路径输入、外部传入未规范化路径等）。
func TestIsPathUnder(t *testing.T) {
	// 用绝对路径前缀构造，规避 filepath.Abs 依赖当前工作目录
	base := filepath.Join(t.TempDir(), "parent")
	child := filepath.Join(base, "child", "repo")

	cases := []struct {
		name   string
		child  string
		parent string
		want   bool
	}{
		{"child 在 parent 下", child, base, true},
		{"child == parent", base, base, true},
		{"大小写差异（盘符/路径段）", upperPath(child), upperPath(base), true},
		{"分隔符差异（\\ vs /）", toSlashPath(child), base, true},
		{"兄弟路径不误判", filepath.Join(filepath.Dir(base), "parent-sibling", "repo"), base, false},
		{"前缀同名兄弟不误判", filepath.Join(filepath.Dir(base), "parent-other"), base, false},
		{"parent 末尾带分隔符不应误判", child, base + string(filepath.Separator), true},
		{"空 parent 视为 cwd（child 不在 cwd 下时 false）", filepath.Join(t.TempDir(), "x"), "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPathUnder(c.child, c.parent); got != c.want {
				t.Errorf("isPathUnder(%q, %q) = %v, want %v", c.child, c.parent, got, c.want)
			}
		})
	}
}

// upperPath 将路径中所有 ASCII 字母转大写，用于测试大小写不敏感判定。
func upperPath(p string) string {
	return strings.ToUpper(p)
}

// toSlashPath 将路径分隔符统一为 /，用于测试分隔符差异下的判定。
func toSlashPath(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}
