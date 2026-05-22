# 切换分支功能实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在 Git 仓库的操作面板中新增"切换分支"按钮，弹窗展示本地和远程分支列表，支持搜索过滤和分支切换。

**Architecture:** 后端新增两个 Wails 绑定方法（获取分支列表、切换分支），底层通过 `git branch -a` 和 `git checkout` 命令实现。前端在 ContentPanel.vue 中添加按钮和弹窗，使用 el-select 分组展示分支。

**Tech Stack:** Go / Wails v2 / Vue 3 / Element Plus

---

### Task 1: 新增数据模型

**Files:**
- Modify: `model/commit.go` (在文件末尾追加)

**Step 1: 在 `model/commit.go` 末尾新增 BranchInfo 和 BranchList 结构体**

```go
// BranchInfo 分支信息
type BranchInfo struct {
	Name      string `json:"name"`      // 分支名（远程分支含前缀如 origin/feat-x）
	IsRemote  bool   `json:"isRemote"`  // 是否远程分支
	IsCurrent bool   `json:"isCurrent"` // 是否当前分支
}

// BranchList 分支列表
type BranchList struct {
	Branches []BranchInfo `json:"branches"`
}
```

**Step 2: 验证编译通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go build ./...`
Expected: 编译成功，无错误

**Step 3: 提交**

```bash
git add model/commit.go
git commit -m "feat: 新增 BranchInfo/BranchList 数据模型"
```

---

### Task 2: 新增 Git 底层命令方法

**Files:**
- Modify: `util/git.go` (在 Pull 方法后追加)

**Step 1: 新增 GetBranchesAll 和 HasLocalChanges 和 CheckoutLocal 和 CheckoutRemote 方法**

在 `util/git.go` 的 `Pull` 方法后追加：

```go
// GetBranchesAll 获取所有分支（本地+远程）
func (g *GitCommand) GetBranchesAll(dir string) (string, error) {
	return g.Execute(dir, "branch", "-a")
}

