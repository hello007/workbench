# 批量更新仓库 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在文件树文件夹节点的右键菜单中新增"更新仓库"功能，支持递归扫描并批量并行拉取所有 Git 仓库，实时展示进度。

**Architecture:** 后端 Go 侧新增 `ScanGitRepos`（递归扫描）和 `BatchPull`（goroutine 池并行拉取），通过 Wails `runtime.EventsEmit` 推送每个仓库的完成事件。前端监听事件实时更新 Element Plus 弹窗中的进度条和结果表格。

**Tech Stack:** Go 1.24 (goroutine + channel semaphore + Wails EventsEmit), Vue 3 (Composition API + Element Plus el-dialog/el-table/el-progress + Wails EventsOn)

---

### Task 1: 新增 PullResult 和 PullSummary 数据模型

**Files:**
- Modify: `model/models.go` (追加到文件末尾)

**Step 1: 在 `model/models.go` 末尾追加两个结构体**

在文件最后一行 `}` 之后追加：

```go
// PullResult 单个仓库的拉取结果
type PullResult struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// PullSummary ScanAndPullRepos 的初始返回值
type PullSummary struct {
	Total int `json:"total"`
}
```

**Step 2: 验证编译通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go build ./...`
Expected: 编译成功，无错误

**Step 3: Commit**

```bash
git add model/models.go
git commit -m "feat: add PullResult and PullSummary models for batch pull"
```

---

### Task 2: 实现 ScanGitRepos 递归扫描方法

**Files:**
- Modify: `service/git.go`
- Create: `service/git_test.go`

**Step 1: 编写 ScanGitRepos 的测试**

创建 `service/git_test.go`：

```go
package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScanGitRepos_SingleRepo(t *testing.T) {
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	svc := NewGitService()
	repos := svc.ScanGitRepos(dir)

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0] != dir {
		t.Errorf("expected %s, got %s", dir, repos[0])
	}
}

func TestScanGitRepos_NestedRepos(t *testing.T) {
	root := t.TempDir()

	// 创建三个 git 仓库
	repoA := filepath.Join(root, "project-a")
	repoB := filepath.Join(root, "subdir", "project-b")
	repoC := filepath.Join(root, "subdir", "deep", "project-c")

	for _, repo := range []string{repoA, repoB, repoC} {
		os.MkdirAll(repo, 0755)
		runGit(t, repo, "init")
		runGit(t, repo, "config", "user.email", "test@test.com")
		runGit(t, repo, "config", "user.name", "test")
	}

	svc := NewGitService()
	repos := svc.ScanGitRepos(root)

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d: %v", len(repos), repos)
	}
}

func TestScanGitRepos_NoRepos(t *testing.T) {
	dir := t.TempDir()

	svc := NewGitService()
	repos := svc.ScanGitRepos(dir)

	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v in %s failed: %v", args, dir, err)
	}
}
```

**Step 2: 运行测试验证失败**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestScanGitRepos -v`
Expected: FAIL — `svc.ScanGitRepos` 方法不存在

**Step 3: 在 `service/git.go` 中实现 ScanGitRepos**

在 `service/git.go` 文件末尾（`ExtractRepoName` 方法之后）追加：

```go
// ScanGitRepos 递归扫描目录下所有 Git 仓库
// 如果 rootPath 本身是 git 仓库，直接返回 [rootPath]
// 否则递归遍历子目录，收集所有 git 仓库路径
func (s *GitService) ScanGitRepos(rootPath string) []string {
	if s.gitCmd.IsGitRepository(rootPath) {
		return []string{rootPath}
	}

	var repos []string
	s.scanDir(rootPath, &repos)
	return repos
}

func (s *GitService) scanDir(dir string, repos *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		fullPath := filepath.Join(dir, entry.Name())
		// 跳过 .git 目录本身
		if entry.Name() == ".git" {
			continue
		}

		if s.gitCmd.IsGitRepository(fullPath) {
			*repos = append(*repos, fullPath)
		} else {
			s.scanDir(fullPath, repos)
		}
	}
}
```

需要在文件顶部的 import 中确认已有 `"os"` 和 `"path/filepath"`。当前 import 只有 `fmt`, `strings`, `model`, `util`，需要追加：

将 import 块改为：

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"workbench/model"
	"workbench/util"
)
```

**Step 4: 运行测试验证通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestScanGitRepos -v`
Expected: PASS — 全部 3 个测试通过

**Step 5: Commit**

```bash
git add service/git.go service/git_test.go
git commit -m "feat: implement ScanGitRepos for recursive git repo discovery"
```

---

### Task 3: 实现 BatchPull 并行拉取方法

