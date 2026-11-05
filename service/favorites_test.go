package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"workbench/model"
	"workbench/util"
)

func createFavoritesTestService(t *testing.T) *FavoritesService {
	t.Helper()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "favorites.json")
	return NewFavoritesService(configPath)
}

func TestFavorites_AddAndLoad(t *testing.T) {
	svc := createFavoritesTestService(t)

	err := svc.Add("C:\\projects\\myapp", "", "默认")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	favs, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("Expected 1 favorite, got %d", len(favs))
	}
	if favs[0].Path != "C:\\projects\\myapp" {
		t.Errorf("Path mismatch: %s", favs[0].Path)
	}
}

func TestFavorites_AddDuplicate(t *testing.T) {
	svc := createFavoritesTestService(t)

	svc.Add("C:\\projects\\myapp", "", "默认")
	err := svc.Add("C:\\projects\\myapp", "", "默认")
	if err == nil {
		t.Fatal("Expected error for duplicate path")
	}
}

func TestFavorites_Remove(t *testing.T) {
	svc := createFavoritesTestService(t)

	svc.Add("C:\\projects\\a", "", "默认")
	svc.Add("C:\\projects\\b", "", "默认")

	err := svc.Remove("C:\\projects\\a")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	favs, _ := svc.Load()
	if len(favs) != 1 {
		t.Fatalf("Expected 1 after remove, got %d", len(favs))
	}
}

func TestFavorites_UpdateAlias(t *testing.T) {
	svc := createFavoritesTestService(t)

	svc.Add("C:\\projects\\myapp", "", "默认")
	err := svc.UpdateAlias("C:\\projects\\myapp", "My App")
	if err != nil {
		t.Fatalf("UpdateAlias: %v", err)
	}

	favs, _ := svc.Load()
	if favs[0].Alias != "My App" {
		t.Errorf("Alias not updated: %s", favs[0].Alias)
	}
}

func TestFavorites_MaxLimit(t *testing.T) {
	svc := createFavoritesTestService(t)

	for i := 0; i < 100; i++ {
		svc.Add(filepath.Join("C:\\projects", fmt.Sprintf("proj%d", i)), "", "默认")
	}

	err := svc.Add("C:\\projects\\overflow", "", "默认")
	if err == nil {
		t.Fatal("Expected error when exceeding 100 limit")
	}
}

func TestFavorites_UpdateGroup(t *testing.T) {
	svc := createFavoritesTestService(t)

	svc.Add("C:\\projects\\myapp", "", "默认")
	err := svc.UpdateGroup("C:\\projects\\myapp", "工作")
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}

	favs, _ := svc.Load()
	if favs[0].Group != "工作" {
		t.Errorf("Group not updated: %s", favs[0].Group)
	}
}

func TestFavorites_UpdateGroup_NotExists(t *testing.T) {
	svc := createFavoritesTestService(t)

	err := svc.UpdateGroup("C:\\nonexistent", "工作")
	if err == nil {
		t.Fatal("Expected error for nonexistent path")
	}
}

func TestFavorites_LoadEmpty(t *testing.T) {
	svc := createFavoritesTestService(t)

	favs, err := svc.Load()
	if err != nil {
		t.Fatalf("Load empty: %v", err)
	}
	if len(favs) != 0 {
		t.Errorf("Expected 0 favorites from empty file, got %d", len(favs))
	}
}

func TestFavorites_Remove_NotExists(t *testing.T) {
	svc := createFavoritesTestService(t)

	svc.Add("C:\\projects\\a", "", "默认")
	err := svc.Remove("C:\\nonexistent")
	// Should not error, just save without the nonexistent path
	if err != nil {
		t.Fatalf("Remove nonexistent should not error: %v", err)
	}
}

// 路径归一测试统一用 t.TempDir() 构造绝对路径：断言依赖平台分隔符，
// 硬编码 Windows 路径在 Linux CI 下 filepath.Clean 不会生效。

// TestFavorites_NormalizeTrailingSeparator 守护收藏路径归一：
// 尾分隔符差异（D:\repo\ 与 D:\repo）视为同一路径，判重命中且落盘为清洗后的写法。
func TestFavorites_NormalizeTrailingSeparator(t *testing.T) {
	svc := createFavoritesTestService(t)
	repo := filepath.Join(t.TempDir(), "repo")
	withTrailing := repo + string(os.PathSeparator)

	if err := svc.Add(withTrailing, "", "默认"); err != nil {
		t.Fatalf("Add with trailing separator: %v", err)
	}

	favs, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("Expected 1 favorite, got %d", len(favs))
	}
	if favs[0].Path != repo {
		t.Errorf("stored path should be cleaned to %q, got %q", repo, favs[0].Path)
	}

	// 不带尾分隔符的同一路径应判重（否则 D:\repo\ 与 D:\repo 重复收藏、聚合根重复）
	if err := svc.Add(repo, "重复", "默认"); err == nil {
		t.Fatal("Expected duplicate error for same path without trailing separator")
	}
}

// TestFavorites_Remove_NormalizesPath 守护删除侧归一：带尾分隔符也能删掉已收藏路径。
func TestFavorites_Remove_NormalizesPath(t *testing.T) {
	svc := createFavoritesTestService(t)
	repo := filepath.Join(t.TempDir(), "repo")

	if err := svc.Add(repo, "", "默认"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.Remove(repo + string(os.PathSeparator)); err != nil {
		t.Fatalf("Remove with trailing separator: %v", err)
	}

	favs, _ := svc.Load()
	if len(favs) != 0 {
		t.Fatalf("Expected 0 favorites after remove, got %d", len(favs))
	}
}

// TestFavorites_Update_NormalizesPath 守护更新侧归一：别名与分组更新同样按清洗路径命中。
func TestFavorites_Update_NormalizesPath(t *testing.T) {
	svc := createFavoritesTestService(t)
	repo := filepath.Join(t.TempDir(), "repo")
	withTrailing := repo + string(os.PathSeparator)

	if err := svc.Add(repo, "", "默认"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := svc.UpdateAlias(withTrailing, "别名"); err != nil {
		t.Fatalf("UpdateAlias with trailing separator: %v", err)
	}
	if err := svc.UpdateGroup(withTrailing, "工作"); err != nil {
		t.Fatalf("UpdateGroup with trailing separator: %v", err)
	}

	favs, _ := svc.Load()
	if len(favs) != 1 {
		t.Fatalf("Expected 1 favorite, got %d", len(favs))
	}
	if favs[0].Alias != "别名" || favs[0].Group != "工作" {
		t.Errorf("alias/group not updated: alias=%q group=%q", favs[0].Alias, favs[0].Group)
	}
}

// TestFavorites_Load_NormalizesLegacyPaths 守护旧数据兼容：
// 早期落盘的路径可能带尾分隔符，Load 须在内存归一（不强制落盘迁移）。
func TestFavorites_Load_NormalizesLegacyPaths(t *testing.T) {
	svc := createFavoritesTestService(t)
	repo := filepath.Join(t.TempDir(), "repo")

	// 直接落盘旧数据形态（带尾分隔符）
	err := util.SaveJSON(svc.configPath, FavoritesConfig{
		Favorites: []*model.Favorite{{Path: repo + string(os.PathSeparator), Group: "默认"}},
	})
	if err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}

	favs, err := svc.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 1 {
		t.Fatalf("Expected 1 favorite, got %d", len(favs))
	}
	if favs[0].Path != repo {
		t.Errorf("legacy path should be normalized in memory to %q, got %q", repo, favs[0].Path)
	}
}
