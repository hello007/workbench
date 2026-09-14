# 文件复制/剪切/粘贴功能 实现计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在文件树右键菜单和操作面板中新增剪切、复制、粘贴操作，支持文件和文件夹的复制与移动。

**Architecture:** 后端在 `service/fileoperation.go` 新增 `CopyItem`/`MoveItem` 方法，通过 `util/file.go` 的递归复制辅助函数处理文件系统操作；前端在 `Home.vue` 维护剪贴板响应式状态，`FileTreePanel` 和 `ContentPanel` 通过事件委托操作。

**Tech Stack:** Go (后端), Vue 3 + Element Plus (前端), Wails v2 (绑定)

---

### Task 1: 编写后端单元测试

**Files:**
- Modify: `service/fileoperation_test.go`（末尾追加）

**Step 1: 添加测试函数**

在 `service/fileoperation_test.go` 文件末尾追加以下测试：

```go
func TestFindAvailableName_NoConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	result := findAvailableName(target)
	if result != target {
		t.Errorf("Expected %s, got %s", target, result)
	}
}

func TestFindAvailableName_FileConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	os.WriteFile(target, []byte("x"), 0644)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFindAvailableName_MultipleConflicts(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")
	os.WriteFile(target, []byte("x"), 0644)
	os.WriteFile(filepath.Join(dir, "test(1).txt"), []byte("x"), 0644)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "test(2).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestFindAvailableName_DirectoryConflict(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "folder")
	os.MkdirAll(target, 0755)

	result := findAvailableName(target)
	expected := filepath.Join(dir, "folder(1)")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCopyItem_File(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("hello"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(src, targetDir)
	if err != nil {
		t.Fatalf("CopyItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test.txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, err := os.ReadFile(expected)
	if err != nil {
		t.Fatalf("Copied file not found: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(data))
	}
}

func TestCopyItem_FileConflictAutoRename(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("original"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "test.txt"), []byte("existing"), 0644)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(src, targetDir)
	if err != nil {
		t.Fatalf("CopyItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}

	data, _ := os.ReadFile(expected)
	if string(data) != "original" {
		t.Errorf("Expected 'original', got '%s'", string(data))
	}
}

func TestCopyItem_Directory(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.CopyItem(srcDir, targetDir)
	if err != nil {
		t.Fatalf("CopyItem directory failed: %v", err)
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

func TestCopyItem_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	_, err := svc.CopyItem(filepath.Join(dir, "nonexistent"), dir)
	if err == nil {
		t.Fatal("Expected error for nonexistent source")
	}
}

func TestMoveItem_File(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	src := filepath.Join(srcDir, "test.txt")
	os.WriteFile(src, []byte("move me"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)

	svc := NewFileOperationService()
	result, err := svc.MoveItem(src, targetDir)
	if err != nil {
		t.Fatalf("MoveItem failed: %v", err)
	}

	if _, err := os.Stat(src); err == nil {
		t.Error("Source file still exists after move")
	}

	data, _ := os.ReadFile(result)
	if string(data) != "move me" {
		t.Errorf("Expected 'move me', got '%s'", string(data))
	}
}

func TestMoveItem_SameDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "test.txt")
	os.WriteFile(src, []byte("x"), 0644)

	svc := NewFileOperationService()
	_, err := svc.MoveItem(src, dir)
	if err == nil {
		t.Fatal("Expected error when source and target are same directory")
	}
}

func TestMoveItem_FileConflictAutoRename(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "src")
	os.MkdirAll(srcDir, 0755)
	os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("moving"), 0644)

	targetDir := filepath.Join(dir, "dest")
	os.MkdirAll(targetDir, 0755)
	os.WriteFile(filepath.Join(targetDir, "test.txt"), []byte("existing"), 0644)

	svc := NewFileOperationService()
	result, err := svc.MoveItem(filepath.Join(srcDir, "test.txt"), targetDir)
	if err != nil {
		t.Fatalf("MoveItem failed: %v", err)
	}

	expected := filepath.Join(targetDir, "test(1).txt")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
```

**Step 2: 运行测试确认失败**

Run: `cd workbench && go test ./service/ -run "TestFindAvailableName|TestCopyItem|TestMoveItem" -v`
Expected: 编译失败（函数未定义）

---

### Task 2: 实现 util 层递归复制辅助函数

**Files:**
- Modify: `util/file.go`（末尾追加，import 区添加 `"io"`）

**Step 1: 添加 `io` 到 import 块**

将 `util/file.go` 的 import 块改为：

```go
import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)
```

**Step 2: 在文件末尾追加 CopyFile 和 CopyDir**

```go
// CopyFile 复制单个文件（保留权限）
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// CopyDir 递归复制目录
func CopyDir(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}
```

