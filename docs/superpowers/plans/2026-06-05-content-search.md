# 跨仓库内容搜索实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Command Palette 中新增 `:` 前缀内容搜索模式，支持跨工作目录搜索文件内容，ripgrep 自动加速 + Go 原生降级。

**Architecture:** 后端新增 `ContentSearchService`，检测系统 ripgrep 可用时调用 `rg` 命令，否则降级为 Go 原生遍历搜索。前端在 `useCommandPalette` composable 中新增 `content` / `content-global` 模式，Command Palette 统一回车触发搜索。搜索结果按仓库分组展示，点击用 VSCode 打开并跳转到行号。

**Tech Stack:** Go（后端搜索服务）、Vue 3 Composition API（前端交互）、Wails 绑定（前后端桥接）、ripgrep（可选加速）

---

## 文件结构

| 操作 | 文件 | 职责 |
|---|---|---|
| **新建** | `model/content_search.go` | 内容搜索数据模型（`ContentSearchResult`、`ContentSearchGroup`） |
| **新建** | `service/content_search.go` | 内容搜索服务（ripgrep 检测 + Go 原生降级 + 查询解析） |
| **新建** | `service/content_search_test.go` | 内容搜索单元测试 |
| **修改** | `model/settings.go` | 新增 `SearchExcludeDirs`、`SearchExcludeFiles` 字段 |
| **修改** | `app.go` | 新增 `ContentSearch` Wails 绑定方法、初始化 `contentSearchSvc` |
| **修改** | `frontend/src/composables/useCommandPalette.js` | 新增 `content`/`content-global` 模式、内容搜索逻辑 |
| **修改** | `frontend/src/components/CommandPalette.vue` | 新增内容搜索结果区域、全局搜索提示、搜索进度 |
| **修改** | `frontend/src/components/FileTreePanel.vue` | 右键菜单新增「在此目录中搜索」 |
| **修改** | `frontend/src/views/Home.vue` | 监听 `open-content-search` 事件、传递 `directories` 到 CommandPalette |
| **修改** | `frontend/src/components/SettingsPanel.vue` | 新增「搜索」设置页 |

---

### Task 1: 数据模型 — ContentSearchResult 和 ContentSearchGroup

**Files:**
- Create: `model/content_search.go`
- Test: `model/models_test.go`

- [ ] **Step 1: 创建数据模型文件**

```go
// model/content_search.go
package model

// ContentSearchResult 内容搜索单条结果
type ContentSearchResult struct {
	RepoName string `json:"repoName"` // 仓库名
	RepoPath string `json:"repoPath"` // 仓库绝对路径
	FilePath string `json:"filePath"` // 相对路径
	LineNum  int    `json:"lineNum"`  // 行号
	LineText string `json:"lineText"` // 匹配行内容
}

// ContentSearchGroup 按仓库分组的搜索结果
type ContentSearchGroup struct {
	RepoName string                 `json:"repoName"`
	RepoPath string                 `json:"repoPath"`
	Items    []*ContentSearchResult `json:"items"`
}

// ContentSearchQuery 解析后的搜索查询参数
type ContentSearchQuery struct {
	Keyword  string // 搜索关键词
	FileExt  string // 文件类型过滤（如 ".java"），为空则不过滤
	SubDir   string // 子目录路径（相对于工作目录），为空则搜索整个目录
	SearchAll bool  // 是否搜索所有工作目录
}
```

- [ ] **Step 2: 运行现有测试确认无影响**

Run: `go test ./model/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add model/content_search.go
git commit -m "feat(search): add content search data models"
```

---

### Task 2: 扩展 AppSettings — 搜索排除配置

**Files:**
- Modify: `model/settings.go`
- Modify: `model/settings_test.go`

- [ ] **Step 1: 在 AppSettings 新增搜索排除字段**

在 `model/settings.go` 的 `AppSettings` 结构体中新增：

```go
// AppSettings 应用设置
type AppSettings struct {
	GpuDisabled        bool     `json:"gpuDisabled"`
	DefaultShell       string   `json:"defaultShell"`
	GitBashPath        string   `json:"gitBashPath"`
	WslDistro          string   `json:"wslDistro"`
	SearchExcludeDirs  []string `json:"searchExcludeDirs"`  // 搜索排除目录
	SearchExcludeFiles []string `json:"searchExcludeFiles"` // 搜索排除文件模式
}
```

- [ ] **Step 2: 运行测试确认无破坏**

Run: `go test ./model/... ./service/...`
Expected: PASS（旧 JSON 无新字段时 Go 反序列化自动使用零值）

- [ ] **Step 3: Commit**

```bash
git add model/settings.go
git commit -m "feat(settings): add search exclude config to AppSettings"
```

---

### Task 3: 后端搜索服务 — ContentSearchService

**Files:**
- Create: `service/content_search.go`

- [ ] **Step 1: 实现 ContentSearchService 完整代码**

