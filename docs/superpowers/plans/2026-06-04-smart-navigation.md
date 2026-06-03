# 智能导航中心 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 构建统一的 Command Palette 式导航中心，集成树状态记忆、收藏夹、快速搜索和最近访问功能，大幅提升文件导航效率。

**Architecture:** 前端新增 `CommandPalette.vue` 组件作为统一入口，新增 `composables/useTreeState.js`、`composables/useFavorites.js`、`composables/useRecentAccess.js` 三个组合式函数管理各自状态。后端新增 `SearchFiles` API 提供文件搜索能力，收藏夹配置复用现有 JSON 持久化模式存入 `data/favorites.json`。

**Tech Stack:** Vue 3 Composition API, Element Plus, Go, Wails binding, localStorage

---

## 文件结构

| 操作 | 路径 | 职责 |
|------|------|------|
| Create | `frontend/src/composables/useTreeState.js` | 树状态记忆：保存/恢复每个工作目录的展开状态 |
| Create | `frontend/src/composables/useFavorites.js` | 收藏夹管理：CRUD、分组、跳转逻辑 |
| Create | `frontend/src/composables/useRecentAccess.js` | 最近访问记录管理 |
| Create | `frontend/src/composables/useCommandPalette.js` | Command Palette 核心逻辑：搜索、模式切换、结果排序 |
| Create | `frontend/src/components/CommandPalette.vue` | Command Palette UI 组件 |
| Create | `service/search.go` | 文件搜索后端服务 |
| Create | `service/search_test.go` | 文件搜索服务测试 |
| Create | `service/favorites.go` | 收藏夹后端服务 |
| Create | `service/favorites_test.go` | 收藏夹服务测试 |
| Modify | `frontend/src/views/Home.vue` | 集成 Command Palette、树状态记忆 |
| Modify | `frontend/src/components/FileTreePanel.vue` | 集成树状态保存/恢复，右键菜单添加"添加到收藏"和"添加为工作目录" |
| Modify | `frontend/src/components/DirectoryTree.vue` | 路径省略显示、深层目录支持 |
| Modify | `app.go` | 注册新的后端 API |
| Modify | `model/models.go` | 新增 Favorite 和 SearchResult 模型 |
| Create | `frontend/src/components/__tests__/CommandPalette.spec.js` | 前端测试 |
| Create | `frontend/src/composables/__tests__/useTreeState.spec.js` | 树状态单测 |
| Create | `frontend/src/composables/__tests__/useFavorites.spec.js` | 收藏夹单测 |
| Create | `frontend/src/composables/__tests__/useRecentAccess.spec.js` | 最近访问单测 |

---

## Task 1: 后端数据模型

**Files:**
- Modify: `model/models.go`

- [ ] **Step 1: 添加 Favorite 和 SearchResult 模型**

在 `model/models.go` 末尾添加：

```go
// Favorite 收藏夹条目
type Favorite struct {
	Path      string `json:"path"`
	Alias     string `json:"alias,omitempty"`
	Group     string `json:"group"`
	CreatedAt int64  `json:"createdAt"`
}

// SearchResult 文件搜索结果
type SearchResult struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}
```

- [ ] **Step 2: 运行后端测试确认无破坏**

Run: `go test ./...`
Expected: All existing tests PASS

- [ ] **Step 3: 提交**

```bash
git add model/models.go
git commit -m "feat(model): add Favorite and SearchResult models"
```

---

## Task 2: 文件搜索后端服务

**Files:**
- Create: `service/search.go`
- Create: `service/search_test.go`

- [ ] **Step 1: 编写搜索服务测试**

创建 `service/search_test.go`：

```go
package service

import (
	"os"
	"path/filepath"
	"testing"
)

func createSearchTestDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	dirs := []string{"src", "src/components", "src/utils", "docs"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(root, d), 0755)
	}

	files := []string{
		"src/main.go",
		"src/components/Button.vue",
		"src/components/Modal.vue",
		"src/utils/helper.js",
		"docs/README.md",
	}
	for _, f := range files {
		os.WriteFile(filepath.Join(root, f), []byte("test"), 0644)
	}
	return root
}

func TestSearchFiles_ByName(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "Button", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected at least one result")
	}
	if results[0].Name != "Button.vue" {
		t.Errorf("Expected Button.vue, got %s", results[0].Name)
	}
}

func TestSearchFiles_Fuzzy(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "hlp", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Expected fuzzy match for helper.js")
	}
}

func TestSearchFiles_MaxResults(t *testing.T) {
	root := createSearchTestDir(t)
	svc := NewSearchService()

	results, err := svc.Search(root, "", 2)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Expected max 2 results, got %d", len(results))
	}
}

func TestSearchFiles_EmptyDir(t *testing.T) {
	root := t.TempDir()
	svc := NewSearchService()

	results, err := svc.Search(root, "anything", 20)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty dir, got %d", len(results))
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./service/ -run TestSearch -v`
Expected: FAIL (SearchService not defined)

- [ ] **Step 3: 实现搜索服务**

创建 `service/search.go`：