---

### Task 3: 实现 service 层方法

**Files:**
- Modify: `service/fileoperation.go`（import 区添加 `"fmt"`, `"strings"`，末尾追加方法）

**Step 1: 确认 import 包含**

确保 `service/fileoperation.go` 的 import 包含 `"fmt"` 和 `"strings"`：

```go
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"workbench/model"
	"workbench/util"
)
```

**Step 2: 在文件末尾追加 findAvailableName、CopyItem、MoveItem**

```go
// findAvailableName 查找可用路径，冲突时自动追加 (1), (2)...
func findAvailableName(targetPath string) string {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return targetPath
	}

	ext := filepath.Ext(targetPath)
	nameWithoutExt := strings.TrimSuffix(filepath.Base(targetPath), ext)
	dir := filepath.Dir(targetPath)

	for i := 1; i < 1000; i++ {
		newName := fmt.Sprintf("%s(%d)%s", nameWithoutExt, i, ext)
		newPath := filepath.Join(dir, newName)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}

	return targetPath
}

// CopyItem 复制文件或目录到目标文件夹，同名自动重命名
func (s *FileOperationService) CopyItem(sourcePath, targetDir string) (string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	targetPath = findAvailableName(targetPath)

	if info.IsDir() {
		return targetPath, util.CopyDir(sourcePath, targetPath)
	}
	return targetPath, util.CopyFile(sourcePath, targetPath)
}

// MoveItem 移动文件或目录到目标文件夹，同名自动重命名
func (s *FileOperationService) MoveItem(sourcePath, targetDir string) (string, error) {
	sourceDir := filepath.Dir(sourcePath)
	if sourceDir == targetDir {
		return "", fmt.Errorf("源路径与目标路径相同")
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(targetDir, filepath.Base(sourcePath))
	targetPath = findAvailableName(targetPath)

	err = os.Rename(sourcePath, targetPath)
	if err == nil {
		return targetPath, nil
	}

	// 跨盘移动降级为复制+删除
	if info.IsDir() {
		err = util.CopyDir(sourcePath, targetPath)
	} else {
		err = util.CopyFile(sourcePath, targetPath)
	}
	if err != nil {
		return "", err
	}
	return targetPath, os.RemoveAll(sourcePath)
}
```

---

### Task 4: 运行后端测试 + 添加 Wails 绑定

**Step 1: 运行测试**

Run: `cd workbench && go test ./service/ -run "TestFindAvailableName|TestCopyItem|TestMoveItem" -v`
Expected: 全部 PASS

**Step 2: 运行完整后端测试套件**

Run: `cd workbench && go test ./... -v`
Expected: 全部 PASS，无回归

**Step 3: 在 app.go 末尾追加 Wails 绑定方法**

在 `app.go` 末尾追加：

```go
// CopyItem 复制文件或文件夹
func (a *App) CopyItem(sourcePath, targetDir string) string {
	result, err := a.fileOpSvc.CopyItem(sourcePath, targetDir)
	if err != nil {
		return "错误: " + err.Error()
	}
	return result
}

// MoveItem 移动文件或文件夹
func (a *App) MoveItem(sourcePath, targetDir string) string {
	result, err := a.fileOpSvc.MoveItem(sourcePath, targetDir)
	if err != nil {
		return "错误: " + err.Error()
	}
	return result
}
```

**Step 4: 确认编译通过**

Run: `cd workbench && go build ./...`
Expected: 无错误

**Step 5: 提交后端变更**

```bash
git add util/file.go service/fileoperation.go service/fileoperation_test.go app.go
git commit -m "feat: 添加 CopyItem/MoveItem 后端接口，支持文件复制和移动"
```

---

### Task 5: FileTreePanel 右键菜单

**Files:**
- Modify: `frontend/src/components/FileTreePanel.vue`

**Step 1: 添加图标导入**

在 icon import 块中添加 `Scissor` 和 `DocumentCopy`：

```js
import {
  Folder, FolderOpened, Document, SuccessFilled,
  FolderAdd, DocumentAdd, Edit, Delete, CopyDocument,
  Monitor, Refresh, EditPen, Open, Promotion,
  Scissor, DocumentCopy
} from '@element-plus/icons-vue'
```

**Step 2: 添加 clipboard prop**

在 `defineProps` 中增加 `clipboard` 属性：

```js
const props = defineProps({
  directories: { type: Array, default: () => [] },
  selectedDirId: { type: String, default: '' },
  clipboard: { type: Object, default: () => ({ mode: null }) }
})
```

**Step 3: 修改 emits**

```js
const emit = defineEmits(['select', 'batchPull', 'copy', 'cut', 'paste'])
```

**Step 4: 在 onMenuCommand switch 中添加三个 case**

