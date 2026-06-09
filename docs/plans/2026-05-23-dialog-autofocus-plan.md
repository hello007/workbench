# 弹窗自动聚焦实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 为 5 个缺少自动聚焦的弹窗（新建文件/文件夹、拷贝到、添加工作目录、克隆仓库、切换分支）添加自动聚焦功能。

**Architecture:** 参照现有重命名弹窗的实现模式，在 `showXxxDialog` 函数中通过 ref + nextTick/setTimeout 调用 focus()。不引入 watcher，保持与现有代码风格一致。

**Tech Stack:** Vue 3 Composition API + Element Plus

---

### Task 1: FileTreePanel.vue — 新建文件/文件夹弹窗自动聚焦

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:64-70`（模板 el-input）
- Modify: `frontend/src/components/FileTreePanel.vue:300-326`（脚本 ref 声明 + showCreateAt 函数）

**Step 1: 给新建弹窗的 el-input 添加 ref**

在 `frontend/src/components/FileTreePanel.vue` 第 65 行，给文件名输入框加 `ref="createInputRef"`：

```vue
<el-input
  ref="createInputRef"
  v-model="createName"
  :placeholder="createType === 'directory' ? '例如: src' : '例如: main.go'"
  :disabled="createLoading"
  @keyup.enter="handleCreate"
/>
```

**Step 2: 在脚本中声明 ref 变量**

在第 332 行（`renameInputRef` 声明之前）添加：

```js
const createInputRef = ref()
```

**Step 3: 在 showCreateAt 函数中添加聚焦逻辑**

修改第 628-633 行的 `showCreateAt` 函数：

```js
const showCreateAt = (data, type) => {
  createParentData.value = data
  createType.value = type
  createName.value = ''
  createDialogVisible.value = true
  nextTick(() => {
    const input = createInputRef.value?.input
    if (input) {
      input.focus()
    }
  })
}
```

**Step 4: 验证**

运行: `cd frontend && npm run dev`
验证：在文件树中右键点击文件夹 → 新建文件/新建文件夹 → 弹窗打开后光标应在输入框中。

**Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat: 新建文件/文件夹弹窗自动聚焦输入框"
```

---

### Task 2: FileTreePanel.vue — 拷贝到弹窗自动聚焦

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue:123-128`（模板目标地址 el-input）
- Modify: `frontend/src/components/FileTreePanel.vue`（脚本 ref 声明 + showCopyToDialog 函数）

**Step 1: 给拷贝到弹窗的目标地址 el-input 添加 ref**

在第 123 行的目标地址输入框加 `ref="copyToTargetInputRef"`：

```vue
<el-input
  ref="copyToTargetInputRef"
  v-model="copyToTargetPath"
  placeholder="请输入目标文件夹路径"
  :disabled="copyToLoading"
  @keyup.enter="handleCopyTo"
/>
```

**Step 2: 在脚本中声明 ref 变量**

在 `copyToPreview` computed 附近添加：

```js
const copyToTargetInputRef = ref()
```

**Step 3: 在 showCopyToDialog 函数中添加聚焦逻辑**

修改第 742-748 行：

```js
const showCopyToDialog = (data) => {
  copyToSourcePath.value = data.path.replaceAll('\\', '/')
  copyToTargetPath.value = ''
  copyToWholeDir.value = data.type === 'directory'
  copyToLoading.value = false
  copyToDialogVisible.value = true
  nextTick(() => {
    const input = copyToTargetInputRef.value?.input
    if (input) {
      input.focus()
    }
  })
}
```

**Step 4: 验证**

在文件树中右键 → 拷贝到 → 弹窗打开后光标应在目标地址输入框中（原地址已有预填值）。

**Step 5: 提交**

```bash
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat: 拷贝到弹窗自动聚焦目标地址输入框"
```

---

### Task 3: DirectoryTree.vue — 添加工作目录弹窗自动聚焦

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue:74`（模板目录名称 el-input）
- Modify: `frontend/src/components/DirectoryTree.vue:250-272`（脚本 ref 声明 + showAddDialog 函数）

**Step 1: 给添加目录弹窗的目录路径 el-input 添加 ref**

在第 77 行的目录路径输入框加 `ref="addPathInputRef"`（聚焦路径输入框，因为输入路径后会自动填充名称）：

```vue
<el-form-item label="目录路径">
  <el-input ref="addPathInputRef" v-model="addForm.path" placeholder="例如: C:\workspace" />
</el-form-item>
```

**Step 2: 在脚本中声明 ref 变量**

在第 253 行附近添加：

```js
const addPathInputRef = ref()
```

**Step 3: 在 showAddDialog 函数中添加聚焦逻辑**

修改第 268-272 行：

