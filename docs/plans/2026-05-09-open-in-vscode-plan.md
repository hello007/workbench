# 右键菜单"用 VSCode 打开"功能实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在文件树右键菜单中新增"用 VSCode 打开"选项，点击后调用 `code` 命令打开对应文件或文件夹

**Architecture:** 后端 service 层新增 `OpenInVSCode` 方法执行 `code <path>` 命令，app 层暴露绑定方法供前端调用，前端在已有右键菜单框架中新增菜单项和处理函数。与现有 `OpenInExplorer` 实现模式完全一致。

**Tech Stack:** Go 1.26, Wails v2, Vue 3, Element Plus

---

## Task 1: 后端 Service 层 — 新增 OpenInVSCode 方法

**Files:**
- Modify: `service/fileoperation.go` (在 `OpenInExplorer` 方法之后追加)
- Test: `service/fileoperation_test.go` (在末尾追加)

**Step 1: 编写失败测试**

在 `service/fileoperation_test.go` 末尾追加：

```go
func TestOpenInVSCode_Directory(t *testing.T) {
	dir := t.TempDir()
	svc := NewFileOperationService()

	err := svc.OpenInVSCode(dir)
	if err != nil {
		t.Fatalf("OpenInVSCode(directory) failed: %v", err)
	}
}

func TestOpenInVSCode_File(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	os.WriteFile(file, []byte("test"), 0644)

	svc := NewFileOperationService()

	err := svc.OpenInVSCode(file)
	if err != nil {
		t.Fatalf("OpenInVSCode(file) failed: %v", err)
	}
}

func TestOpenInVSCode_InvalidCommand(t *testing.T) {
	// 测试空路径时 exec.Command 不会 panic
	svc := NewFileOperationService()

	// code "" 会启动 VSCode 打开空参数（不会崩溃）
	err := svc.OpenInVSCode("")
	// 不验证结果，只验证不 panic
	_ = err
}
```

**Step 2: 运行测试确认失败**

Run: `cd workbench && go test ./service -run TestOpenInVSCode -v`
Expected: 编译失败，`svc.OpenInVSCode undefined`

**Step 3: 实现最小代码**

在 `service/fileoperation.go` 的 `OpenInExplorer` 方法之后追加：

```go
// OpenInVSCode 用 VSCode 打开文件或文件夹
func (s *FileOperationService) OpenInVSCode(path string) error {
	cmd := exec.Command("code", path)
	util.HideCommandWindow(cmd)
	return cmd.Start()
}
```

无需额外 import — `os/exec` 和 `workbench/util` 已在文件顶部导入。

**Step 4: 运行测试确认通过**

Run: `cd workbench && go test ./service -run TestOpenInVSCode -v`
Expected: PASS

**Step 5: 提交**

```bash
cd workbench
git add service/fileoperation.go service/fileoperation_test.go
git commit -m "feat: add OpenInVSCode method in FileOperationService"
```

---

## Task 2: 后端 App 层 — 新增 OpenInVSCode 绑定方法

**Files:**
- Modify: `app.go:413-420` (在 `OpenInExplorer` 方法之后追加)

**Step 1: 添加绑定方法**

在 `app.go` 的 `OpenInExplorer` 方法（第 420 行）之后追加：

```go
// OpenInVSCode 用 VSCode 打开
func (a *App) OpenInVSCode(path string) bool {
	err := a.fileOpSvc.OpenInVSCode(path)
	if err != nil {
		println("Error:", err.Error())
		return false
	}
	return true
}
```

**Step 2: 验证编译通过**

Run: `cd workbench && go build`
Expected: 编译成功，无错误

**Step 3: 提交**

```bash
cd workbench
git add app.go
git commit -m "feat: add OpenInVSCode binding method in App"
```

---

## Task 3: 前端 — 右键菜单新增"用 VSCode 打开"选项

**Files:**
- Modify: `frontend/src/views/Home.vue`

**Step 1: 添加菜单项**

在右键菜单的 **文件夹模板** 中（约第 329 行 `"在资源管理器中打开"` 之后），追加：

```html
<li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
  <el-icon><EditPen /></el-icon>用 VSCode 打开
</li>
```

在右键菜单的 **文件模板** 中（约第 349 行 `"在资源管理器中打开"` 之后），追加同样的菜单项：

```html
<li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
  <el-icon><EditPen /></el-icon>用 VSCode 打开
</li>
```

**Step 2: 导入图标和后端方法**

在 `<script setup>` 的图标导入中（约第 374 行），追加 `EditPen`：

```javascript
import {
  Folder, FolderOpened, Document, SuccessFilled,
  CircleCloseFilled, FolderAdd, DocumentAdd,
  Edit, Delete, CopyDocument, Monitor, Refresh,
  EditPen
} from '@element-plus/icons-vue'
```

在后端方法导入中（约第 387 行），追加 `OpenInVSCode`：

```javascript
import {
  GetDirectories, AddDirectory,
  GetFileTree,
  CreateDirectory, CreateFile, RenameFile, DeleteFile, PreviewFile,
  PullRepo, CloneRepo,
  GetCommitHistory,
  OpenInExplorer,
  OpenInVSCode,
  ScanAndPullRepos
} from '../../wailsjs/go/main/App'
```

**Step 3: 添加菜单命令处理**

在 `onMenuCommand` 函数的 switch 语句中（约第 601 行 `case 'openExplorer'` 之后），追加：

```javascript
case 'openInVSCode':
    handleOpenInVSCode(data.path)
    break
```

在 `handleOpenExplorer` 函数之后（约第 727 行），追加新函数：

```javascript
const handleOpenInVSCode = async (path) => {
  try {
    const result = await OpenInVSCode(path)
    if (!result) {
      ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
    }
  } catch (error) {
    ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
  }
}
```

**Step 4: 验证前端编译通过**

Run: `cd workbench/frontend && npm run build`
Expected: 构建成功，无错误

**Step 5: 提交**

```bash
cd workbench
git add frontend/src/views/Home.vue
git commit -m "feat: add 'Open in VSCode' to file tree context menu"
```

---

## Task 4: 集成验证

**Step 1: 全量后端测试**

Run: `cd workbench && go test ./... -v`
Expected: 所有测试 PASS

**Step 2: Wails 开发模式验证**

Run: `cd workbench && wails dev`

手动验证步骤：
1. 选择一个工作目录
2. 右键点击一个**文件夹** → 确认菜单显示"用 VSCode 打开"
3. 点击"用 VSCode 打开" → 确认 VSCode 打开了该文件夹
4. 右键点击一个**文件** → 确认菜单显示"用 VSCode 打开"
5. 点击"用 VSCode 打开" → 确认 VSCode 打开了该文件
6. 点击其他区域 → 确认右键菜单消失

**Step 3: 构建验证**

Run: `cd workbench && wails build`
Expected: 构建成功，生成 `build/bin/workbench.exe`