在 `case 'delete'` 分支之后，`case 'copyPath'` 之前添加：

```js
    case 'cut':
      emit('cut', data)
      break
    case 'copy':
      emit('copy', data)
      break
    case 'paste':
      emit('paste', data)
      break
```

**Step 5: 修改文件夹右键菜单模板**

在文件夹右键菜单的 `删除` 菜单项（`@click="onMenuCommand('delete')"`）之后、第一个 `<li class="context-menu-divider" />` 之后（即"复制路径"之前），插入：

```html
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
        </li>
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
```

具体位置：在 `<li class="context-menu-item" @click="onMenuCommand('delete')">...</li>` 之后、`<li class="context-menu-divider" />` （copyPath 之前的分隔线）之前。

**Step 6: 修改文件右键菜单模板**

在文件右键菜单的 `删除` 菜单项之后、分隔线之前，插入同样的三个菜单项：

```html
        <li class="context-menu-divider" />
        <li class="context-menu-item" @click="onMenuCommand('cut')">
          <el-icon><Scissor /></el-icon>剪切
        </li>
        <li class="context-menu-item" @click="onMenuCommand('copy')">
          <el-icon><CopyDocument /></el-icon>复制
        </li>
        <li class="context-menu-item" :class="{ 'is-disabled': !clipboard.mode }" @click="clipboard.mode && onMenuCommand('paste')">
          <el-icon><DocumentCopy /></el-icon>粘贴
        </li>
```

**Step 7: 添加禁用样式**

在 `<style scoped>` 中追加：

```css
.context-menu-item.is-disabled {
  color: #c0c4cc;
  cursor: not-allowed;
}
.context-menu-item.is-disabled:hover {
  background-color: transparent;
  color: #c0c4cc;
}
```

---

### Task 6: ContentPanel 操作按钮

**Files:**
- Modify: `frontend/src/components/ContentPanel.vue`

**Step 1: 添加 clipboard prop 和 emits**

修改 `defineProps`，增加 `clipboard`：

```js
const props = defineProps({
  selectedNode: { type: Object, default: null },
  latestCommit: { type: Object, default: null },
  clipboard: { type: Object, default: () => ({ mode: null }) }
})
```

修改 `defineEmits`，增加 copy/cut/paste：

```js
const emit = defineEmits([
  'latestCommit', 'refreshNode', 'createDirectory', 'createFile',
  'rename', 'delete', 'copy', 'cut', 'paste'
])
```

**Step 2: 修改文件夹操作按钮区域**

将文件夹操作区域（约第 38-44 行）的 `<el-button-group>` 改为：

```html
<el-button-group>
  <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
  <el-button @click="$emit('copy', selectedNode)">复制</el-button>
  <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
</el-button-group>
<el-button-group style="margin-top: 10px;">
  <el-button @click="$emit('createDirectory', selectedNode)">新建文件夹</el-button>
  <el-button @click="$emit('createFile', selectedNode)">新建文件</el-button>
  <el-button type="success" @click="showCloneDialog">克隆仓库</el-button>
</el-button-group>
```

**Step 3: 修改文件操作按钮区域**

将文件操作区域（约第 46-53 行）的 `<el-button-group>` 改为：

```html
<el-button-group>
  <el-button @click="$emit('cut', selectedNode)">剪切</el-button>
  <el-button @click="$emit('copy', selectedNode)">复制</el-button>
  <el-button :disabled="!clipboard.mode" @click="$emit('paste', selectedNode)">粘贴</el-button>
</el-button-group>
<el-button-group style="margin-top: 10px;">
  <el-button type="primary" @click="handleOpenWithDefaultApp">打开</el-button>
  <el-button @click="previewFile">预览</el-button>
  <el-button @click="$emit('rename', selectedNode)">重命名</el-button>
  <el-button type="danger" @click="$emit('delete', selectedNode)">删除</el-button>
</el-button-group>
```

---

### Task 7: Home.vue 状态协调

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 导入 CopyItem 和 MoveItem**

在 import 块中添加：

```js
import {
  GetDirectories,
  ScanAndPullRepos,
  DeleteFile,
  CopyItem,
  MoveItem
} from '../../wailsjs/go/main/App'
```

**Step 2: 添加剪贴板状态**

在 `const latestCommit = ref(null)` 之后添加：

```js
const clipboard = reactive({
  mode: null,       // 'copy' | 'cut' | null
  sourcePath: '',
  sourceName: '',
  sourceType: ''
})
```

在 import 中添加 `reactive`：

```js
import { ref, reactive, onMounted, watch } from 'vue'
```

**Step 3: 添加剪贴板操作方法**

在 `onDeleteFromContent` 函数之后添加：