```go
// service/content_search.go
package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"git-manager/model"
)

// 默认排除配置
var defaultExcludeDirs = []string{
	".git", "node_modules", "dist", "build", "target",
	".idea", "__pycache__", ".gradle", "bin", ".settings",
	".vscode", ".cache", "vendor",
}

var defaultExcludeFiles = []string{
	".log", ".tmp", ".class", ".jar", ".war", ".zip", ".tar",
	".gz", ".exe", ".dll", ".so", ".dylib", ".png", ".jpg",
	".jpeg", ".gif", ".ico", ".pdf", ".woff", ".woff2", ".ttf",
	".eot", ".mp3", ".mp4", ".avi", ".mov",
}

// ContentSearchService 内容搜索服务
type ContentSearchService struct {
	rgAvailable bool   // 是否检测到 ripgrep
	rgPath      string // ripgrep 可执行文件路径
}

// NewContentSearchService 创建内容搜索服务
func NewContentSearchService() *ContentSearchService {
	svc := &ContentSearchService{}
	svc.detectRipgrep()
	return svc
}

// detectRipgrep 检测系统是否安装 ripgrep
func (s *ContentSearchService) detectRipgrep() {
	path, err := exec.LookPath("rg")
	if err == nil {
		s.rgAvailable = true
		s.rgPath = path
	}
}

// ContentSearch 执行内容搜索
// dirs: 要搜索的工作目录列表（绝对路径）
// repoNames: 与 dirs 一一对应的仓库名
// keyword: 搜索关键词
// fileExt: 文件类型过滤（如 ".java"），为空不过滤
// subDir: 子目录路径，为空搜索整个目录
// excludeDirs: 用户配置的排除目录
// excludeFiles: 用户配置的排除文件模式
// maxPerRepo: 每仓库最大结果数
func (s *ContentSearchService) ContentSearch(
	ctx context.Context,
	dirs []string,
	repoNames []string,
	keyword, fileExt, subDir string,
	excludeDirs, excludeFiles []string,
	maxPerRepo int,
) ([]*model.ContentSearchGroup, error) {
	if keyword == "" {
		return nil, nil
	}
	if len(excludeDirs) == 0 {
		excludeDirs = defaultExcludeDirs
	}
	if len(excludeFiles) == 0 {
		excludeFiles = defaultExcludeFiles
	}

	var (
		mu     sync.Mutex
		groups []*model.ContentSearchGroup
		wg     sync.WaitGroup
	)

	for i, dir := range dirs {
		wg.Add(1)
		go func(idx int, workDir string) {
			defer wg.Done()
			searchDir := workDir
			if subDir != "" {
				searchDir = filepath.Join(workDir, subDir)
			}

			var items []*model.ContentSearchResult
			var err error

			if s.rgAvailable {
				items, err = s.searchWithRipgrep(ctx, searchDir, keyword, fileExt, excludeDirs, maxPerRepo)
			}
			if !s.rgAvailable || err != nil {
				items = s.searchWithGo(ctx, searchDir, keyword, fileExt, excludeDirs, excludeFiles, maxPerRepo)
			}

			// 补全结果中的仓库信息
			for _, item := range items {
				item.RepoName = repoNames[idx]
				item.RepoPath = workDir
				rel, _ := filepath.Rel(workDir, filepath.Join(searchDir, item.FilePath))
				if rel != "" {
					item.FilePath = rel
				}
			}

			if len(items) > 0 {
				mu.Lock()
				groups = append(groups, &model.ContentSearchGroup{
					RepoName: repoNames[idx],
					RepoPath: workDir,
					Items:    items,
				})
				mu.Unlock()
			}
		}(i, dir)
	}

	wg.Wait()
	return groups, nil
}

// searchWithRipgrep 使用 ripgrep 搜索
func (s *ContentSearchService) searchWithRipgrep(
	ctx context.Context,
	dir, keyword, fileExt string,
	excludeDirs []string,
	maxResults int,
) ([]*model.ContentSearchResult, error) {
	args := []string{
		"--no-heading",
		"--line-number",
		"--color", "never",
		"--max-count", fmt.Sprintf("%d", maxResults),
		"-F", // 固定字符串，不做正则
		"-e", keyword,
	}

	// 文件类型过滤
	if fileExt != "" {
		ext := strings.TrimPrefix(fileExt, ".")
		args = append(args, "--type-add", fmt.Sprintf("custom:*.%s", ext))
		args = append(args, "-t", "custom")
	}

	// 排除目录
	for _, d := range excludeDirs {
		args = append(args, "--glob", "!"+d)
	}

	args = append(args, dir)

	cmd := exec.CommandContext(ctx, s.rgPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	HideCommandWindow(cmd)

	err := cmd.Run()
	if err != nil {
		// rg 退出码 1 表示无匹配，不是错误
		if cmd.ProcessState.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("rg failed: %s", stderr.String())
	}

	var results []*model.ContentSearchResult
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		// rg 输出格式: path:lineNum:content
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		filePath := parts[0]
		lineNum := 0
		fmt.Sscanf(parts[1], "%d", &lineNum)
		lineText := parts[2]

		results = append(results, &model.ContentSearchResult{
			FilePath: filePath,
			LineNum:  lineNum,
			LineText: lineText,
		})
		if len(results) >= maxResults {
			break
		}
	}

	return results, nil
}

// searchWithGo Go 原生搜索（ripgrep 降级方案）
func (s *ContentSearchService) searchWithGo(
	ctx context.Context,
	dir, keyword, fileExt string,
	excludeDirs, excludeFiles []string,
	maxResults int,
) []*model.ContentSearchResult {
	keywordLower := strings.ToLower(keyword)
	var results []*model.ContentSearchResult

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// 检查上下文取消
		select {
		case <-ctx.Done():
			return filepath.SkipAll
		default:
		}

		if info.IsDir() {
			// 排除目录
			if isExcludedDir(info.Name(), excludeDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		// 文件类型过滤
		if fileExt != "" {
			if !strings.HasSuffix(strings.ToLower(info.Name()), strings.ToLower(fileExt)) {
				return nil
			}
		}

		// 排除文件
		if isExcludedFile(info.Name(), excludeFiles) {
			return nil
		}

		// 搜索文件内容
		fileResults := searchFileContent(path, keywordLower, info.Size())
		results = append(results, fileResults...)

		if len(results) >= maxResults {
			return filepath.SkipAll
		}

		return nil
	})

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return results
}

// searchFileContent 搜索单个文件内容
func searchFileContent(path, keywordLower string, fileSize int64) []*model.ContentSearchResult {
	// 跳过大于 10MB 的文件
	if fileSize > 10*1024*1024 {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	// 跳过二进制文件（检测前 8KB 中是否含 NUL 字节）
	checkSize := len(data)
	if checkSize > 8192 {
		checkSize = 8192
	}
	if bytes.Contains(data[:checkSize], []byte{0}) {
		return nil
	}

	var results []*model.ContentSearchResult
	lines := bytes.Split(data, []byte("\n"))
	for i, line := range lines {
		if bytes.Contains(bytes.ToLower(line), []byte(keywordLower)) {
			// 截断过长的行
			lineStr := string(line)
			if len(lineStr) > 300 {
				lineStr = lineStr[:300] + "..."
			}
			results = append(results, &model.ContentSearchResult{
				FilePath: path,
				LineNum:  i + 1,
				LineText: lineStr,
			})
		}
	}

	return results
}

// isExcludedDir 检查目录是否在排除列表中
func isExcludedDir(name string, excludeDirs []string) bool {
	for _, d := range excludeDirs {
		if strings.EqualFold(name, d) {
			return true
		}
	}
	return false
}

// isExcludedFile 检查文件是否在排除列表中（按扩展名匹配）
func isExcludedFile(name string, excludeFiles []string) bool {
	nameLower := strings.ToLower(name)
	for _, pattern := range excludeFiles {
		if strings.HasSuffix(nameLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: 编译确认**

Run: `go build ./...`
Expected: 编译通过，无错误

- [ ] **Step 3: Commit**

```bash
git add service/content_search.go
git commit -m "feat(search): implement ContentSearchService with ripgrep + Go fallback"
```

---

### Task 4: 后端搜索服务 — 单元测试

**Files:**
- Create: `service/content_search_test.go`

- [ ] **Step 1: 编写内容搜索测试**

```go
// service/content_search_test.go
package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSearchWithGo_BasicMatch(t *testing.T) {
	// 创建临时测试目录
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "hello world\nfoo bar\nhello golang\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"", // 无文件类型过滤
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].LineNum != 1 {
		t.Errorf("expected line 1, got %d", results[0].LineNum)
	}
	if results[1].LineNum != 3 {
		t.Errorf("expected line 3, got %d", results[1].LineNum)
	}
}