**Files:**
- Modify: `service/git.go` (追加方法)
- Modify: `service/git_test.go` (追加测试)

**Step 1: 编写 BatchPull 的测试**

在 `service/git_test.go` 末尾追加：

```go
import (
	"context"
	// ... 其他已有的 import
)

func TestBatchPull_SuccessAndFail(t *testing.T) {
	dir := t.TempDir()

	// 创建一个真实的 git 仓库（无远程，pull 会失败）
	repoPath := filepath.Join(dir, "repo")
	os.MkdirAll(repoPath, 0755)
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.email", "test@test.com")
	runGit(t, repoPath, "config", "user.name", "test")

	// 创建一个非 git 目录（会失败）
	nonRepo := filepath.Join(dir, "not-a-repo")
	os.MkdirAll(nonRepo, 0755)

	svc := NewGitService()
	results := svc.BatchPull([]string{repoPath, nonRepo}, 2, context.Background())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// repoPath 应该是失败的（无远程配置）
	repoResult := results[0]
	if repoResult.Path != repoPath {
		t.Errorf("expected path %s, got %s", repoPath, repoResult.Path)
	}

	// nonRepo 应该是失败的
	nonRepoResult := results[1]
	if nonRepoResult.Success {
		t.Error("expected non-repo to fail")
	}
	if nonRepoResult.Error == "" {
		t.Error("expected error message for non-repo")
	}
}
```

注意：需要在已有的 import 块中追加 `"context"`。

**Step 2: 运行测试验证失败**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestBatchPull -v`
Expected: FAIL — `svc.BatchPull` 方法不存在

**Step 3: 在 `service/git.go` 中实现 BatchPull**

在 `service/git.go` 文件末尾追加：

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"
)

// BatchPull 并行拉取多个 Git 仓库
func (s *GitService) BatchPull(repos []string, concurrency int, ctx context.Context) []model.PullResult {
	if concurrency <= 0 {
		concurrency = 5
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		results  []model.PullResult
		sem      = make(chan struct{}, concurrency)
		successCount int
		failCount    int
	)

	for _, repo := range repos {
		wg.Add(1)
		go func(repoPath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			name := filepath.Base(repoPath)
			result := model.PullResult{
				Path: repoPath,
				Name: name,
			}

			if !s.gitCmd.IsGitRepository(repoPath) {
				result.Success = false
				result.Error = "不是 Git 仓库"
			} else {
				gitCmd := util.NewGitCommandWithTimeout(5 * time.Minute)
				output, err := gitCmd.Pull(repoPath)
				if err != nil {
					result.Success = false
					result.Error = err.Error()
				} else {
					result.Success = true
					result.Output = strings.TrimSpace(output)
				}
			}

			mu.Lock()
			results = append(results, result)
			if result.Success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()

			runtime.EventsEmit(ctx, "pull-progress", result)
		}(repo)
	}

	wg.Wait()

	runtime.EventsEmit(ctx, "pull-complete", map[string]int{
		"success": successCount,
		"failed":  failCount,
	})

	return results
}
```

需要在 import 中追加 `"sync"`, `"time"`, `"github.com/wailsapp/wails/v2/pkg/runtime"`：

将 `service/git.go` 的完整 import 块改为：

```go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workbench/model"
	"workbench/util"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)
```

**Step 4: 在 `util/git.go` 中添加 NewGitCommandWithTimeout**

在 `util/git.go` 中 `NewGitCommand` 函数之后追加：

```go
// NewGitCommandWithTimeout 创建指定超时时间的 Git 命令执行器
func NewGitCommandWithTimeout(timeout time.Duration) *GitCommand {
	return &GitCommand{
		timeout: timeout,
	}
}
```

**Step 5: 运行测试验证通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./service/ -run TestBatchPull -v`
Expected: PASS

**Step 6: Commit**

```bash
git add service/git.go service/git_test.go util/git.go
git commit -m "feat: implement BatchPull with goroutine pool and event emission"
```

---

### Task 4: 新增 App.ScanAndPullRepos 绑定方法

**Files:**
- Modify: `app.go`

**Step 1: 在 `app.go` 中新增 ScanAndPullRepos 方法**

在 `PullRepo` 方法（第 216-222 行）之后追加：

```go
// ScanAndPullRepos 扫描并批量拉取 Git 仓库
func (a *App) ScanAndPullRepos(dirPath string) (*model.PullSummary, error) {
	repos := a.gitSvc.ScanGitRepos(dirPath)
	if len(repos) == 0 {
		return nil, fmt.Errorf("未找到任何 Git 仓库")
	}

	summary := &model.PullSummary{Total: len(repos)}

	go func() {
		a.gitSvc.BatchPull(repos, 5, a.ctx)
	}()

	return summary, nil
}
```

**Step 2: 验证编译通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go build ./...`
Expected: 编译成功