```js
const clearClipboard = () => {
  clipboard.mode = null
  clipboard.sourcePath = ''
  clipboard.sourceName = ''
  clipboard.sourceType = ''
}

const handleCopy = (data) => {
  clipboard.mode = 'copy'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 复制成功`)
}

const handleCut = (data) => {
  clipboard.mode = 'cut'
  clipboard.sourcePath = data.path
  clipboard.sourceName = data.name
  clipboard.sourceType = data.type
  ElMessage.success(`${data.path.replaceAll('\\', '/')} 剪切成功`)
}

const resolveTargetDir = (data) => {
  if (data.type === 'directory') {
    return data.path
  }
  const lastSep = Math.max(data.path.lastIndexOf('\\'), data.path.lastIndexOf('/'))
  return lastSep > 0 ? data.path.substring(0, lastSep) : ''
}

const handlePaste = async (targetData) => {
  if (!clipboard.mode || !clipboard.sourcePath) return

  const targetDir = resolveTargetDir(targetData)
  if (!targetDir) return

  try {
    let result
    if (clipboard.mode === 'copy') {
      result = await CopyItem(clipboard.sourcePath, targetDir)
    } else {
      result = await MoveItem(clipboard.sourcePath, targetDir)
    }

    if (result && !result.startsWith('错误')) {
      ElMessage.success(`粘贴成功：${result.replaceAll('\\', '/')}`)
      fileTreePanelRef.value?.refreshNode(targetDir)

      if (clipboard.mode === 'cut') {
        const srcLastSep = Math.max(clipboard.sourcePath.lastIndexOf('\\'), clipboard.sourcePath.lastIndexOf('/'))
        const srcParent = srcLastSep > 0 ? clipboard.sourcePath.substring(0, srcLastSep) : ''
        if (srcParent && srcParent !== targetDir) {
          fileTreePanelRef.value?.refreshNode(srcParent)
        }
        clearClipboard()
      }
    } else {
      ElMessage.error(result || '粘贴失败')
    }
  } catch (error) {
    ElMessage.error('粘贴失败: ' + (error.message || String(error)))
  }
}
```

**Step 4: 添加 watch 清除剪贴板**

在 `onMounted` 之前添加：

```js
watch(() => selectedDirectoryId.value, () => {
  clearClipboard()
})
```

**Step 5: 修改模板 - FileTreePanel**

将 FileTreePanel 标签修改为：

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
/>
```

**Step 6: 修改模板 - ContentPanel**

将 ContentPanel 标签修改为：

```html
<ContentPanel
  ref="contentPanelRef"
  :selected-node="selectedNode"
  :latest-commit="latestCommit"
  :clipboard="clipboard"
  @latest-commit="commit => latestCommit = commit"
  @refresh-node="onRefreshNode"
  @create-directory="node => fileTreePanelRef.showCreateAt(node, 'directory')"
  @create-file="node => fileTreePanelRef.showCreateAt(node, 'file')"
  @rename="onRenameFromContent"
  @delete="onDeleteFromContent"
  @copy="handleCopy"
  @cut="handleCut"
  @paste="handlePaste"
/>
```

**Step 7: 提交前端变更**

```bash
git add frontend/src/components/FileTreePanel.vue frontend/src/components/ContentPanel.vue frontend/src/views/Home.vue
git commit -m "feat: 文件树右键菜单和操作面板新增剪切/复制/粘贴功能"
```

---

### Task 8: 生成 Wails 绑定 + 集成验证

**Step 1: 重新生成 Wails 前端绑定**

Run: `cd workbench && wails generate module`
（或者启动 `wails dev` 时会自动生成）

确认 `frontend/wailsjs/go/main/App.js` 中包含 `CopyItem` 和 `MoveItem` 导出。
确认 `frontend/wailsjs/go/main/App.d.ts` 中包含对应类型声明。

**Step 2: 启动开发环境验证**

Run: `cd workbench && wails dev`

验证清单：
- [ ] 文件夹右键菜单显示 剪切/复制/粘贴 三项
- [ ] 文件右键菜单显示 剪切/复制/粘贴 三项
- [ ] 粘贴在无剪贴板内容时灰色不可点击
- [ ] 复制文件后提示成功，粘贴按钮变为可用
- [ ] 粘贴文件到目标文件夹，文件树刷新
- [ ] 剪切文件后粘贴，源文件消失，目标位置出现
- [ ] 粘贴到同名位置自动重命名为 xxx(1)
- [ ] 切换工作目录后粘贴按钮变为禁用
- [ ] 操作面板的剪切/复制/粘贴按钮功能与右键菜单一致

**Step 3: 最终提交**

```bash
git add frontend/wailsjs/
git commit -m "chore: 更新 Wails 前端绑定"
```
