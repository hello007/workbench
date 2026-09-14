# 拷贝到功能实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 新增"拷贝到"功能，支持将文件/文件夹复制到指定目标目录，提供对话框式交互。

**Architecture:** 后端新增 `CopyTo` service 方法封装校验+拷贝逻辑，app.go 暴露绑定；前端在 FileTreePanel 右键菜单和 ContentPanel 操作面板各增加入口，对话框内置于 FileTreePanel，通过事件流经 Home.vue 调用后端。

**Tech Stack:** Go 1.24 / Wails v2 / Vue 3 Composition API / Element Plus

---

### Task 1: 后端 — CopyTo 方法

**Files:**
- Modify: `service/fileoperation.go:215`（文件末尾追加）
- Test: `service/fileoperation_test.go:404`（文件末尾追加）

**Step 1: 编写 CopyTo 测试**

在 `service/fileoperation_test.go` 末尾追加：

```go
func TestCopyTo_FileToExistingDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyTo(src, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
	data, _ := os.ReadFile(expected)
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCopyTo_FileToNewDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	targetDir := filepath.Join(dir, "newdest")

	svc := NewFileOperationService()
	result, err := svc.CopyTo(src, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo failed: %v", err)
	}

	if _, err := os.Stat(targetDir); err != nil {
		t.Fatalf("Target directory not created: %v", err)
	}
	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCopyTo_DirWholeDir(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested"), 0644)
	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyTo(srcDir, targetDir, true)
	if err != nil {
		t.Fatalf("CopyTo wholeDir failed: %v", err)
	}

	expected := filepath.Join(targetDir, "srcdir")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
	data, _ := os.ReadFile(filepath.Join(expected, "sub", "nested.txt"))
	if string(data) != "nested" {
		t.Errorf("Expected 'nested', got '%s'", string(data))
	}
}

func TestCopyTo_DirContentOnly(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(srcDir, "b.txt"), []byte("b"), 0644)
	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyTo(srcDir, targetDir, false)
	if err != nil {
		t.Fatalf("CopyTo contentOnly failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "a.txt")); err != nil {
		t.Error("a.txt not found in target")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "b.txt")); err != nil {
		t.Error("b.txt not found in target")
	}
	// srcdir 本身不应被拷贝
	if _, err := os.Stat(filepath.Join(targetDir, "srcdir")); err == nil {
		t.Error("srcdir should not exist in target when copyWholeDir=false")
	}
}

func TestCopyTo_SourceNotExist(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	_, err := svc.CopyTo(filepath.Join(dir, "nonexistent"), dir, false)
	if err == nil {
		t.Fatal("Expected error for nonexistent source")
	}
}

func TestCopyTo_TargetIsFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)
	targetFile := filepath.Join(dir, "target.txt")
	os.WriteFile(targetFile, []byte("x"), 0644)

	svc := NewFileOperationService()
	_, err := svc.CopyTo(src, targetFile, false)
	if err == nil {
		t.Fatal("Expected error when target is a file")
	}
}
```

**Step 2: 运行测试确认失败**

Run: `cd .worktrees/feature-copy-to && go test ./service/ -run TestCopyTo -v`
Expected: 编译失败（`CopyTo` 方法未定义）

**Step 3: 实现 CopyTo 方法**

在 `service/fileoperation.go` 末尾追加：

```go
// CopyTo 将文件或文件夹拷贝到指定目标目录
func (s *FileOperationService) CopyTo(sourcePath, targetPath string, copyWholeDir bool) (string, error) {
	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return "", fmt.Errorf("原地址不存在: %s", sourcePath)
	}

	targetInfo, err := os.Stat(targetPath)
	if err == nil && !targetInfo.IsDir() {
		return "", fmt.Errorf("目标地址不是文件夹: %s", targetPath)
	}
	if err != nil {
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return "", fmt.Errorf("创建目标目录失败: %w", err)
		}
	}

	if !sourceInfo.IsDir() || copyWholeDir {
		return s.CopyItem(sourcePath, targetPath)
	}

	// copyWholeDir=false + 源是文件夹：拷贝目录内容到目标下
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		return "", err
	}

	var lastResult string
	for _, entry := range entries {
		entryPath := filepath.Join(sourcePath, entry.Name())
		result, err := s.CopyItem(entryPath, targetPath)
		if err != nil {
			return "", fmt.Errorf("拷贝 %s 失败: %w", entry.Name(), err)
		}
		lastResult = result
	}

	return lastResult, nil
}
```