**Step 3: Commit**

```bash
git add app.go
git commit -m "feat: add ScanAndPullRepos binding method with async batch pull"
```

---

### Task 5: 重新生成 Wails 前端绑定

**Files:**
- Modify: `frontend/wailsjs/go/main/App.js` (自动生成，无需手动编辑)
- Modify: `frontend/wailsjs/go/main/App.d.ts` (自动生成)

**Step 1: 运行 Wails 生成命令**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails generate module`

Expected: 成功生成，`App.js` 中新增 `ScanAndPullRepos` 导出函数

**Step 2: 验证绑定已生成**

Run: `grep "ScanAndPullRepos" d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend/wailsjs/go/main/App.js`
Expected: 找到 `export function ScanAndPullRepos(arg1) {`

**Step 3: Commit**

```bash
git add frontend/wailsjs/go/main/App.js frontend/wailsjs/go/main/App.d.ts
git commit -m "chore: regenerate wails bindings for ScanAndPullRepos"
```

---

### Task 6: 前端 — 新增右键菜单项"更新仓库"

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 在右键菜单中添加"更新仓库"菜单项**

在 `Home.vue` 第 275 行（`<li class="context-menu-item" @click="onMenuCommand('openExplorer')">` 之后、`</template>` 之前）插入：

```html
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
          <el-icon><Refresh /></el-icon>更新仓库
        </li>
```

**Step 2: 在 script 的 import 中添加 Refresh 图标**

在 `Home.vue` 约第 302 行的图标 import 中，添加 `Refresh`：

将图标 import 改为：

```js
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Monitor,
  Refresh
} from '@element-plus/icons-vue'
```

**Step 3: 在 onMenuCommand switch 中添加 pullRepos case**

在 `Home.vue` 约第 527 行的 `case 'openExplorer'` 之后添加：

```js
    case 'pullRepos':
      handleBatchPull(data)
      break