// HasLocalChanges 检查是否有未提交的变更
func (g *GitCommand) HasLocalChanges(dir string) (bool, error) {
	output, err := g.Execute(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// CheckoutLocal 切换到本地分支
func (g *GitCommand) CheckoutLocal(dir, branch string) (string, error) {
	return g.Execute(dir, "checkout", branch)
}

// CheckoutRemote 从远程分支创建本地分支并跟踪
func (g *GitCommand) CheckoutRemote(dir, remoteBranch, localBranch string) (string, error) {
	return g.Execute(dir, "checkout", "-b", localBranch, remoteBranch)
}
```

**Step 2: 验证编译通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go build ./...`
Expected: 编译成功

**Step 3: 提交**

```bash
git add util/git.go
git commit -m "feat: 新增 Git 分支列表和切换命令方法"
```

---

### Task 3: 新增 Service 层方法

**Files:**
- Modify: `service/git.go`

**Step 1: 新增 GetBranches 和 CheckoutBranch 方法**

在 `service/git.go` 的 `DiscardChanges` 方法前追加：

```go
// GetBranches 获取仓库的分支列表
func (s *GitService) GetBranches(dirPath string) (*model.BranchList, error) {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return nil, fmt.Errorf("不是Git仓库")
	}

	output, err := s.gitCmd.GetBranchesAll(dirPath)
	if err != nil {
		return nil, fmt.Errorf("获取分支列表失败: %w", err)
	}

	var branches []model.BranchInfo
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		isCurrent := false
		if strings.HasPrefix(line, "* ") {
			isCurrent = true
			line = strings.TrimSpace(line[2:])
		} else {
			line = strings.TrimSpace(line[2:]) // 去掉 "  " 前缀
		}

		// 过滤 HEAD -> 引用
		if strings.Contains(line, "HEAD ->") {
			continue
		}

		if strings.HasPrefix(line, "remotes/") {
			name := strings.TrimPrefix(line, "remotes/")
			branches = append(branches, model.BranchInfo{
				Name:      name,
				IsRemote:  true,
				IsCurrent: isCurrent,
			})
		} else {
			branches = append(branches, model.BranchInfo{
				Name:      line,
				IsRemote:  false,
				IsCurrent: isCurrent,
			})
		}
	}

	return &model.BranchList{Branches: branches}, nil
}

// CheckoutBranch 切换分支
func (s *GitService) CheckoutBranch(dirPath string, branchName string, isRemote bool) error {
	if !s.gitCmd.IsGitRepository(dirPath) {
		return fmt.Errorf("不是Git仓库")
	}

	hasChanges, err := s.gitCmd.HasLocalChanges(dirPath)
	if err != nil {
		return fmt.Errorf("检查工作区状态失败: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("当前有未提交的变更，请先提交或暂存后再切换分支")
	}

	if isRemote {
		// 从远程分支名提取本地分支名（如 origin/feat-x → feat-x）
		parts := strings.SplitN(branchName, "/", 2)
		localName := branchName
		if len(parts) == 2 {
			localName = parts[1]
		}
		_, err := s.gitCmd.CheckoutRemote(dirPath, branchName, localName)
		return err
	}

	_, err = s.gitCmd.CheckoutLocal(dirPath, branchName)
	return err
}
```

**Step 2: 验证编译通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go build ./...`
Expected: 编译成功

**Step 3: 提交**

```bash
git add service/git.go
git commit -m "feat: 新增 GetBranches/CheckoutBranch 服务层方法"
```

---

### Task 4: 新增 Wails 绑定方法

**Files:**
- Modify: `app.go`

**Step 1: 在 `DiscardChanges` 方法后追加两个绑定方法**

```go
// GetBranches 获取仓库分支列表
func (a *App) GetBranches(path string) (*model.BranchList, error) {
	if path == "" {
		return nil, fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.GetBranches(path)
}

// CheckoutBranch 切换分支
func (a *App) CheckoutBranch(path string, branchName string, isRemote bool) error {
	if path == "" {
		return fmt.Errorf("路径不能为空")
	}
	return a.gitSvc.CheckoutBranch(path, branchName, isRemote)
}
```

**Step 2: 验证编译通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && go build ./...`
Expected: 编译成功

**Step 3: 运行 `wails dev` 生成前端绑定**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && wails dev`（启动后等几秒，让 wails 自动生成 `frontend/wailsjs/go/main/App.js` 和 `App.d.ts`，然后停止）

验证生成的文件中包含 `GetBranches` 和 `CheckoutBranch`：
Run: `grep -c "GetBranches\|CheckoutBranch" D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend/wailsjs/go/main/App.js`
Expected: 至少 4（每个方法出现 export + wrapper 函数）

**Step 4: 提交**

```bash
git add app.go frontend/wailsjs/
git commit -m "feat: 新增 GetBranches/CheckoutBranch Wails 绑定方法"
```

---

### Task 5: 前端 — 新增按钮和弹窗

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

这是最关键的步骤。分三个子步骤：模板、脚本、样式。

**Step 1: 修改模板 — 替换按钮区域 + 新增弹窗**

将第 10-15 行（Git 拉取更新按钮区域）替换为：

```vue
      <!-- Git 操作按钮 -->
      <div v-if="selectedNode.isGitRepo" style="margin-top: 10px;">
        <el-button type="primary" @click="pullRepo" :loading="gitLoading">
          拉取更新
        </el-button>
        <el-button @click="showBranchDialog" :loading="branchLoading">
          切换分支
        </el-button>
      </div>
```

在 `<!-- 单仓库拉取结果弹窗 -->` 之前（约第 170 行前）插入切换分支弹窗：

```vue
    <!-- 切换分支对话框 -->
    <el-dialog
      v-model="branchDialogVisible"
      title="切换分支"
      width="480px"
      append-to-body
    >
      <div style="margin-bottom: 12px; font-size: 13px; color: #909399;">
        当前分支：<span style="color: #303133; font-weight: 600;">{{ currentBranchName }}</span>
      </div>
      <el-select
        v-model="selectedBranch"
        placeholder="搜索并选择分支"
        filterable
        style="width: 100%;"
        :disabled="switchingBranch"
      >
        <el-option-group label="本地分支">
          <el-option
            v-for="b in localBranches"
            :key="b.name"
            :label="b.name"
            :value="b.name"
            :disabled="b.isCurrent"
          />
        </el-option-group>
        <el-option-group v-if="remoteBranches.length > 0" label="远程分支">
          <el-option
            v-for="b in remoteBranches"
            :key="b.name"
            :label="b.name"
            :value="b.name"
            :disabled="b.isCurrent"
          />
        </el-option-group>
      </el-select>
      <template #footer>
        <el-button @click="branchDialogVisible = false" :disabled="switchingBranch">取消</el-button>
        <el-button
          type="primary"
          @click="doCheckout"
          :loading="switchingBranch"
          :disabled="!selectedBranch || selectedBranch === currentBranchName"
        >
          切换
        </el-button>
      </template>
    </el-dialog>
```

**Step 2: 修改脚本 — 新增 import 和响应式变量和方法**

在 import 区域（约第 273-275 行）中，修改 Wails 方法导入：

```javascript
import {
  PreviewFile, PullRepo, CloneRepo, OpenWithDefaultApp,
  OpenInExplorer, OpenInVSCode, OpenInWarp,
  GetBranches, CheckoutBranch
} from '../../wailsjs/go/main/App'
```

在 `singlePullResult` 变量声明后（约第 325 行后）追加分支相关变量：

```javascript
const branchDialogVisible = ref(false)
const branchLoading = ref(false)
const switchingBranch = ref(false)
const branchList = ref([])
const selectedBranch = ref('')
const currentBranchName = ref('')
const localBranches = computed(() => branchList.value.filter(b => !b.isRemote))
const remoteBranches = computed(() => branchList.value.filter(b => b.isRemote))
```

在 import 行添加 `computed`：

```javascript
import { ref, reactive, computed, onBeforeUnmount, watch } from 'vue'
```

在 `pullRepo` 方法前追加分支相关方法：

```javascript
const showBranchDialog = async () => {
  if (!props.selectedNode) return

  branchLoading.value = true
  branchDialogVisible.value = true
  selectedBranch.value = ''

  try {
    const result = await GetBranches(props.selectedNode.path)
    branchList.value = result.branches || []
    const current = branchList.value.find(b => b.isCurrent)
    currentBranchName.value = current ? current.name : ''
  } catch (error) {
    ElMessage.error('获取分支列表失败: ' + (error.message || String(error)))
  } finally {
    branchLoading.value = false
  }
}

const doCheckout = async () => {
  if (!props.selectedNode || !selectedBranch.value) return

  const branch = branchList.value.find(b => b.name === selectedBranch.value)
  if (!branch) return

  switchingBranch.value = true
  try {
    await CheckoutBranch(props.selectedNode.path, selectedBranch.value, branch.isRemote)
    ElMessage.success('已切换到分支: ' + selectedBranch.value)
    branchDialogVisible.value = false
    gitInfoRef.value?.handleRefresh()
    commitHistoryRef.value?.handleRefresh()
  } catch (error) {
    ElMessage.error('切换分支失败: ' + (error.message || String(error)))
  } finally {
    switchingBranch.value = false
  }
}
```

**Step 3: 验证前端编译通过**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager/frontend && npx vue-tsc --noEmit 2>&1 || npm run build`
Expected: 编译成功

**Step 4: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat: 新增切换分支按钮和弹窗"
```

---

### Task 6: 端到端验证

**Step 1: 启动开发服务器**

Run: `cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/git-manager && wails dev`

**Step 2: 功能测试**

在应用中：
1. 选择一个 Git 仓库节点
2. 确认操作面板出现"拉取更新"和"切换分支"两个按钮
3. 点击"切换分支"
4. 确认弹窗显示当前分支名，下拉列表分组展示本地/远程分支
5. 搜索过滤功能正常
6. 当前分支在下拉中 disabled
7. 选择另一个本地分支 → 点击切换 → 确认成功
8. 确认 Git 信息面板和提交历史自动刷新
9. 在有未提交变更时尝试切换 → 确认报错提示

**Step 3: 最终提交（如有修复）**

```bash
git add -A
git commit -m "fix: 修复切换分支功能的细节问题"
```

---

### Task 7: 更新 README（可选）

确认是否需要更新 README.md 中的功能说明。

```bash
git add docs/功能说明.md README.md
git commit -m "docs: 更新文档，补充切换分支功能说明"
```