```go
package service

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"git-manager/model"
)

type SearchService struct{}

func NewSearchService() *SearchService {
	return &SearchService{}
}

func (s *SearchService) Search(rootDir, query string, maxResults int) ([]*model.SearchResult, error) {
	var results []*model.SearchResult
	queryLower := strings.ToLower(query)

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == ".git" && info.IsDir() {
			return filepath.SkipDir
		}
		if info.Name() == "node_modules" && info.IsDir() {
			return filepath.SkipDir
		}
		if path == rootDir {
			return nil
		}

		name := info.Name()
		if queryLower == "" || fuzzyMatch(strings.ToLower(name), queryLower) {
			fileType := "file"
			if info.IsDir() {
				fileType = "directory"
			}
			relPath, _ := filepath.Rel(rootDir, path)
			results = append(results, &model.SearchResult{
				Name: name,
				Path: relPath,
				Type: fileType,
			})
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		iScore := matchScore(strings.ToLower(results[i].Name), queryLower)
		jScore := matchScore(strings.ToLower(results[j].Name), queryLower)
		return iScore > jScore
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}
	return results, nil
}

func fuzzyMatch(text, pattern string) bool {
	if pattern == "" {
		return true
	}
	pi := 0
	for i := 0; i < len(text) && pi < len(pattern); i++ {
		if text[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func matchScore(text, pattern string) int {
	if pattern == "" {
		return 0
	}
	if text == pattern {
		return 100
	}
	if strings.HasPrefix(text, pattern) {
		return 80
	}
	if strings.Contains(text, pattern) {
		return 60
	}
	return 40
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./service/ -run TestSearch -v`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add service/search.go service/search_test.go
git commit -m "feat(service): add file search service with fuzzy matching"
```

---

## Task 3: 收藏夹后端服务

**Files:**
- Create: `service/favorites.go`
- Create: `service/favorites_test.go`

- [ ] **Step 1: 编写收藏夹服务测试**

创建 `service/favorites_test.go`：

```go
package service