func TestSearchWithGo_FileExtFilter(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建 .java 和 .xml 文件
	javaFile := filepath.Join(tmpDir, "App.java")
	xmlFile := filepath.Join(tmpDir, "config.xml")
	os.WriteFile(javaFile, []byte("public class App {}\n"), 0644)
	os.WriteFile(xmlFile, []byte("<config>App</config>\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"App",
		".java",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (only .java), got %d", len(results))
	}
}

func TestSearchWithGo_ExcludeDirs(t *testing.T) {
	tmpDir := t.TempDir()
	// 在 node_modules 下创建文件（应被排除）
	subDir := filepath.Join(tmpDir, "node_modules")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "package.js"), []byte("var config = {};\n"), 0644)
	// 在根目录创建文件
	os.WriteFile(filepath.Join(tmpDir, "main.js"), []byte("var config = {};\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"config",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (node_modules excluded), got %d", len(results))
	}
}

func TestSearchWithGo_SkipBinary(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建含 NUL 字节的二进制文件
	binFile := filepath.Join(tmpDir, "data.bin")
	os.WriteFile(binFile, []byte("hello\x00world\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 0 {
		t.Fatalf("expected 0 results (binary skipped), got %d", len(results))
	}
}

func TestSearchWithGo_MaxResults(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	// 10 行匹配
	content := ""
	for i := 0; i < 10; i++ {
		content += "keyword match line\n"
	}
	os.WriteFile(testFile, []byte(content), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"keyword",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		3, // 限制 3 条
	)

	if len(results) > 3 {
		t.Fatalf("expected at most 3 results, got %d", len(results))
	}
}

func TestSearchWithGo_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("Hello World\nHELLO WORLD\nhello world\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"hello",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	if len(results) != 3 {
		t.Fatalf("expected 3 results (case insensitive), got %d", len(results))
	}
}

