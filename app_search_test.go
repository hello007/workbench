package main

import (
	"path/filepath"
	"strings"
	"testing"

	"workbench/service"
	"workbench/util/testutil"
)

// newAppWithContentSearch 构造仅含内容搜索依赖的 App（settingsSvc 空配置走默认排除项，
// contentSearchSvc 按 CI/本机 rg 可用性自选 rg 或 Go 原生路径，两路径结果语义已对齐）
func newAppWithContentSearch(t *testing.T) *App {
	t.Helper()
	return &App{AppServices: &AppServices{
		settingsSvc:      service.NewSettingsService(filepath.Join(t.TempDir(), "settings.json")),
		contentSearchSvc: service.NewContentSearchService(),
	}}
}

// TestContentSearchDirs_ReturnsHitGroup 指定目录命中：返回单组且组信息与入参目录对号
func TestContentSearchDirs_ReturnsHitGroup(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "hello keyword world\n")

	app := newAppWithContentSearch(t)
	groups, err := app.ContentSearchDirs("keyword", "", "", []string{dir}, []string{"repo-a"})
	if err != nil {
		t.Fatalf("ContentSearchDirs: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("应有 1 个结果组: got %d", len(groups))
	}
	g := groups[0]
	if g.RepoName != "repo-a" || g.RepoPath != dir {
		t.Errorf("组信息应对号入座: got %s/%s", g.RepoName, g.RepoPath)
	}
	if len(g.Items) != 1 || !strings.Contains(g.Items[0].LineText, "keyword") {
		t.Errorf("应命中 1 行且含关键词: got %+v", g.Items)
	}
	if g.Items[0].RepoPath != dir {
		t.Errorf("结果项 RepoPath 应补全为工作目录: got %s", g.Items[0].RepoPath)
	}
}

// TestContentSearchDirs_ZeroHitsReturnsEmpty 零命中返回空组列表（非错误）
func TestContentSearchDirs_ZeroHitsReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "a.txt"), "nothing here\n")

	app := newAppWithContentSearch(t)
	groups, err := app.ContentSearchDirs("missing", "", "", []string{dir}, []string{"repo-a"})
	if err != nil {
		t.Fatalf("ContentSearchDirs: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("零命中应返回空组列表: got %d", len(groups))
	}
}

// TestContentSearchDirs_EmptyKeywordOrDirs 空 keyword / 空目录列表幂等空返
func TestContentSearchDirs_EmptyKeywordOrDirs(t *testing.T) {
	app := newAppWithContentSearch(t)
	if groups, err := app.ContentSearchDirs("", "", "", []string{t.TempDir()}, []string{"r"}); err != nil || groups != nil {
		t.Errorf("空 keyword 应 nil,nil: got %v, %v", groups, err)
	}
	if groups, err := app.ContentSearchDirs("kw", "", "", nil, nil); err != nil || groups != nil {
		t.Errorf("空 dirs 应 nil,nil: got %v, %v", groups, err)
	}
}

// TestContentSearchDirs_LengthMismatch dirs 与 repoNames 数量不一致拒绝执行（防御分支）
func TestContentSearchDirs_LengthMismatch(t *testing.T) {
	app := newAppWithContentSearch(t)
	_, err := app.ContentSearchDirs("kw", "", "", []string{t.TempDir()}, nil)
	if err == nil {
		t.Fatal("数量不一致应返回错误")
	}
	if !strings.Contains(err.Error(), "数量不一致") {
		t.Errorf("错误信息应说明数量不一致: %v", err)
	}
}
