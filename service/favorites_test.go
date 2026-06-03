package service

import (
	"fmt"
	"path/filepath"
	"testing"
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