func TestSearchWithGo_SubDir(t *testing.T) {
	tmpDir := t.TempDir()
	// 创建子目录结构
	os.MkdirAll(filepath.Join(tmpDir, "src", "main"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("target keyword\n"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "src", "main", "app.txt"), []byte("target keyword\n"), 0644)

	svc := &ContentSearchService{}
	results := svc.searchWithGo(
		context.Background(),
		tmpDir,
		"keyword",
		"",
		defaultExcludeDirs,
		defaultExcludeFiles,
		20,
	)

	// 搜索整个目录，应该找到 2 个
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestIsExcludedDir(t *testing.T) {
	if !isExcludedDir("node_modules", defaultExcludeDirs) {
		t.Error("node_modules should be excluded")
	}
	if !isExcludedDir(".git", defaultExcludeDirs) {
		t.Error(".git should be excluded")
	}
	if isExcludedDir("src", defaultExcludeDirs) {
		t.Error("src should not be excluded")
	}
}

func TestIsExcludedFile(t *testing.T) {
	if !isExcludedFile("app.log", defaultExcludeFiles) {
		t.Error("app.log should be excluded")
	}
	if !isExcludedFile("App.class", defaultExcludeFiles) {
		t.Error("App.class should be excluded")
	}
	if isExcludedFile("App.java", defaultExcludeFiles) {
		t.Error("App.java should not be excluded")
	}
}

func TestContentSearch_MultipleDirs(t *testing.T) {
	// 创建两个临时目录模拟多个工作目录
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	os.WriteFile(filepath.Join(dir1, "a.txt"), []byte("findme in dir1\n"), 0644)
	os.WriteFile(filepath.Join(dir2, "b.txt"), []byte("findme in dir2\n"), 0644)

	svc := NewContentSearchService()
	groups, err := svc.ContentSearch(
		context.Background(),
		[]string{dir1, dir2},
		[]string{"repo1", "repo2"},
		"findme",
		"", "", // 无文件类型过滤，无子目录
		nil, nil, // 使用默认排除
		20,
	)

	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./service/ -run TestSearch -v`
Expected: 全部 PASS

- [ ] **Step 3: Commit**

```bash
git add service/content_search_test.go
git commit -m "test(search): add unit tests for ContentSearchService"
```

---

### Task 5: Wails 绑定 — app.go 新增 ContentSearch 方法

**Files:**
- Modify: `app.go`

- [ ] **Step 1: 在 App 结构体中新增 contentSearchSvc 字段**

在 `app.go` 的 `App` 结构体中新增字段：

```go
type App struct {
	ctx              context.Context
	directorySvc     *service.DirectoryService
	fileTreeSvc      *service.FileTreeService
	fileOpSvc        *service.FileOperationService
	gitSvc           *service.GitService
	settingsSvc      *service.SettingsService
	terminalSvc      *service.TerminalService
	searchSvc        *service.SearchService
	favoritesSvc     *service.FavoritesService
	contentSearchSvc *service.ContentSearchService // 新增
}
```

- [ ] **Step 2: 在 startup 方法中初始化 contentSearchSvc**

在 `startup` 方法中，`a.searchSvc = service.NewSearchService()` 之后添加：

```go
	a.contentSearchSvc = service.NewContentSearchService()
```

- [ ] **Step 3: 新增 ContentSearch 绑定方法**

在 `app.go` 的 `// ===== 搜索相关 =====` 区域中，`SearchFiles` 方法之后添加：

```go
// ContentSearch 内容搜索
// keyword: 搜索关键词
// fileExt: 文件类型过滤（如 ".java"，为空则不过滤）
// subDir: 子目录路径（相对于工作目录，为空则搜索整个目录）
// searchAll: 是否搜索所有工作目录（true 搜索全部，false 仅当前目录）
func (a *App) ContentSearch(keyword, fileExt, subDir string, searchAll bool) ([]*model.ContentSearchGroup, error) {
	if keyword == "" {
		return nil, nil
	}

	// 加载设置获取排除配置
	settings, _ := a.settingsSvc.Load()

	// 确定搜索目录列表
	var dirs []string
	var repoNames []string

	if searchAll {
		directories, err := a.directorySvc.Load()
		if err != nil {
			return nil, err
		}
		for _, d := range directories {
			dirs = append(dirs, d.Path)
			repoNames = append(repoNames, d.Name)
		}
	} else {
		// 仅当前选中的工作目录
		directories, _ := a.directorySvc.Load()
		for _, d := range directories {
			if d.IsDefault {
				dirs = append(dirs, d.Path)
				repoNames = append(repoNames, d.Name)
				break
			}
		}
		// 如果没有默认目录，用第一个
		if len(dirs) == 0 && len(directories) > 0 {
			dirs = append(dirs, directories[0].Path)
			repoNames = append(repoNames, directories[0].Name)
		}
	}

	if len(dirs) == 0 {
		return nil, nil
	}

	// 全局搜索超时 60s，单目录 10s
	timeout := 10
	if searchAll {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	return a.contentSearchSvc.ContentSearch(
		ctx, dirs, repoNames,
		keyword, fileExt, subDir,
		settings.SearchExcludeDirs, settings.SearchExcludeFiles,
		20,
	)
}
```

注意：需要在 `app.go` 的 import 中确认有 `"time"` 包。

- [ ] **Step 4: 编译确认**

Run: `go build ./...`
Expected: 编译通过

- [ ] **Step 5: Commit**

```bash
git add app.go
git commit -m "feat(search): add ContentSearch Wails binding in app.go"
```

---

### Task 6: 前端 composable — useCommandPalette 新增内容搜索模式

**Files:**
- Modify: `frontend/src/composables/useCommandPalette.js`

- [ ] **Step 1: 重写 useCommandPalette 支持内容搜索**

完整替换 `frontend/src/composables/useCommandPalette.js`：

```js
import { ref, computed } from 'vue'
import { SearchFiles, ContentSearch } from '../../wailsjs/go/main/App'

export function useCommandPalette() {
  const visible = ref(false)
  const input = ref('')
  const selectedIndex = ref(0)
  const fileResults = ref([])
  const searchLoading = ref(false)

  // 内容搜索相关状态
  const contentGroups = ref([])
  const contentSearching = ref(false)
  const contentSearchProgress = ref('')

  const mode = computed(() => {
    if (input.value.startsWith('::')) return 'content-global'
    if (input.value.startsWith(':')) return 'content'
    if (input.value.startsWith('#')) return 'workdir'
    if (input.value.startsWith('@')) return 'favorites'
    if (input.value.startsWith('>')) return 'command'
    return 'general'
  })

  const query = computed(() => {
    if (mode.value === 'content' || mode.value === 'content-global') {
      return input.value.replace(/^::?/, '').trim()
    }
    if (mode.value !== 'general') {
      return input.value.slice(1).trim()
    }
    return input.value.trim()
  })

  // 解析内容搜索查询参数
  const contentQuery = computed(() => {
    const raw = query.value
    if (!raw) return { keyword: '', fileExt: '', subDir: '' }

    let remaining = raw
    let fileExt = ''
    let subDir = ''

    // 提取文件类型（以 . 开头的第一个词）
    const extRegex = /^\.(\w+)\s+/
    const extMatch = remaining.match(extRegex)
    if (extMatch) {
      fileExt = '.' + extMatch[1]
      remaining = remaining.slice(extMatch[0].length)
    }

    // 提取子目录路径（以 / 或 \ 结尾的部分）
    const pathRegex = /^(.+?)[/\\]\s+/
    const pathMatch = remaining.match(pathRegex)
    if (pathMatch) {
      subDir = pathMatch[1].replace(/[/\\]$/, '')
      remaining = remaining.slice(pathMatch[0].length)
    }

    return { keyword: remaining.trim(), fileExt, subDir }
  })

  function open() {
    visible.value = true
    input.value = ''
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
  }

  function close() {
    visible.value = false
    input.value = ''
    fileResults.value = []
    contentGroups.value = []
  }

  function openWithContentSearch(subDir) {
    visible.value = true
    input.value = ':' + subDir + '/ '
    selectedIndex.value = 0
    fileResults.value = []
    contentGroups.value = []
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

  async function executeContentSearch(currentDirPath) {
    const { keyword, fileExt, subDir } = contentQuery.value
    if (!keyword) return

    const isGlobal = mode.value === 'content-global'
    contentSearching.value = true
    contentSearchProgress.value = '搜索中...'
    contentGroups.value = []

    try {
      const groups = await ContentSearch(keyword, fileExt, subDir, isGlobal)
      contentGroups.value = groups || []
    } catch (e) {
      contentGroups.value = []
    } finally {
      contentSearching.value = false
      contentSearchProgress.value = ''
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
    contentQuery,
    contentGroups,
    contentSearching,
    contentSearchProgress,
    open,
    close,
    openWithContentSearch,
    searchFiles,
    executeContentSearch,
    moveSelection,
    resetSelection
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/composables/useCommandPalette.js
git commit -m "feat(search): add content/content-global mode to useCommandPalette"
```

---

### Task 7: 前端 UI — CommandPalette.vue 新增内容搜索区域

**Files:**
- Modify: `frontend/src/components/CommandPalette.vue`

- [ ] **Step 1: 更新 script 部分 — 引入新状态和方法**

在 `<script setup>` 中，替换 `useCommandPalette()` 解构调用为完整版本：

```js
const {
  input, mode, query, selectedIndex,
  fileResults, searchLoading,
  contentQuery, contentGroups, contentSearching, contentSearchProgress,
  searchFiles, executeContentSearch, openWithContentSearch,
  resetSelection
} = useCommandPalette()
```

新增 `onConfirmContentSearch` 方法（在 `onInput` 之后）：

```js
function onConfirmContentSearch() {
  executeContentSearch(props.currentDir)
}
```

修改 `onInput` 方法，去掉内容搜索相关的防抖（内容搜索统一 Enter 触发）：

```js
function onInput() {
  resetSelection()

  if (mode.value === 'favorites') {
    favoriteResults.value = searchFavorites(query.value)
  } else if (mode.value === 'general' && query.value) {
    favoriteResults.value = searchFavorites(query.value).slice(0, 5)
    searchTimer = setTimeout(() => {
      searchFiles(props.currentDir)
    }, 300)
  } else if (mode.value === 'content' || mode.value === 'content-global') {
    // 内容搜索不在输入时触发，仅清空上次结果
    favoriteResults.value = []
    fileResults.value = []
  } else {
    favoriteResults.value = []
    fileResults.value = []
  }
}
```

修改 `selectCurrent` 方法，新增内容搜索 Enter 处理分支：

在 `selectCurrent` 函数的开头新增内容搜索判断：

```js
function selectCurrent() {
  // 内容搜索模式：Enter 触发搜索
  if (mode.value === 'content' || mode.value === 'content-global') {
    if (contentSearching.value) return
    // 如果已有结果，Enter 选择当前高亮项
    if (contentGroups.value.length > 0) {
      const item = getContentItemByIndex(selectedIndex.value)
      if (item) {
        openContentResultInVSCode(item)
        onClose()
        return
      }
    }
    // 没有结果或无高亮，触发搜索
    if (contentQuery.value.keyword) {
      onConfirmContentSearch()
    }
    return
  }

  // ... 原有 selectCurrent 逻辑保持不变
}
```

新增内容搜索辅助方法（在 `selectCurrent` 之后）：

```js
// 获取内容搜索结果总数
const contentTotalItems = computed(() => {
  return contentGroups.value.reduce((sum, g) => sum + g.items.length, 0)
})

// 根据全局 index 获取内容搜索结果项
function getContentItemByIndex(index) {
  let offset = 0
  for (const group of contentGroups.value) {
    if (index < offset + group.items.length) {
      return group.items[index - offset]
    }
    offset += group.items.length
  }
  return null
}

// 用 VSCode 打开内容搜索结果（跳转到行号）
function openContentResultInVSCode(item) {
  const fullPath = item.repoPath + '\\' + item.filePath.replace(/\//g, '\\')
  const gotoArg = `${fullPath}:${item.lineNum}`
  // 使用 Wails 后端 OpenInVSCode 不支持 --goto，直接用 runtime.BrowserOpenURL 或命令行
  window['runtime']?.BrowserOpenURL(`vscode://file/${fullPath}:${item.lineNum}`)
  // 备选方案：如果 vscode:// 协议不生效，可以通过后端调用 code --goto
}

// 关键词高亮
function highlightMatch(text, keyword) {
  if (!keyword) return escapeHtml(text)
  const escaped = escapeHtml(text)
  const keywordEscaped = escapeHtml(keyword).replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return escaped.replace(new RegExp(keywordEscaped, 'gi'), '<mark>$&</mark>')
}

function escapeHtml(str) {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
```

- [ ] **Step 2: 更新 template — 新增内容搜索结果区域**

在 `<div class="palette-content">` 内部，在「文件搜索结果」section 之后、「加载和空状态」之前，新增以下模板：

```html
      <!-- 内容搜索 - 全局搜索提示 -->
      <div v-if="(mode === 'content-global') && !contentSearching && contentGroups.length === 0 && contentQuery.keyword" class="result-section">
        <div class="content-search-confirm">
          <el-icon><Search /></el-icon>
          <span>将在 {{ workDirs.length }} 个工作目录中搜索 "<strong>{{ contentQuery.keyword }}</strong>"</span>
          <span class="hint">按 Enter 确认搜索</span>
        </div>
      </div>

      <!-- 内容搜索 - 输入提示（单目录） -->
      <div v-if="mode === 'content' && !contentSearching && contentGroups.length === 0 && contentQuery.keyword" class="result-section">
        <div class="content-search-hint">
          <el-icon><Search /></el-icon>
          <span>搜索 "{{ contentQuery.keyword }}"</span>
          <span v-if="contentQuery.fileExt" class="hint-tag">{{ contentQuery.fileExt }}</span>
          <span v-if="contentQuery.subDir" class="hint-tag">{{ contentQuery.subDir }}/</span>
          <span class="hint">按 Enter 搜索</span>
        </div>
      </div>

      <!-- 内容搜索结果 -->
      <div v-if="contentGroups.length > 0" class="result-section">
        <div v-for="group in contentGroups" :key="group.repoName" class="content-group">
          <div class="section-title content-group-title">
            <el-icon><Folder /></el-icon>
            {{ group.repoName }}
          </div>
          <div
            v-for="(item, idx) in group.items"
            :key="group.repoName + '-' + idx"
            class="result-item"
            :class="{ 'result-item--active': getContentItemIndex(group, idx) === selectedIndex }"
            @click="openContentResultInVSCode(item); onClose()"
            @mouseenter="selectedIndex = getContentItemIndex(group, idx)"
          >
            <div class="result-info">
              <div class="result-name content-file-line">{{ item.filePath }}:<span class="line-num">{{ item.lineNum }}</span></div>
              <div class="result-line" v-html="highlightMatch(item.lineText, contentQuery.keyword)"></div>
            </div>
          </div>
        </div>
      </div>
```

同时需要新增 `getContentItemIndex` 辅助方法：

```js
function getContentItemIndex(group, localIdx) {
  let offset = 0
  for (const g of contentGroups.value) {
    if (g.repoName === group.repoName) {
      return offset + localIdx
    }
    offset += g.items.length
  }
  return offset + localIdx
}
```

- [ ] **Step 3: 更新 placeholder**

将输入框的 `placeholder` 改为：

```
placeholder="搜索文件、目录 (#工作目录 @收藏夹 :内容搜索)"
```

- [ ] **Step 4: 更新空状态条件**

将原有的空状态 div 条件扩展，增加内容搜索模式判断：

```html
<div v-if="mode === 'general' && query && !searchLoading && fileResults.length === 0 && favoriteResults.length === 0" class="result-empty">
  未找到匹配项
</div>
<div v-if="(mode === 'content' || mode === 'content-global') && !contentSearching && contentGroups.length === 0 && contentQuery.keyword && (mode === 'content-global' ? false : true)" class="result-empty">
  未找到匹配内容
</div>
```

等简化一下，把第二个空状态放在内容搜索结果后面：

```html
<div v-if="(mode === 'content' || mode === 'content-global') && contentSearching === false && contentGroups.length === 0 && contentQuery.keyword && searchExecuted" class="result-empty">
  未找到匹配内容
</div>
```

需要新增 `searchExecuted` ref，在 `executeContentSearch` 中搜索完成后设为 true：

在 `useCommandPalette.js` 中新增：

```js
const contentSearchExecuted = ref(false)
```

在 `executeContentSearch` 的 finally 中：

```js
contentSearchExecuted.value = true
```

在 `onInput` 清空时重置：

```js
contentSearchExecuted.value = false
```

同时将 `contentSearchExecuted` 加入 return。

- [ ] **Step 5: 新增样式**

在 `<style scoped>` 中追加：

```css
/* 内容搜索确认提示 */
.content-search-confirm {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 14px 20px;
  background: #fdf6ec;
  color: #e6a23c;
  font-size: 13px;
}

.content-search-confirm strong {
  color: #303133;
}

.content-search-hint {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 20px;
  color: #909399;
  font-size: 13px;
}

.hint {
  color: #c0c4cc;
  font-size: 12px;
  margin-left: auto;
}

.hint-tag {
  background: #f0f2f5;
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: #606266;
}

/* 内容搜索分组 */
.content-group {
  margin-bottom: 4px;
}

.content-group-title {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #409eff !important;
  font-weight: 500;
}

/* 匹配行内容 */
.content-file-line {
  font-size: 12px !important;
  color: #606266 !important;
}

.line-num {
  color: #409eff;
  font-weight: 600;
}

.result-line {
  font-size: 12px;
  color: #303133;
  padding: 2px 0;
  font-family: 'Consolas', 'Monaco', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: #f5f7fa;
  padding: 3px 6px;
  border-radius: 3px;
  margin-top: 3px;
}

.result-line :deep(mark) {
  background: #fde68a;
  color: #92400e;
  padding: 0 1px;
  border-radius: 2px;
}
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/CommandPalette.vue frontend/src/composables/useCommandPalette.js
git commit -m "feat(search): add content search UI in CommandPalette"
```

---

### Task 8: 前端 — Home.vue 连接内容搜索事件

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 新增 open-content-search 事件处理**

在 `Home.vue` 的 `<CommandPalette>` 标签上新增属性和事件：

```html
<CommandPalette
  v-model="commandPaletteVisible"
  :current-dir="currentDirPath"
  :work-dirs="directories"
  @select-file="onPaletteSelectFile"
  @select-favorite="onPaletteSelectFavorite"
  @select-workdir="onPaletteSelectWorkDir"
  @open-content-search="onOpenContentSearch"
/>
```

在 `<FileTreePanel>` 标签上新增事件：

```html
<FileTreePanel
  ref="fileTreePanelRef"
  :directories="directories"
  :selected-dir-id="selectedDirectoryId"
  :clipboard="clipboard"
  @select="onNodeSelect"
  @batch-pull="onBatchPull"
  @copy="handleCopy"
  @cut="handleCut"
  @paste="handlePaste"
  @copy-to="handleCopyTo"
  @contextmenu="onFileTreeContextMenu"
  @delete="onDeleteFromFileTree"
  @add-work-dir="onAddWorkDir"
  @open-content-search="onOpenContentSearch"
/>
```

新增处理函数（在 `onPaletteSelectWorkDir` 之后）：

```js
function onOpenContentSearch(subDir) {
  commandPaletteVisible.value = true
  // 等 CommandPalette 渲染后设置输入
  nextTick(() => {
    // 通过 ref 调用 CommandPalette 的 openWithContentSearch
    // CommandPalette 需要暴露该方法
  })
}
```

由于 `useCommandPalette` 是在 `CommandPalette.vue` 内部使用的 composable，`Home.vue` 无法直接调用其方法。改用 props 传递初始值方案：

在 `Home.vue` 新增 ref：

```js
const contentSearchInit = ref('')
```

修改 `onOpenContentSearch`：

```js
function onOpenContentSearch(subDir) {
  contentSearchInit.value = subDir ? ':' + subDir + '/ ' : ':'
  commandPaletteVisible.value = true
}
```

在 `<CommandPalette>` 上新增 prop：

```html
:content-search-init="contentSearchInit"
```

在 `CommandPalette.vue` 中接收 prop 并在 watch 中使用：

```js
const props = defineProps({
  modelValue: Boolean,
  currentDir: String,
  workDirs: Array,
  contentSearchInit: { type: String, default: '' }
})

// 在 watch visible 中：
watch(visible, async (val) => {
  if (val) {
    recentItems.value = getRecent(10)
    await loadFavorites()
    if (props.contentSearchInit) {
      input.value = props.contentSearchInit
    }
    await nextTick()
    searchInputRef.value?.focus()
  }
})
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/views/Home.vue frontend/src/components/CommandPalette.vue
git commit -m "feat(search): wire content search events from Home.vue"
```

---

### Task 9: 前端 — FileTreePanel 右键菜单新增「在此目录中搜索」

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

- [ ] **Step 1: 新增 emit 声明**

在 `FileTreePanel.vue` 的 `defineEmits` 中新增 `open-content-search`：

```js
const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste', 'copyTo', 'contextmenu', 'delete', 'add-work-dir', 'open-content-search'])
```

- [ ] **Step 2: 在右键菜单目录节点模板中新增菜单项**

在 `<template v-else-if="contextMenu.data?.type === 'directory'">` 内，在「更新仓库」菜单项的分隔线之前（约 218 行 `li class="context-menu-divider"` 之前），新增：

```html
        <li class="context-menu-item" @click="onMenuCommand('contentSearch')">
          <el-icon><Search /></el-icon>在此目录中搜索
        </li>
```

需要在 icons-vue 导入中确认有 `Search` 图标。检查文件顶部 import，如没有则添加。

- [ ] **Step 3: 在 onMenuCommand 中新增 contentSearch 分支**

在 `onMenuCommand` 函数的 switch/if 分支中新增：

```js
case 'contentSearch':
  // 计算相对路径
  const currentWorkDir = props.directories.find(d => d.id === props.selectedDirId)
  if (currentWorkDir && data.path.startsWith(currentWorkDir.path)) {
    const relPath = data.path.slice(currentWorkDir.path.length).replace(/^[\\\/]/, '')
    emit('open-content-search', relPath)
  } else {
    emit('open-content-search', '')
  }
  break
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat(search): add 'Search in this directory' to file tree context menu"
```

---

### Task 10: 设置面板 — 搜索排除配置

**Files:**
- Modify: `frontend/src/components/SettingsPanel.vue`

- [ ] **Step 1: 新增搜索设置 tab**

在 `tabs` 数组中新增搜索 tab：

```js
const tabs = [
  { id: 'general', label: '通用' },
  { id: 'terminal', label: '终端' },
  { id: 'search', label: '搜索' },
  { id: 'shortcuts', label: '快捷键' }
]
```

- [ ] **Step 2: 新增搜索配置响应式变量**

在 `const wslDistro = ref('')` 之后新增：

```js
const excludeDirs = ref([])
const excludeFiles = ref([])
const newExcludeDir = ref('')
const newExcludeFile = ref('')
```

- [ ] **Step 3: 更新 loadSettings 函数**

在 `loadSettings` 函数中新增读取：

```js
excludeDirs.value = settings.searchExcludeDirs || []
excludeFiles.value = settings.searchExcludeFiles || []
```

- [ ] **Step 4: 新增搜索设置辅助方法**

```js
const addExcludeDir = () => {
  const val = newExcludeDir.value.trim()
  if (val && !excludeDirs.value.includes(val)) {
    excludeDirs.value.push(val)
    onSettingsChange()
  }
  newExcludeDir.value = ''
}

const removeExcludeDir = (tag) => {
  excludeDirs.value = excludeDirs.value.filter(d => d !== tag)
  onSettingsChange()
}

const addExcludeFile = () => {
  const val = newExcludeFile.value.trim()
  if (val && !excludeFiles.value.includes(val)) {
    excludeFiles.value.push(val)
    onSettingsChange()
  }
  newExcludeFile.value = ''
}

const removeExcludeFile = (tag) => {
  excludeFiles.value = excludeFiles.value.filter(f => f !== tag)
  onSettingsChange()
}
```

- [ ] **Step 5: 更新 SaveSettings 调用**

修改 `onSettingsChange` 和 `onGpuChange` 中的 `SaveSettings` 参数，新增搜索排除字段：

```js
await SaveSettings({
  gpuDisabled: !gpuEnabled.value,
  defaultShell: defaultShell.value,
  gitBashPath: gitBashPath.value,
  wslDistro: wslDistro.value,
  searchExcludeDirs: excludeDirs.value,
  searchExcludeFiles: excludeFiles.value
})
```

- [ ] **Step 6: 新增搜索设置 tab 模板**

在 `<div v-show="activeTab === 'terminal'">` 的结束标签之后、`<!-- 快捷键页 -->` 之前新增：

```html
        <!-- 搜索页 -->
        <div v-show="activeTab === 'search'">
          <div class="settings-section-title">搜索</div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除目录</div>
              <div class="settings-item-desc">搜索时跳过这些目录</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="dir in excludeDirs"
                :key="dir"
                closable
                size="small"
                @close="removeExcludeDir(dir)"
              >{{ dir }}</el-tag>
              <el-input
                v-model="newExcludeDir"
                size="small"
                style="width: 120px;"
                placeholder="添加目录"
                @keyup.enter="addExcludeDir"
              />
              <el-button size="small" @click="addExcludeDir">添加</el-button>
            </div>
          </div>
          <div class="settings-item settings-item--column">
            <div class="settings-item-info">
              <div class="settings-item-label">排除文件</div>
              <div class="settings-item-desc">搜索时跳过这些扩展名的文件</div>
            </div>
            <div class="settings-tags">
              <el-tag
                v-for="file in excludeFiles"
                :key="file"
                closable
                size="small"
                @close="removeExcludeFile(file)"
              >{{ file }}</el-tag>
              <el-input
                v-model="newExcludeFile"
                size="small"
                style="width: 120px;"
                placeholder="如 .log"
                @keyup.enter="addExcludeFile"
              />
              <el-button size="small" @click="addExcludeFile">添加</el-button>
            </div>
          </div>
        </div>
```

- [ ] **Step 7: 新增样式**

在 `<style scoped>` 中追加：

```css
.settings-item--column {
  flex-direction: column;
  align-items: flex-start;
  gap: 10px;
}

.settings-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  width: 100%;
}
```

- [ ] **Step 8: Commit**

```bash
git add frontend/src/components/SettingsPanel.vue
git commit -m "feat(search): add search exclude config in SettingsPanel"
```

---

### Task 11: 集成测试和最终验证

**Files:** 无新增

- [ ] **Step 1: 运行后端全量测试**

Run: `go test ./... -v`
Expected: 全部 PASS

- [ ] **Step 2: 重新生成 Wails 绑定**

Run: `wails generate module`
Expected: 生成 `ContentSearch` 的前端绑定文件

- [ ] **Step 3: 编译前端确认无错误**

Run: `cd frontend && npm run build`
Expected: 编译成功

- [ ] **Step 4: 启动应用手动验证**

Run: `wails dev`

验证清单：
1. 打开 Command Palette → 输入 `:test` → 按 Enter → 确认搜索结果展示
2. 输入 `:test` → 确认显示「按 Enter 搜索」提示
3. 输入 `::test` → 确认显示全局搜索确认提示
4. 输入 `:.java test` → 确认只搜索 .java 文件
5. 右键文件树目录 → 点击「在此目录中搜索」→ 确认 Command Palette 打开并预填路径
6. 设置 → 搜索 tab → 添加/删除排除目录 → 确认保存生效
7. 点击搜索结果 → 确认 VSCode 打开对应文件和行号

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(search): cross-repository content search - complete integration"
```