**Step 4: 运行测试确认通过**

Run: `cd .worktrees/feature-copy-to && go test ./service/ -run TestCopyTo -v`
Expected: 全部 PASS

**Step 5: 提交**

```bash
cd .worktrees/feature-copy-to
git add service/fileoperation.go service/fileoperation_test.go
git commit -m "feat: 新增 CopyTo 方法（含校验和目录内容拷贝）"
```

---

### Task 2: 后端 — app.go 绑定

**Files:**
- Modify: `app.go:469`（在 `MoveItem` 方法后追加）

**Step 1: 添加 CopyTo 绑定方法**

在 `app.go` 的 `MoveItem` 方法后（第 469 行后）追加：

```go
// CopyTo 将文件或文件夹拷贝到指定目标目录
func (a *App) CopyTo(sourcePath, targetPath string, copyWholeDir bool) string {
	result, err := a.fileOpSvc.CopyTo(sourcePath, targetPath, copyWholeDir)
	if err != nil {
		println("Error:", err.Error())
		return "错误: " + err.Error()
	}
	return result
}
```

**Step 2: 运行后端测试确认无回归**

Run: `cd .worktrees/feature-copy-to && go test ./service/... ./model/... -v`
Expected: 全部 PASS

**Step 3: 提交**

```bash
cd .worktrees/feature-copy-to
git add app.go
git commit -m "feat: app.go 新增 CopyTo 绑定方法"
```

---

### Task 3: 前端 — setup.js 添加 CopyTo mock

**Files:**
- Modify: `frontend/src/test/setup.js:8-19`

**Step 1: 在 mock 对象中添加 CopyTo**

在 `frontend/src/test/setup.js` 的 `vi.mock('../../wailsjs/go/main/App', ...)` 对象中追加：

```js
CopyTo: vi.fn(() => Promise.resolve('')),
```

**Step 2: 提交**

```bash
cd .worktrees/feature-copy-to
git add frontend/src/test/setup.js
git commit -m "test: setup.js 新增 CopyTo mock"
```

---

### Task 4: 前端 — FileTreePanel 右键菜单 + 对话框

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

**Step 1: 在 `defineEmits` 中添加 `copyTo` 事件**

第 234 行，将：
```js
const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste'])
```
改为：
```js
const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste', 'copyTo'])
```

**Step 2: 在右键菜单中添加"拷贝到..."菜单项**

在目录菜单（第 131 行 `</li>` 之后，第 132 行 `<li class="context-menu-divider" />` 之前）插入：
```html
<li class="context-menu-item" @click="onMenuCommand('copyTo')">
  <el-icon><FolderAdd /></el-icon>拷贝到...
</li>
```

在文件菜单（第 169 行 `</li>` 之后，第 170 行 `<li class="context-menu-divider" />` 之前）插入同样内容。

**Step 3: 在 `onMenuCommand` switch 中添加 `copyTo` 分支**

在 `case 'paste'` 分支后追加：
```js
case 'copyTo':
  emit('copyTo', data)
  break
```

**Step 4: 添加"拷贝到"对话框状态变量**

在重命名对话框状态后（约第 268 行后）追加：
```js
// ---- 拷贝到对话框状态 ----
const copyToDialogVisible = ref(false)
const copyToSourcePath = ref('')
const copyToTargetPath = ref('')
const copyToWholeDir = ref(true)
const copyToLoading = ref(false)
```

**Step 5: 添加 `showCopyToDialog` 方法**

在 `copyToClipboard` 方法前追加：
```js
// ---- 拷贝到对话框 ----
const showCopyToDialog = (data) => {
  copyToSourcePath.value = data.path.replaceAll('\\', '/')
  copyToTargetPath.value = ''
  copyToWholeDir.value = data.type === 'directory'
  copyToLoading.value = false
  copyToDialogVisible.value = true
}

const handleCopyTo = () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }

  emit('copyTo', {
    sourcePath: copyToSourcePath.value,
    targetPath: copyToTargetPath.value,
    copyWholeDir: copyToWholeDir.value
  })
  copyToDialogVisible.value = false
}
```