import (
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
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./service/ -run TestFavorites -v`
Expected: FAIL

- [ ] **Step 3: 实现收藏夹服务**

创建 `service/favorites.go`：

```go
package service

import (
	"fmt"
	"time"

	"git-manager/model"
	"git-manager/util"
)

type FavoritesService struct {
	configPath string
}

type FavoritesConfig struct {
	Favorites []*model.Favorite `json:"favorites"`
}

func NewFavoritesService(configPath string) *FavoritesService {
	return &FavoritesService{configPath: configPath}
}

func (s *FavoritesService) Load() ([]*model.Favorite, error) {
	if !util.FileExists(s.configPath) {
		return []*model.Favorite{}, nil
	}
	var config FavoritesConfig
	err := util.LoadJSON(s.configPath, &config)
	if err != nil {
		return nil, err
	}
	return config.Favorites, nil
}

func (s *FavoritesService) save(favorites []*model.Favorite) error {
	config := FavoritesConfig{Favorites: favorites}
	return util.SaveJSON(s.configPath, config)
}

func (s *FavoritesService) Add(path, alias, group string) error {
	favorites, err := s.Load()
	if err != nil {
		return err
	}

	if len(favorites) >= 100 {
		return fmt.Errorf("收藏夹已满（最多 100 条）")
	}

	for _, f := range favorites {
		if f.Path == path {
			return fmt.Errorf("该路径已收藏")
		}
	}

	if group == "" {
		group = "默认"
	}

	favorites = append(favorites, &model.Favorite{
		Path:      path,
		Alias:     alias,
		Group:     group,
		CreatedAt: time.Now().UnixMilli(),
	})
	return s.save(favorites)
}

func (s *FavoritesService) Remove(path string) error {
	favorites, err := s.Load()
	if err != nil {
		return err
	}

	var filtered []*model.Favorite
	for _, f := range favorites {
		if f.Path != path {
			filtered = append(filtered, f)
		}
	}
	return s.save(filtered)
}

func (s *FavoritesService) UpdateAlias(path, alias string) error {
	favorites, err := s.Load()
	if err != nil {
		return err
	}

	for _, f := range favorites {
		if f.Path == path {
			f.Alias = alias
			return s.save(favorites)
		}
	}
	return fmt.Errorf("收藏不存在")
}

func (s *FavoritesService) UpdateGroup(path, group string) error {
	favorites, err := s.Load()
	if err != nil {
		return err
	}

	for _, f := range favorites {
		if f.Path == path {
			f.Group = group
			return s.save(favorites)
		}
	}
	return fmt.Errorf("收藏不存在")
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./service/ -run TestFavorites -v`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add service/favorites.go service/favorites_test.go
git commit -m "feat(service): add favorites service with CRUD and limit"
```

---

## Task 4: 注册后端 API

**Files:**
- Modify: `app.go`

- [ ] **Step 1: 在 App 结构体添加新服务**

在 `app.go` 的 `App` struct 中添加：

```go
searchSvc    *SearchService
favoritesSvc *FavoritesService
```

在 `startup` 方法中初始化：

```go
favoritesPath := filepath.Join(dataDir, "favorites.json")
a.searchSvc = service.NewSearchService()
a.favoritesSvc = service.NewFavoritesService(favoritesPath)
```

- [ ] **Step 2: 添加搜索 API 方法**

```go
// SearchFiles 搜索文件
func (a *App) SearchFiles(rootDir, query string, maxResults int) []*model.SearchResult {
	results, err := a.searchSvc.Search(rootDir, query, maxResults)
	if err != nil {
		println("SearchFiles error:", err.Error())
		return []*model.SearchResult{}
	}
	return results
}
```

- [ ] **Step 3: 添加收藏夹 API 方法**

```go
// GetFavorites 获取所有收藏
func (a *App) GetFavorites() []*model.Favorite {
	favorites, err := a.favoritesSvc.Load()
	if err != nil {
		println("GetFavorites error:", err.Error())
		return []*model.Favorite{}
	}
	return favorites
}

// AddFavorite 添加收藏
func (a *App) AddFavorite(path, alias, group string) string {
	err := a.favoritesSvc.Add(path, alias, group)
	if err != nil {
		return err.Error()
	}
	return ""
}

// RemoveFavorite 移除收藏
func (a *App) RemoveFavorite(path string) string {
	err := a.favoritesSvc.Remove(path)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteAlias 更新收藏别名
func (a *App) UpdateFavoriteAlias(path, alias string) string {
	err := a.favoritesSvc.UpdateAlias(path, alias)
	if err != nil {
		return err.Error()
	}
	return ""
}

// UpdateFavoriteGroup 更新收藏分组
func (a *App) UpdateFavoriteGroup(path, group string) string {
	err := a.favoritesSvc.UpdateGroup(path, group)
	if err != nil {
		return err.Error()
	}
	return ""
}
```

- [ ] **Step 4: 运行全量测试**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add app.go
git commit -m "feat(app): register search and favorites API bindings"
```

---

## Task 5: 树状态记忆 composable

**Files:**
- Create: `frontend/src/composables/useTreeState.js`
- Create: `frontend/src/composables/__tests__/useTreeState.spec.js`

- [ ] **Step 1: 编写测试**

创建 `frontend/src/composables/__tests__/useTreeState.spec.js`：

```javascript
import { describe, it, expect, beforeEach } from 'vitest'
import { useTreeState } from '../useTreeState'

describe('useTreeState', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('saves and restores expanded paths', () => {
    const { saveState, restoreState } = useTreeState()

    saveState('/work/projectA', {
      expandedPaths: ['/work/projectA/src', '/work/projectA/src/components'],
      scrollTop: 120,
      selectedPath: '/work/projectA/src/main.js'
    })

    const state = restoreState('/work/projectA')
    expect(state.expandedPaths).toEqual(['/work/projectA/src', '/work/projectA/src/components'])
    expect(state.scrollTop).toBe(120)
    expect(state.selectedPath).toBe('/work/projectA/src/main.js')
  })

  it('returns empty state for unknown directory', () => {
    const { restoreState } = useTreeState()

    const state = restoreState('/unknown/dir')
    expect(state.expandedPaths).toEqual([])
    expect(state.scrollTop).toBe(0)
    expect(state.selectedPath).toBeNull()
  })

  it('limits expanded paths to 200', () => {
    const { saveState, restoreState } = useTreeState()

    const paths = Array.from({ length: 250 }, (_, i) => `/dir/path${i}`)
    saveState('/work/big', { expandedPaths: paths, scrollTop: 0, selectedPath: null })

    const state = restoreState('/work/big')
    expect(state.expandedPaths.length).toBe(200)
  })

  it('clears state for a directory', () => {
    const { saveState, clearState, restoreState } = useTreeState()

    saveState('/work/projectA', { expandedPaths: ['/a'], scrollTop: 0, selectedPath: null })
    clearState('/work/projectA')

    const state = restoreState('/work/projectA')
    expect(state.expandedPaths).toEqual([])
  })
})
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd frontend && npx vitest run src/composables/__tests__/useTreeState.spec.js`
Expected: FAIL

- [ ] **Step 3: 实现 useTreeState**

创建 `frontend/src/composables/useTreeState.js`：

```javascript
const MAX_EXPANDED_PATHS = 200
const STORAGE_PREFIX = 'treeState:'

export function useTreeState() {
  function saveState(dirPath, state) {
    const key = STORAGE_PREFIX + dirPath
    const data = {
      expandedPaths: (state.expandedPaths || []).slice(-MAX_EXPANDED_PATHS),
      scrollTop: state.scrollTop || 0,
      selectedPath: state.selectedPath || null
    }
    try {
      localStorage.setItem(key, JSON.stringify(data))
    } catch (e) {
      // localStorage 满时静默失败
    }
  }

  function restoreState(dirPath) {
    const key = STORAGE_PREFIX + dirPath
    try {
      const raw = localStorage.getItem(key)
      if (!raw) return emptyState()
      return JSON.parse(raw)
    } catch {
      return emptyState()
    }
  }

  function clearState(dirPath) {
    const key = STORAGE_PREFIX + dirPath
    localStorage.removeItem(key)
  }

  function emptyState() {
    return { expandedPaths: [], scrollTop: 0, selectedPath: null }
  }

  return { saveState, restoreState, clearState }
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd frontend && npx vitest run src/composables/__tests__/useTreeState.spec.js`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add frontend/src/composables/useTreeState.js frontend/src/composables/__tests__/useTreeState.spec.js
git commit -m "feat(composable): add useTreeState for per-directory tree state persistence"
```

---

## Task 6: 最近访问 composable

**Files:**
- Create: `frontend/src/composables/useRecentAccess.js`
- Create: `frontend/src/composables/__tests__/useRecentAccess.spec.js`

- [ ] **Step 1: 编写测试**

创建 `frontend/src/composables/__tests__/useRecentAccess.spec.js`：

```javascript
import { describe, it, expect, beforeEach } from 'vitest'
import { useRecentAccess } from '../useRecentAccess'

describe('useRecentAccess', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('records access and retrieves recent items', () => {
    const { record, getRecent } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    record({ path: '/a/src', type: 'dir', workDir: '/a' })

    const items = getRecent(10)
    expect(items.length).toBe(2)
    expect(items[0].path).toBe('/a/src')
  })

  it('deduplicates by path and updates lastAccess', () => {
    const { record, getRecent } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    record({ path: '/a/other.js', type: 'file', workDir: '/a' })
    record({ path: '/a/file.js', type: 'file', workDir: '/a' })

    const items = getRecent(10)
    expect(items.length).toBe(2)
    expect(items[0].path).toBe('/a/file.js')
  })

  it('limits to 50 records', () => {
    const { record, getRecent } = useRecentAccess()

    for (let i = 0; i < 60; i++) {
      record({ path: `/dir/file${i}.js`, type: 'file', workDir: '/dir' })
    }

    const items = getRecent(100)
    expect(items.length).toBe(50)
  })

  it('clears all records', () => {
    const { record, getRecent, clear } = useRecentAccess()

    record({ path: '/a/file.js', type: 'file', workDir: '/a' })
    clear()

    const items = getRecent(10)
    expect(items.length).toBe(0)
  })
})
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd frontend && npx vitest run src/composables/__tests__/useRecentAccess.spec.js`
Expected: FAIL

- [ ] **Step 3: 实现 useRecentAccess**

创建 `frontend/src/composables/useRecentAccess.js`：

```javascript
const STORAGE_KEY = 'recentAccess'
const MAX_RECORDS = 50

export function useRecentAccess() {
  function loadRecords() {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return []
      return JSON.parse(raw)
    } catch {
      return []
    }
  }

  function saveRecords(records) {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(records))
    } catch {
      // 静默失败
    }
  }

  function record({ path, type, workDir }) {
    let records = loadRecords()

    records = records.filter(r => r.path !== path)

    records.unshift({
      path,
      type,
      workDir,
      lastAccess: Date.now()
    })

    if (records.length > MAX_RECORDS) {
      records = records.slice(0, MAX_RECORDS)
    }

    saveRecords(records)
  }

  function getRecent(limit = 10) {
    const records = loadRecords()
    return records.slice(0, limit)
  }

  function clear() {
    localStorage.removeItem(STORAGE_KEY)
  }

  return { record, getRecent, clear }
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd frontend && npx vitest run src/composables/__tests__/useRecentAccess.spec.js`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add frontend/src/composables/useRecentAccess.js frontend/src/composables/__tests__/useRecentAccess.spec.js
git commit -m "feat(composable): add useRecentAccess for navigation history"
```

---

## Task 7: 收藏夹 composable

**Files:**
- Create: `frontend/src/composables/useFavorites.js`
- Create: `frontend/src/composables/__tests__/useFavorites.spec.js`

- [ ] **Step 1: 编写测试**

创建 `frontend/src/composables/__tests__/useFavorites.spec.js`：

```javascript
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useFavorites } from '../useFavorites'

vi.mock('../../wailsjs/go/main/App', () => ({
  GetFavorites: vi.fn(() => Promise.resolve([
    { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 }
  ])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))

describe('useFavorites', () => {
  it('loads favorites', async () => {
    const { favorites, loadFavorites } = useFavorites()
    await loadFavorites()
    expect(favorites.value.length).toBe(1)
    expect(favorites.value[0].alias).toBe('My App')
  })

  it('adds a favorite', async () => {
    const { addFavorite } = useFavorites()
    const result = await addFavorite('C:\\new\\path', '', '默认')
    expect(result).toBe('')
  })

  it('searches favorites by alias and path', () => {
    const { favorites, searchFavorites } = useFavorites()
    favorites.value = [
      { path: 'C:\\projects\\app', alias: 'My App', group: '默认', createdAt: 1000 },
      { path: 'C:\\work\\server', alias: '', group: '工作', createdAt: 2000 }
    ]

    const results = searchFavorites('app')
    expect(results.length).toBe(1)
    expect(results[0].path).toBe('C:\\projects\\app')
  })
})
```

- [ ] **Step 2: 运行测试验证失败**

Run: `cd frontend && npx vitest run src/composables/__tests__/useFavorites.spec.js`
Expected: FAIL

- [ ] **Step 3: 实现 useFavorites**

创建 `frontend/src/composables/useFavorites.js`：

```javascript
import { ref } from 'vue'
import { GetFavorites, AddFavorite, RemoveFavorite, UpdateFavoriteAlias, UpdateFavoriteGroup } from '../../wailsjs/go/main/App'

export function useFavorites() {
  const favorites = ref([])

  async function loadFavorites() {
    favorites.value = await GetFavorites()
  }

  async function addFavorite(path, alias, group) {
    const err = await AddFavorite(path, alias, group || '默认')
    if (!err) {
      await loadFavorites()
    }
    return err
  }

  async function removeFavorite(path) {
    const err = await RemoveFavorite(path)
    if (!err) {
      await loadFavorites()
    }
    return err
  }

  async function updateAlias(path, alias) {
    return await UpdateFavoriteAlias(path, alias)
  }

  async function updateGroup(path, group) {
    return await UpdateFavoriteGroup(path, group)
  }

  function searchFavorites(query) {
    if (!query) return favorites.value
    const q = query.toLowerCase()
    return favorites.value.filter(f => {
      const name = (f.alias || f.path).toLowerCase()
      return name.includes(q) || f.path.toLowerCase().includes(q)
    })
  }

  return { favorites, loadFavorites, addFavorite, removeFavorite, updateAlias, updateGroup, searchFavorites }
}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `cd frontend && npx vitest run src/composables/__tests__/useFavorites.spec.js`
Expected: All PASS

- [ ] **Step 5: 提交**

```bash
git add frontend/src/composables/useFavorites.js frontend/src/composables/__tests__/useFavorites.spec.js
git commit -m "feat(composable): add useFavorites for bookmark management"
```

---

## Task 8: Command Palette composable

**Files:**
- Create: `frontend/src/composables/useCommandPalette.js`

- [ ] **Step 1: 实现 useCommandPalette**

创建 `frontend/src/composables/useCommandPalette.js`：

```javascript
import { ref, computed } from 'vue'
import { SearchFiles } from '../../wailsjs/go/main/App'

export function useCommandPalette() {
  const visible = ref(false)
  const input = ref('')
  const selectedIndex = ref(0)
  const fileResults = ref([])
  const searchLoading = ref(false)

  const mode = computed(() => {
    if (input.value.startsWith('#')) return 'workdir'
    if (input.value.startsWith('@')) return 'favorites'
    if (input.value.startsWith('>')) return 'command'
    return 'general'
  })

  const query = computed(() => {
    if (mode.value !== 'general') {
      return input.value.slice(1).trim()
    }
    return input.value.trim()
  })

  function open() {
    visible.value = true
    input.value = ''
    selectedIndex.value = 0
    fileResults.value = []
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
  }

  async function searchFiles(rootDir) {
    if (!query.value || mode.value !== 'general') {
      fileResults.value = []
      return
    }
    searchLoading.value = true
    try {
      fileResults.value = await SearchFiles(rootDir, query.value, 20)
    } catch (e) {
      fileResults.value = []
    } finally {
      searchLoading.value = false
    }
  }

  function moveSelection(delta) {
    const maxIndex = fileResults.value.length - 1
    selectedIndex.value = Math.max(0, Math.min(maxIndex, selectedIndex.value + delta))
  }

  function resetSelection() {
    selectedIndex.value = 0
  }

  return {
    visible,
    input,
    mode,
    query,
    selectedIndex,
    fileResults,
    searchLoading,
    open,
    close,
    searchFiles,
    moveSelection,
    resetSelection
  }
}
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/composables/useCommandPalette.js
git commit -m "feat(composable): add useCommandPalette for unified search UI"
```

---

## Task 9: Command Palette 组件

**Files:**
- Create: `frontend/src/components/CommandPalette.vue`

- [ ] **Step 1: 实现 CommandPalette UI**

创建 `frontend/src/components/CommandPalette.vue`：

```vue
<template>
  <el-dialog
    v-model="visible"
    :show-close="false"
    :close-on-click-modal="true"
    :close-on-press-escape="true"
    width="600px"
    top="15vh"
    class="command-palette-dialog"
    @close="onClose"
  >
    <template #header>
      <el-input
        v-model="input"
        placeholder="搜索文件、目录 (#切换工作目录 @收藏夹)"
        size="large"
        clearable
        autofocus
        @keydown.down.prevent="moveDown"
        @keydown.up.prevent="moveUp"
        @keydown.enter.prevent="selectCurrent"
        @input="onInput"
      >
        <template #prefix>
          <el-icon><Search /></el-icon>
        </template>
      </el-input>
    </template>

    <div class="palette-content">
      <!-- 最近访问 -->
      <div v-if="showRecent" class="result-section">
        <div class="section-title">最近访问</div>
        <div
          v-for="(item, index) in recentItems"
          :key="'recent-' + index"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectItem(item)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon">
            <component :is="item.type === 'file' ? Document : Folder" />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
          <div class="result-time">{{ formatTime(item.lastAccess) }}</div>
        </div>
      </div>

      <!-- 收藏夹结果 -->
      <div v-if="showFavorites" class="result-section">
        <div class="section-title">收藏夹</div>
        <div
          v-for="(item, index) in favoriteResults"
          :key="'fav-' + index"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectFavorite(item)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon" color="#f59e0b">
            <Star />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ item.alias || getFileName(item.path) }}</div>
            <div class="result-path">{{ item.path }}</div>
          </div>
        </div>
      </div>

      <!-- 工作目录切换 -->
      <div v-if="showWorkDirs" class="result-section">
        <div class="section-title">工作目录</div>
        <div
          v-for="(dir, index) in workDirs"
          :key="'dir-' + dir.id"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectWorkDir(dir)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon">
            <Folder />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ dir.name }}</div>
            <div class="result-path">{{ dir.path }}</div>
          </div>
        </div>
      </div>

      <!-- 文件搜索结果 -->
      <div v-if="showFileResults" class="result-section">
        <div class="section-title">搜索结果</div>
        <div v-if="searchLoading" class="result-loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          搜索中...
        </div>
        <div
          v-for="(file, index) in fileResults"
          :key="'file-' + index"
          class="result-item"
          :class="{ 'result-item--active': index === selectedIndex }"
          @click="selectFile(file)"
          @mouseenter="selectedIndex = index"
        >
          <el-icon class="result-icon">
            <component :is="file.type === 'file' ? Document : Folder" />
          </el-icon>
          <div class="result-info">
            <div class="result-name">{{ file.name }}</div>
            <div class="result-path">{{ file.path }}</div>
          </div>
        </div>
        <div v-if="!searchLoading && fileResults.length === 0 && query" class="result-empty">
          未找到匹配项
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import { Search, Document, Folder, Star, Loading } from '@element-plus/icons-vue'
import { useCommandPalette } from '../composables/useCommandPalette'
import { useRecentAccess } from '../composables/useRecentAccess'
import { useFavorites } from '../composables/useFavorites'

const props = defineProps({
  modelValue: Boolean,
  currentDir: String,
  workDirs: Array
})

const emit = defineEmits(['update:modelValue', 'select-file', 'select-favorite', 'select-workdir'])

const { input, mode, query, selectedIndex, fileResults, searchLoading, searchFiles, moveSelection, resetSelection } = useCommandPalette()
const { getRecent } = useRecentAccess()
const { favorites, loadFavorites, searchFavorites } = useFavorites()

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val)
})

const recentItems = ref([])
const favoriteResults = ref([])

const showRecent = computed(() => mode.value === 'general' && !query.value && recentItems.value.length > 0)
const showFavorites = computed(() => mode.value === 'favorites' || (mode.value === 'general' && query.value && favoriteResults.value.length > 0))
const showWorkDirs = computed(() => mode.value === 'workdir')
const showFileResults = computed(() => mode.value === 'general' && query.value)

let searchTimer = null

function onInput() {
  resetSelection()
  clearTimeout(searchTimer)
  
  if (mode.value === 'favorites') {
    favoriteResults.value = searchFavorites(query.value)
  } else if (mode.value === 'general' && query.value) {
    favoriteResults.value = searchFavorites(query.value).slice(0, 5)
    searchTimer = setTimeout(() => {
      searchFiles(props.currentDir)
    }, 300)
  }
}

function moveDown() {
  moveSelection(1)
}

function moveUp() {
  moveSelection(-1)
}

function selectCurrent() {
  if (showRecent.value && recentItems.value[selectedIndex.value]) {
    selectItem(recentItems.value[selectedIndex.value])
  } else if (showFavorites.value && favoriteResults.value[selectedIndex.value]) {
    selectFavorite(favoriteResults.value[selectedIndex.value])
  } else if (showWorkDirs.value && props.workDirs[selectedIndex.value]) {
    selectWorkDir(props.workDirs[selectedIndex.value])
  } else if (showFileResults.value && fileResults.value[selectedIndex.value]) {
    selectFile(fileResults.value[selectedIndex.value])
  }
}

function selectItem(item) {
  emit('select-file', item)
  onClose()
}

function selectFile(file) {
  emit('select-file', { path: file.path, type: file.type })
  onClose()
}

function selectFavorite(fav) {
  emit('select-favorite', fav)
  onClose()
}

function selectWorkDir(dir) {
  emit('select-workdir', dir)
  onClose()
}

function onClose() {
  visible.value = false
  input.value = ''
  fileResults.value = []
  favoriteResults.value = []
  resetSelection()
}

function getFileName(path) {
  const parts = path.replace(/\\/g, '/').split('/')
  return parts[parts.length - 1]
}

function formatTime(timestamp) {
  const now = Date.now()
  const diff = now - timestamp
  const minutes = Math.floor(diff / 60000)
  if (minutes < 1) return '刚刚'
  if (minutes < 60) return `${minutes}分钟前`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}小时前`
  return `${Math.floor(hours / 24)}天前`
}

watch(visible, (val) => {
  if (val) {
    recentItems.value = getRecent(10)
  }
})

onMounted(() => {
  loadFavorites()
})
</script>

<style scoped>
.command-palette-dialog :deep(.el-dialog__header) {
  padding: 15px 20px 10px;
}

.command-palette-dialog :deep(.el-dialog__body) {
  padding: 0 0 15px 0;
  max-height: 400px;
  overflow-y: auto;
}

.palette-content {
  min-height: 100px;
}

.result-section {
  margin-bottom: 10px;
}

.section-title {
  font-size: 12px;
  color: #909399;
  padding: 8px 20px 5px;
  font-weight: 500;
}

.result-item {
  display: flex;
  align-items: center;
  padding: 10px 20px;
  cursor: pointer;
  transition: background 0.2s;
}

.result-item:hover,
.result-item--active {
  background: #f5f7fa;
}

.result-icon {
  font-size: 18px;
  margin-right: 12px;
  flex-shrink: 0;
}

.result-info {
  flex: 1;
  min-width: 0;
}

.result-name {
  font-size: 14px;
  color: #303133;
  margin-bottom: 2px;
}

.result-path {
  font-size: 12px;
  color: #909399;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.result-time {
  font-size: 12px;
  color: #c0c4cc;
  margin-left: 10px;
  flex-shrink: 0;
}

.result-loading {
  padding: 20px;
  text-align: center;
  color: #909399;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.result-empty {
  padding: 20px;
  text-align: center;
  color: #909399;
}
</style>
```

- [ ] **Step 2: 提交**

```bash
git add frontend/src/components/CommandPalette.vue
git commit -m "feat(component): add CommandPalette with search, favorites, and workdir switching"
```

---

## Task 10: 集成树状态记忆到 FileTreePanel

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: FileTreePanel 添加状态保存/恢复支持**

在 `FileTreePanel.vue` 的 `<script setup>` 中引入 useTreeState 并暴露方法：

```javascript
import { useTreeState } from '../composables/useTreeState'

const { saveState, restoreState } = useTreeState()

// 收集当前展开节点的路径
function getExpandedPaths() {
  const tree = fileTreeRef.value
  if (!tree) return []
  const store = tree.store
  const paths = []
  function walk(node) {
    if (node.expanded && node.data && node.data.path) {
      paths.push(node.data.path)
    }
    if (node.childNodes) {
      node.childNodes.forEach(walk)
    }
  }
  walk(store.root)
  return paths
}

// 保存当前树状态
function saveCurrentState(dirPath) {
  const treeEl = document.querySelector('.tree-content')
  saveState(dirPath, {
    expandedPaths: getExpandedPaths(),
    scrollTop: treeEl ? treeEl.scrollTop : 0,
    selectedPath: null
  })
}

// 恢复树状态（在树刷新后调用）
async function restoreTreeState(dirPath) {
  const state = restoreState(dirPath)
  if (state.expandedPaths.length === 0) return

  const tree = fileTreeRef.value
  if (!tree) return

  for (const path of state.expandedPaths) {
    const node = tree.getNode(path)
    if (node && !node.expanded) {
      node.expand()
      await new Promise(r => setTimeout(r, 50))
    }
  }

  if (state.scrollTop > 0) {
    const treeEl = document.querySelector('.tree-content')
    if (treeEl) treeEl.scrollTop = state.scrollTop
  }
}

defineExpose({
  refreshNode,
  showRenameAt,
  closeMenu,
  saveCurrentState,
  restoreTreeState
})
```

- [ ] **Step 2: Home.vue 在工作目录切换时保存/恢复状态**

在 `Home.vue` 的 `onDirectorySelect` 方法中添加状态管理：

```javascript
const onDirectorySelect = async (dirId) => {
  // 保存当前工作目录的树状态
  if (selectedDirectoryId.value) {
    const currentDir = directories.value.find(d => d.id === selectedDirectoryId.value)
    if (currentDir) {
      fileTreePanelRef.value?.saveCurrentState(currentDir.path)
    }
  }

  selectedDirectoryId.value = dirId
  selectedNode.value = null
  latestCommit.value = null
  contentPanelRef.value?.clearPreview()

  // 等待树重新渲染后恢复状态
  await nextTick()
  const newDir = directories.value.find(d => d.id === dirId)
  if (newDir) {
    setTimeout(() => {
      fileTreePanelRef.value?.restoreTreeState(newDir.path)
    }, 300)
  }
}
```

在 Home.vue 顶部引入 `nextTick`：

```javascript
import { ref, reactive, onMounted, watch, nextTick } from 'vue'
```

- [ ] **Step 3: 运行前端测试**

Run: `cd frontend && npm test`
Expected: All PASS

- [ ] **Step 4: 手动验证**

Run: `wails dev`
操作：切换工作目录 → 展开几个节点 → 切到另一个目录 → 切回来验证状态恢复

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue frontend/src/views/Home.vue
git commit -m "feat(tree): integrate tree state memory on work directory switch"
```

---

## Task 11: 集成 Command Palette 到 Home.vue

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 添加 CommandPalette 到 Home.vue**

在 Home.vue 的 template 中添加 CommandPalette 组件和快捷键监听：

```vue
<CommandPalette
  v-model="commandPaletteVisible"
  :current-dir="currentDirPath"
  :work-dirs="directories"
  @select-file="onPaletteSelectFile"
  @select-favorite="onPaletteSelectFavorite"
  @select-workdir="onPaletteSelectWorkDir"
/>
```

在 script 中添加：

```javascript
import CommandPalette from '../components/CommandPalette.vue'
import { useRecentAccess } from '../composables/useRecentAccess'

const commandPaletteVisible = ref(false)
const { record: recordAccess } = useRecentAccess()

const currentDirPath = computed(() => {
  const dir = directories.value.find(d => d.id === selectedDirectoryId.value)
  return dir ? dir.path : ''
})

// Command Palette 事件处理
function onPaletteSelectFile(item) {
  // 在文件树中定位
  recordAccess({ path: item.path, type: item.type, workDir: currentDirPath.value })
  fileTreePanelRef.value?.locateNode(item.path)
}

function onPaletteSelectFavorite(fav) {
  recordAccess({ path: fav.path, type: 'dir', workDir: currentDirPath.value })
  // 如果在当前工作目录下，直接定位
  if (fav.path.startsWith(currentDirPath.value)) {
    fileTreePanelRef.value?.locateNode(fav.path)
  } else {
    // 查找属于哪个工作目录
    const targetDir = directories.value.find(d => fav.path.startsWith(d.path))
    if (targetDir) {
      onDirectorySelect(targetDir.id)
      setTimeout(() => fileTreePanelRef.value?.locateNode(fav.path), 500)
    }
  }
}

function onPaletteSelectWorkDir(dir) {
  onDirectorySelect(dir.id)
}

// Ctrl+P 快捷键
function handleKeydown(e) {
  if (e.ctrlKey && e.key === 'p') {
    e.preventDefault()
    commandPaletteVisible.value = true
  }
}

onMounted(async () => {
  document.addEventListener('keydown', handleKeydown)
  await loadDirectories()
})
```

在 onNodeSelect 中记录最近访问：

```javascript
const onNodeSelect = (data) => {
  selectedNode.value = data
  contentPanelRef.value?.clearPreview()
  recordAccess({ path: data.path, type: data.type, workDir: currentDirPath.value })
}
```

- [ ] **Step 2: 运行前端测试**

Run: `cd frontend && npm test`
Expected: All PASS

- [ ] **Step 3: 手动验证**

Run: `wails dev`
操作：按 Ctrl+P → 输入文件名 → 验证搜索结果 → 选中跳转

- [ ] **Step 4: 提交**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat(home): integrate Command Palette with Ctrl+P shortcut"
```

---

## Task 12: 右键菜单增加"添加到收藏"和"添加为工作目录"

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

- [ ] **Step 1: 在右键菜单中添加新选项**

在 FileTreePanel.vue 的右键菜单数组中添加：

目录节点的右键菜单增加：
```javascript
{ label: '添加到收藏', icon: Star, action: 'addFavorite' },
{ label: '添加为工作目录', icon: FolderAdd, action: 'addAsWorkDir' },
```

文件节点的右键菜单增加：
```javascript
{ label: '添加到收藏', icon: Star, action: 'addFavorite' },
```

- [ ] **Step 2: 添加菜单处理逻辑**

```javascript
import { useFavorites } from '../composables/useFavorites'
const { addFavorite } = useFavorites()

const emit = defineEmits([...existingEmits, 'add-work-dir'])

async function handleAddFavorite(node) {
  const err = await addFavorite(node.path, '', '默认')
  if (err) {
    ElMessage.warning(err)
  } else {
    ElMessage.success('已添加到收藏')
  }
}

function handleAddAsWorkDir(node) {
  emit('add-work-dir', { path: node.path, name: node.name })
}
```

- [ ] **Step 3: Home.vue 处理"添加为工作目录"事件**

```javascript
const onAddWorkDir = async (data) => {
  const dir = await AddDirectory(data.name, data.path, false)
  if (dir) {
    await loadDirectories()
    ElMessage.success('已添加为工作目录')
  }
}
```

在 FileTreePanel 组件上绑定：`@add-work-dir="onAddWorkDir"`

- [ ] **Step 4: 手动验证**

Run: `wails dev`
操作：右键目录 → 验证新菜单项 → 添加收藏 → 添加为工作目录

- [ ] **Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue frontend/src/views/Home.vue
git commit -m "feat(menu): add 'Add to Favorites' and 'Add as Work Directory' context menu items"
```

---

## Task 13: DirectoryTree 深层目录路径省略显示

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue`

- [ ] **Step 1: 修改工作目录显示逻辑**

在 DirectoryTree.vue 中添加路径省略函数并应用到模板：

```javascript
function shortenPath(path) {
  if (!path || path.length <= 40) return path
  const parts = path.replace(/\\/g, '/').split('/')
  if (parts.length <= 3) return path
  return `.../${parts[parts.length - 2]}/${parts[parts.length - 1]}`
}
```

修改模板中 `dir-path` 的显示：

```vue
<div class="dir-path" :title="dir.path">{{ shortenPath(dir.path) }}</div>
```

- [ ] **Step 2: 手动验证**

Run: `wails dev`
操作：添加一个深层目录作为工作目录 → 验证路径省略显示和 tooltip

- [ ] **Step 3: 提交**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat(directory-tree): shorten long paths with ellipsis display"
```

---

## Task 14: 端到端集成测试

**Files:**
- Create: `frontend/src/components/__tests__/CommandPalette.spec.js`

- [ ] **Step 1: 编写 CommandPalette 组件测试**

创建 `frontend/src/components/__tests__/CommandPalette.spec.js`：

```javascript
import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import CommandPalette from '../CommandPalette.vue'

vi.mock('../../wailsjs/go/main/App', () => ({
  SearchFiles: vi.fn(() => Promise.resolve([
    { name: 'main.go', path: 'src/main.go', type: 'file' }
  ])),
  GetFavorites: vi.fn(() => Promise.resolve([])),
  AddFavorite: vi.fn(() => Promise.resolve('')),
  RemoveFavorite: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteAlias: vi.fn(() => Promise.resolve('')),
  UpdateFavoriteGroup: vi.fn(() => Promise.resolve(''))
}))

describe('CommandPalette', () => {
  const defaultProps = {
    modelValue: true,
    currentDir: 'C:\\projects\\test',
    workDirs: [
      { id: '1', name: 'Project A', path: 'C:\\projects\\a' },
      { id: '2', name: 'Project B', path: 'C:\\projects\\b' }
    ]
  }

  it('renders when visible', () => {
    const wrapper = mount(CommandPalette, { props: defaultProps })
    expect(wrapper.find('.command-palette-dialog').exists()).toBe(true)
  })

  it('switches to workdir mode with # prefix', async () => {
    const wrapper = mount(CommandPalette, { props: defaultProps })
    const input = wrapper.find('input')
    await input.setValue('#')
    await nextTick()
    expect(wrapper.find('.section-title').text()).toContain('工作目录')
  })

  it('emits select-workdir on workdir click', async () => {
    const wrapper = mount(CommandPalette, { props: defaultProps })
    const input = wrapper.find('input')
    await input.setValue('#')
    await nextTick()
    const items = wrapper.findAll('.result-item')
    if (items.length > 0) {
      await items[0].trigger('click')
      expect(wrapper.emitted('select-workdir')).toBeTruthy()
    }
  })
})
```

- [ ] **Step 2: 运行所有前端测试**

Run: `cd frontend && npm test`
Expected: All PASS

- [ ] **Step 3: 运行所有后端测试**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 4: 提交**

```bash
git add frontend/src/components/__tests__/CommandPalette.spec.js
git commit -m "test(palette): add CommandPalette component tests"
```

---

## Task 15: 最终验证与文档更新

**Files:**
- Modify: `docs/功能说明.md`
- Modify: `README.md`（如需要）

- [ ] **Step 1: 完整功能验证**

Run: `wails dev`

验证清单：
1. Ctrl+P 打开 Command Palette
2. 无输入时显示最近访问
3. 输入文件名搜索并跳转
4. `#` 前缀切换工作目录
5. `@` 前缀搜索收藏夹
6. 右键 → 添加到收藏
7. 右键 → 添加为工作目录
8. 切换工作目录后展开状态恢复
9. 深层目录路径省略显示

- [ ] **Step 2: 更新功能说明文档**

在 `docs/功能说明.md` 中新增：

```markdown
## 5. 智能导航中心

- Command Palette 快速导航（Ctrl+P）
- 文件/目录模糊搜索
- 收藏夹系统（添加、分组、跳转）
- 工作目录快速切换（#前缀）
- 每个工作目录独立记忆展开状态
- 最近访问历史
- 深层子目录可作为工作目录
```

- [ ] **Step 3: 提交**

```bash
git add docs/功能说明.md
git commit -m "docs: update feature list with Smart Navigation Center"
```