```

**Step 4: 添加 handleBatchPull 函数占位**

在 `Home.vue` 约第 323 行的 import 中，添加 `ScanAndPullRepos`：

将 import 改为：

```js
import {
  GetDirectories, AddDirectory,
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo, CloneRepo,
  GetCommitHistory,
  OpenInExplorer,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'
```

在 `handleOpenExplorer` 函数之后（约第 650 行）添加占位函数：

```js
const handleBatchPull = async (data) => {
  // TODO: Task 7 实现
  ElMessage.info('更新仓库: ' + data.path)
}
```

**Step 5: 验证编译通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend && npm run build`
Expected: 编译成功

**Step 6: Commit**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat: add '更新仓库' context menu item for directories"
```

---

### Task 7: 前端 — 实现进度弹窗和事件监听

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 添加 Wails EventsOn import**

在 `Home.vue` 的 `<script setup>` 部分（约第 300 行），修改 import 添加 Wails runtime 事件函数：

在已有的 `import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'` 之后添加：

```js
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
```

**Step 2: 添加进度弹窗的响应式状态**

在 `Home.vue` 约第 358 行（`const cloneLoading = ref(false)` 之后）添加：

```js
const pullDialogVisible = ref(false)
const pullProgress = reactive({ current: 0, total: 0 })
const pullResults = ref([])
const pullCompleted = ref(false)
const pullSummary = reactive({ success: 0, failed: 0 })
```

**Step 3: 替换 handleBatchPull 占位函数**

将 Task 6 中的占位函数替换为完整实现：

```js
const handleBatchPull = async (data) => {
  try {
    const summary = await ScanAndPullRepos(data.path)

    // 重置状态
    pullResults.value = []
    pullProgress.current = 0
    pullProgress.total = summary.total
    pullCompleted.value = false
    pullSummary.success = 0
    pullSummary.failed = 0
    pullDialogVisible.value = true
  } catch (error) {
    ElMessage.warning(error || '未找到任何 Git 仓库')
  }
}

const cleanupPullEvents = () => {
  EventsOff("pull-progress")
  EventsOff("pull-complete")
}

const setupPullEvents = () => {
  cleanupPullEvents()
  EventsOn("pull-progress", (result) => {
    pullResults.value = [...pullResults.value, result]
    pullProgress.current++
  })
  EventsOn("pull-complete", (summary) => {
    pullCompleted.value = true
    pullSummary.success = summary.success || 0
    pullSummary.failed = summary.failed || 0
  })
}
```

**Step 4: 在 onMounted 中注册事件监听**

在 `onMounted` 回调中（约第 807 行），在现有代码之后添加：

```js
  setupPullEvents()
```

**Step 5: 在 onBeforeUnmount 中清理事件监听**

在 `onBeforeUnmount` 回调中（约第 815 行），在现有代码之后添加：

```js
  cleanupPullEvents()
```

**Step 6: 在模板中添加进度弹窗**

在 `Home.vue` 模板中，在 `<!-- 自定义右键菜单 -->` 之前（约第 248 行），插入进度弹窗：

```html
    <!-- 更新仓库进度弹窗 -->
    <el-dialog
      v-model="pullDialogVisible"
      :title="pullCompleted ? '更新完成' : '更新仓库'"
      width="700px"
      :close-on-click-modal="false"
      :close-on-press-escape="!pullCompleted"
      :show-close="pullCompleted"
    >
      <div style="margin-bottom: 16px;">
        <el-progress
          :percentage="pullProgress.total > 0 ? Math.round(pullProgress.current / pullProgress.total * 100) : 0"
          :format="() => `${pullProgress.current} / ${pullProgress.total}`"
          :status="pullCompleted ? (pullSummary.failed > 0 ? 'warning' : 'success') : undefined"
        />
        <div v-if="pullCompleted" style="margin-top: 8px; color: #909399; font-size: 13px;">
          成功: {{ pullSummary.success }}，失败: {{ pullSummary.failed }}
        </div>
      </div>

      <el-table :data="pullResults" style="width: 100%" max-height="400" size="small">
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-icon v-if="row.success" color="#67C23A"><SuccessFilled /></el-icon>
            <el-icon v-else color="#F56C6C"><CircleCloseFilled /></el-icon>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="仓库名称" min-width="150" show-overflow-tooltip />
        <el-table-column prop="path" label="路径" min-width="250" show-overflow-tooltip />
        <el-table-column label="结果" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span v-if="row.success" style="color: #67C23A;">{{ row.output || '已是最新' }}</span>
            <span v-else style="color: #F56C6C;">{{ row.error }}</span>
          </template>
        </el-table-column>
      </el-table>

      <template #footer>
        <el-button
          type="primary"
          @click="pullDialogVisible = false"
          :disabled="!pullCompleted"
        >
          {{ pullCompleted ? '关闭' : '更新中...' }}
        </el-button>
      </template>
    </el-dialog>
```

**Step 7: 在图标 import 中添加 CircleCloseFilled**

将图标 import 改为：

```js
import {
  Folder,
  FolderOpened,
  Document,
  SuccessFilled,
  CircleCloseFilled,
  FolderAdd,
  DocumentAdd,
  Edit,
  Delete,
  CopyDocument,
  Monitor,
  Refresh
} from '@element-plus/icons-vue'
```

**Step 8: 验证编译通过**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend && npm run build`
Expected: 编译成功

**Step 9: Commit**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat: implement batch pull progress dialog with real-time events"
```

---

### Task 8: 端到端验证

**Files:** 无新增/修改

**Step 1: 运行后端全部测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && go test ./...`
Expected: 全部 PASS

**Step 2: 运行前端全部测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench/frontend && npm test`
Expected: 全部 PASS

**Step 3: 启动开发模式进行手动测试**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails dev`

手动测试用例：

| # | 测试场景 | 预期结果 |
|---|----------|----------|
| 1 | 右键文件节点 | 菜单中**不显示**"更新仓库" |
| 2 | 右键 git 仓库文件夹 | 菜单显示"更新仓库"，点击后弹窗显示 1 个仓库 |
| 3 | 右键非 git 目录（含子仓库） | 菜单显示"更新仓库"，点击后弹窗显示扫描到的所有子仓库 |
| 4 | 右键非 git 目录（无子仓库） | 菜单显示"更新仓库"，点击后弹出 warning 提示"未找到任何 Git 仓库" |
| 5 | 弹窗进行中 | 进度条实时更新，表格逐行显示结果，关闭按钮禁用 |
| 6 | 全部完成 | 标题变为"更新完成"，底部显示汇总，关闭按钮启用 |
| 7 | 部分仓库失败 | 失败行显示红色叉号和错误信息 |

**Step 4: 构建生产版本验证**

Run: `cd d:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails build -clean`
Expected: 构建成功，输出 `build/bin/workbench.exe`

**Step 5: 最终 Commit（如有修复）**

如果手动测试发现 bug 并修复后：

```bash
git add -A
git commit -m "fix: address issues found during e2e testing of batch pull"
```