**Step 6: 在 template 中添加对话框 HTML**

在重命名对话框 `</el-dialog>` 后（约第 99 行后）追加：
```html
<!-- 拷贝到对话框 -->
<el-dialog
  v-model="copyToDialogVisible"
  title="拷贝到"
  width="480px"
>
  <el-form label-width="100px">
    <el-form-item label="原地址">
      <el-input
        v-model="copyToSourcePath"
        placeholder="请输入原文件或文件夹路径"
        :disabled="copyToLoading"
      />
    </el-form-item>
    <el-form-item label="目标地址">
      <el-input
        v-model="copyToTargetPath"
        placeholder="请输入目标文件夹路径"
        :disabled="copyToLoading"
        @keyup.enter="handleCopyTo"
      />
    </el-form-item>
    <el-form-item>
      <el-checkbox
        v-model="copyToWholeDir"
        :disabled="copyToLoading"
      >
        对原地址目录整体操作
      </el-checkbox>
    </el-form-item>
  </el-form>
  <template #footer>
    <el-button @click="copyToDialogVisible = false" :disabled="copyToLoading">取消</el-button>
    <el-button type="primary" @click="handleCopyTo" :loading="copyToLoading">确定</el-button>
  </template>
</el-dialog>
```

**Step 7: 更新 `onMenuCommand` 中的 `copyTo` 分支**

将 Step 3 中的 `copyTo` 分支从 `emit('copyTo', data)` 改为调用对话框方法：
```js
case 'copyTo':
  showCopyToDialog(data)
  break
```

**Step 8: 提交**

```bash
cd .worktrees/feature-copy-to
git add frontend/src/components/FileTreePanel.vue
git commit -m "feat: FileTreePanel 新增拷贝到右键菜单和对话框"
```

---

### Task 5: 前端 — ContentPanel 按钮和事件

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

**Step 1: 在 `defineEmits` 中添加 `copyTo`**

第 181-191 行，在 `'paste'` 后追加 `'copyTo'`：
```js
const emit = defineEmits([
  'latestCommit',
  'refreshNode',
  'createDirectory',
  'createFile',
  'rename',
  'delete',
  'copy',
  'cut',
  'paste',
  'copyTo'
])
```

**Step 2: 在文件夹操作按钮组中添加"拷贝到"按钮**

第 40-44 行，在粘贴按钮后追加：
```html
<el-button-group>
  <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
  <el-button @click="$emit('copy', selectedNode)">复制</el-button>
  <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
  <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
</el-button-group>
```

**Step 3: 在文件操作按钮组中添加"拷贝到"按钮**

第 56-60 行，同样在粘贴按钮后追加：
```html
<el-button-group>
  <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
  <el-button @click="$emit('copy', selectedNode)">复制</el-button>
  <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
  <el-button @click="$emit('copyTo', selectedNode)">拷贝到</el-button>
</el-button-group>
```

**Step 4: 提交**

```bash
cd .worktrees/feature-copy-to
git add frontend/src/components/ContentPanel.vue
git commit -m "feat: ContentPanel 新增拷贝到按钮"
```

---

### Task 6: 前端 — Home.vue 事件处理

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 导入 CopyTo**

第 53-62 行的 import 中追加 `CopyTo`：
```js
import {
  GetDirectories,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  MoveItem,
  CopyTo,
  CopyToSystemClipboard,
  CutToSystemClipboard,
  ReadFromSystemClipboard
} from '../../wailsjs/go/main/App'
```

**Step 2: 添加 `handleCopyTo` 方法**

在 `handlePaste` 方法后（约第 262 行后）追加：
```js
const handleCopyTo = async (data) => {
  copyToLoading.value = true
  try {
    const result = await CopyTo(data.sourcePath, data.targetPath, data.copyWholeDir)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      ElMessage.success('拷贝成功')
      fileTreePanelRef.value?.refreshNode(data.targetPath)
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    copyToLoading.value = false
  }
}
```