```js
const showAddDialog = () => {
  addForm.value = { name: '', path: '', isDefault: false }
  addNameManuallySet.value = false
  addDialogVisible.value = true
  nextTick(() => {
    const input = addPathInputRef.value?.input
    if (input) {
      input.focus()
    }
  })
}
```

**Step 4: 验证**

点击工作目录面板的 + 按钮 → 弹窗打开后光标应在目录路径输入框中（输入路径后名称会自动填充）。

**Step 5: 提交**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: 添加工作目录弹窗自动聚焦路径输入框"
```

---

### Task 4: ContentPanel.vue — 克隆仓库弹窗自动聚焦

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue:208-213`（模板 Git 地址 el-input）
- Modify: `frontend/src/components/ContentPanel.vue:366-368`（脚本 ref 声明）
- Modify: `frontend/src/components/ContentPanel.vue:553-556`（showCloneDialog 函数）

**Step 1: 给克隆仓库弹窗的 Git 地址 el-input 添加 ref**

在第 208 行加 `ref="cloneInputRef"`：

```vue
<el-input
  ref="cloneInputRef"
  v-model="cloneUrl"
  placeholder="例如: https://github.com/user/repo.git"
  :disabled="cloneLoading"
  @keyup.enter="cloneRepo"
/>
```

**Step 2: 在脚本中声明 ref 变量**

在第 368 行（`cloneLoading` 附近）添加：

```js
const cloneInputRef = ref()
```

**Step 3: 在 showCloneDialog 函数中添加聚焦逻辑**

需要在 script 中引入 `nextTick`。检查第 317 行的 import：

```js
import { ref, reactive, computed, onBeforeUnmount, watch, nextTick } from 'vue'
```

修改第 553-556 行：

```js
const showCloneDialog = () => {
  cloneUrl.value = ''
  cloneDialogVisible.value = true
  nextTick(() => {
    const input = cloneInputRef.value?.input
    if (input) {
      input.focus()
    }
  })
}
```

**Step 4: 验证**

选中一个文件夹 → 克隆仓库 → 弹窗打开后光标应在 Git 地址输入框中。

**Step 5: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat: 克隆仓库弹窗自动聚焦 Git 地址输入框"
```

---

### Task 5: ContentPanel.vue — 切换分支弹窗自动聚焦

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue:157-182`（模板 el-select）
- Modify: `frontend/src/components/ContentPanel.vue:380-387`（脚本 ref 声明）
- Modify: `frontend/src/components/ContentPanel.vue:395-412`（showBranchDialog 函数）

**Step 1: 给切换分支弹窗的 el-select 添加 ref**

在第 157 行加 `ref="branchSelectRef"`：

```vue
<el-select
  ref="branchSelectRef"
  v-model="selectedBranch"
  placeholder="搜索并选择分支"
  filterable
  style="width: 100%;"
  :disabled="switchingBranch"
>
```

**Step 2: 在脚本中声明 ref 变量**

在第 387 行附近添加：

```js
const branchSelectRef = ref()
```

**Step 3: 在 showBranchDialog 函数中添加聚焦逻辑**

修改第 395-412 行，在数据加载完成后聚焦：

```js
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
    nextTick(() => {
      branchSelectRef.value?.focus()
    })
  } catch (error) {
    ElMessage.error('获取分支列表失败: ' + (error.message || String(error)))
  } finally {
    branchLoading.value = false
  }
}
```

**Step 4: 验证**

选中一个 Git 仓库 → 切换分支 → 弹窗打开并加载完分支列表后，下拉选择器应自动聚焦（可直接输入搜索）。

**Step 5: 提交**

```bash
git add frontend/src/components/ContentPanel.vue
git commit -m "feat: 切换分支弹窗自动聚焦分支选择器"
```

---

### Task 6: 全量验证

**Step 1: 启动开发服务器**

```bash
cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench && wails dev
```

**Step 2: 逐个验证所有 7 个弹窗**

| 弹窗 | 操作方式 | 预期行为 |
|------|----------|----------|
| 新建文件 | 右键文件夹 → 新建文件 | 光标在文件名输入框 |
| 新建文件夹 | 右键文件夹 → 新建文件夹 | 光标在文件夹名输入框 |
| 拷贝到 | 右键 → 拷贝到... | 光标在目标地址输入框 |
| 添加工作目录 | 点击 + 按钮 | 光标在目录路径输入框 |
| 克隆仓库 | 点击克隆仓库按钮 | 光标在 Git 地址输入框 |
| 切换分支 | 点击切换分支按钮 | 分支下拉框聚焦可搜索 |
| 重命名文件 | 右键 → 重命名 | 光标在新名称输入框（已有功能，回归验证） |
| 重命名目录 | 右键 → 重命名 | 光标在新名称输入框（已有功能，回归验证） |

**Step 3: 确认是否需要更新 README.md**