注意：`copyToLoading` 需要在 FileTreePanel 中暴露给 Home.vue，或者采用另一种方式——将对话框提交时的逻辑改为直接 emit 参数，由 Home.vue 控制加载状态。

**更好的方案**：对话框只负责收集参数并 emit，Home.vue 负责调用后端和加载状态。因此需要微调 FileTreePanel：

在 FileTreePanel 中：
- `handleCopyTo` 只 emit 参数，不关闭对话框
- 暴露 `setCopyToLoading` 方法

在 Home.vue 中：
- `handleCopyTo` 调用后端，成功后关闭对话框并刷新

**修正 Step 2（替换上面的）**：

Home.vue 追加：
```js
const handleCopyTo = async (data) => {
  fileTreePanelRef.value?.setCopyToLoading(true)
  try {
    const result = await CopyTo(data.sourcePath, data.targetPath, data.copyWholeDir)
    if (result && result.startsWith('错误')) {
      ElMessage.error(result)
    } else {
      ElMessage.success('拷贝成功')
      fileTreePanelRef.value?.closeCopyToDialog()
      fileTreePanelRef.value?.refreshNode(data.targetPath)
    }
  } catch (error) {
    ElMessage.error('拷贝失败: ' + (error.message || String(error)))
  } finally {
    fileTreePanelRef.value?.setCopyToLoading(false)
  }
}
```

**Step 3: 在 FileTreePanel 上绑定 `@copy-to` 事件**

第 13-24 行的 `<FileTreePanel>` 标签中追加：
```html
@copy-to="handleCopyTo"
```

**Step 4: 在 ContentPanel 上绑定 `@copy-to` 事件**

第 25-40 行的 `<ContentPanel>` 标签中追加：
```html
@copy-to="handleCopyTo"
```

**Step 5: 更新 FileTreePanel 暴露方法**

在 `defineExpose` 中追加 `setCopyToLoading` 和 `closeCopyToDialog`：
```js
defineExpose({
  refreshNode,
  expandAll,
  collapseAll,
  showRenameAt,
  showCreateAt,
  setCopyToLoading: (val) => { copyToLoading.value = val },
  closeCopyToDialog: () => { copyToDialogVisible.value = false }
})
```

**Step 6: 更新 FileTreePanel 的 `handleCopyTo` 方法**

将 Task 4 中的 `handleCopyTo` 改为只 emit 参数（不关闭对话框，不设 loading）：
```js
const handleCopyTo = () => {
  if (!copyToSourcePath.value.trim()) {
    ElMessage.warning('请输入原地址')
    return
  }
  if (!copyToTargetPath.value.trim()) {
    ElMessage.warning('请输入目标地址')
    return
  }

  emit('copyTo', {
    sourcePath: copyToSourcePath.value,
    targetPath: copyToTargetPath.value,
    copyWholeDir: copyToWholeDir.value
  })
}
```

**Step 7: 提交**

```bash
cd .worktrees/feature-copy-to
git add frontend/src/views/Home.vue frontend/src/components/FileTreePanel.vue
git commit -m "feat: Home.vue 新增 handleCopyTo 事件处理"
```

---

### Task 7: 集成验证

**Step 1: 运行后端测试**

Run: `cd .worktrees/feature-copy-to && go test ./service/... ./model/... -v`
Expected: 全部 PASS

**Step 2: 检查 Wails 绑定生成**

Run: `cd .worktrees/feature-copy-to && wails dev`
Expected: 应用正常启动，`frontend/wailsjs/go/main/App.js` 中自动生成 `CopyTo` 绑定

**Step 3: 手动验证功能**

在应用中测试以下场景：
1. 右键文件 → 拷贝到 → 填入目标 → 确定 → 文件被复制
2. 右键文件夹 → 拷贝到 → 勾选整体操作 → 确定 → 整个文件夹被复制
3. 右键文件夹 → 拷贝到 → 取消勾选整体操作 → 确定 → 只有内容被复制
4. 原地址不存在 → 提示错误
5. 目标地址是文件 → 提示错误
6. 目标地址不存在 → 自动创建目录
7. 操作面板按钮同样触发拷贝到对话框

**Step 4: 最终提交**

```bash
cd .worktrees/feature-copy-to
git add -A
git commit -m "feat: 拷贝到功能完整实现"
```
