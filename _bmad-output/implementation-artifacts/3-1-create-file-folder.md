# Story 3.1: 创建文件和文件夹

Status: done

## Story

As a 开发者,
I want 在指定文件夹下创建新文件或子文件夹,
So that 我可以直接在文件树中快速创建所需资源。

## Acceptance Criteria

1. **AC1 - 新建文件（FR12）**：右键点击文件夹节点，选择"新建文件"并输入文件名，在该目录下创建指定名称的空文件
2. **AC2 - 新建文件夹（FR13）**：右键点击文件夹节点，选择"新建文件夹"并输入名称，在该目录下创建指定名称的子文件夹

## Tasks / Subtasks

- [x] Task 1: 验证后端创建逻辑（AC: #1, #2）
  - [x] 1.1 阅读 `service/fileoperation.go:23-31` 的 `CreateDirectory` 方法：`filepath.Join` 拼接路径 + `os.Stat` 存在性检查 + `util.CreateDirectory`
  - [x] 1.2 阅读 `service/fileoperation.go:34-42` 的 `CreateFile` 方法：`filepath.Join` 拼接路径 + `os.Stat` 存在性检查 + `util.CreateFile`
  - [x] 1.3 验证 `app.go:144-161` 的 `CreateDirectory` 和 `CreateFile` Wails 绑定方法（调度层 ≤10 行）
  - [x] 1.4 确认 `fileoperation_test.go:50-108` 已有 4 个测试覆盖：新建目录、目录已存在、新建文件、文件已存在

- [x] Task 2: 验证前端创建对话框与交互（AC: #1, #2）
  - [x] 2.1 阅读 `FileTreePanel.vue:50-72` 的创建对话框模板：父文件夹路径 + 名称输入 + 确定/取消按钮
  - [x] 2.2 阅读 `FileTreePanel.vue:304-310` 的对话框状态：`createDialogVisible`、`createType`、`createName`、`createLoading`、`createParentData`
  - [x] 2.3 阅读 `FileTreePanel.vue:525-531` 的 `showCreateAt(data, type)` 方法：设置父节点数据和创建类型
  - [x] 2.4 阅读 `FileTreePanel.vue:533-560` 的 `handleCreate` 方法：空名验证 → CreateDirectory/CreateFile → 成功刷新父节点 → 失败提示
  - [x] 2.5 验证右键菜单触发：`onMenuCommand('createFile')` → `showCreateAt(data, 'file')`，`onMenuCommand('createDir')` → `showCreateAt(data, 'directory')`
  - [x] 2.6 验证 ContentPanel 联动：`ContentPanel.vue:47-48` 新建按钮通过 Home.vue 路由到 `fileTreePanelRef.showCreateAt`

- [x] Task 3: 编写前端测试（AC: #1, #2）
  - [x] 3.1 编写 `showCreateAt` 测试：验证调用后对话框状态正确（type、parentData、dialogVisible）
  - [x] 3.2 编写 `handleCreate` 测试：验证空名警告、CreateDirectory 调用、CreateFile 调用、成功后刷新父节点
  - [x] 3.3 编写 `handleCreate` 错误处理测试：验证创建失败时 ElMessage.error
  - [x] 3.4 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**创建文件和文件夹功能已完整实现并投产。** 本 Story 属于验证性质，确认后端 `CreateDirectory`/`CreateFile` 和前端对话框交互满足 FR12/FR13 的所有要求，并补充前端测试覆盖。

### 现有实现分析

**Go 后端 — 创建服务：**

- `service/fileoperation.go:23-31` — `CreateDirectory(parentPath, name)`：
  ```go
  func (s *FileOperationService) CreateDirectory(parentPath, name string) error {
      fullPath := filepath.Join(parentPath, name)
      if _, err := os.Stat(fullPath); err == nil {
          return os.ErrExist
      }
      return util.CreateDirectory(fullPath)
  }
  ```
  - `filepath.Join` 拼接完整路径（自动处理路径分隔符）
  - `os.Stat` 检查文件/目录是否已存在，存在返回 `os.ErrExist`
  - `util.CreateDirectory` 创建目录（底层 `os.MkdirAll`）

- `service/fileoperation.go:34-42` — `CreateFile(parentPath, name, content)`：
  ```go
  func (s *FileOperationService) CreateFile(parentPath, name, content string) error {
      fullPath := filepath.Join(parentPath, name)
      if _, err := os.Stat(fullPath); err == nil {
          return os.ErrExist
      }
      return util.CreateFile(fullPath, content)
  }
  ```
  - 同样的存在性检查模式
  - `util.CreateFile` 写入文件（底层 `os.WriteFile`）
  - 前端传入空字符串 `''` 作为 content，创建空文件

- `app.go:144-151` — `CreateDirectory` Wails 绑定：
  ```go
  func (a *App) CreateDirectory(parentPath, name string) bool {
      err := a.fileOpSvc.CreateDirectory(parentPath, name)
      if err != nil { println("Error:", err.Error()); return false }
      return true
  }
  ```
  - 标准调度层模式：调用 service → 错误日志 → 返回 bool

- `app.go:153-161` — `CreateFile` Wails 绑定：同理，调用 `fileOpSvc.CreateFile`

**现有后端测试覆盖：**

- `TestCreateDirectory_New`（`fileoperation_test.go:50-66`）— 创建新目录，验证 IsDir
- `TestCreateDirectory_AlreadyExists`（`fileoperation_test.go:68-78`）— 目录已存在返回 `os.ErrExist`
- `TestCreateFile_New`（`fileoperation_test.go:80-96`）— 创建新文件，验证内容正确
- `TestCreateFile_AlreadyExists`（`fileoperation_test.go:98-108`）— 文件已存在返回 `os.ErrExist`

**前端 — 创建对话框：**

- `FileTreePanel.vue:50-72` — 对话框模板：
  - 标题动态切换：`createType === 'directory' ? '新建文件夹' : '新建文件'`
  - 父文件夹路径：`<el-input :model-value="createParentPath" disabled />`
  - 名称输入：`<el-input v-model="createName">`，Enter 触发 `handleCreate`
  - 确定/取消按钮：确定按钮 `:loading="createLoading"`

- `FileTreePanel.vue:304-310` — 对话框状态：
  - `createDialogVisible`：对话框可见性
  - `createType`：'directory' | 'file'
  - `createName`：用户输入的名称
  - `createLoading`：创建中 loading 状态
  - `createParentData`：父节点数据对象
  - `createParentPath`：computed，`createParentData.value?.path || ''`

- `FileTreePanel.vue:525-531` — `showCreateAt(data, type)`：
  - 设置 `createParentData`、`createType`、清空 `createName`、打开对话框

- `FileTreePanel.vue:533-560` — `handleCreate()`：
  1. 空名验证：`createName.value.trim()` 为空时 `ElMessage.warning`
  2. `createParentData.value` 空值检查
  3. `createLoading.value = true`
  4. 根据 `createType` 调用 `CreateDirectory` 或 `CreateFile`
  5. 成功：`ElMessage.success` + 关闭对话框 + `refreshNode(createParentData.value.path)`
  6. 失败：`ElMessage.error`
  7. `finally`：`createLoading.value = false`

- `FileTreePanel.vue:726-735` — `defineExpose` 包含 `showCreateAt`

**前端 — 右键菜单触发：**

- `FileTreePanel.vue:152-153` — 文件夹右键菜单"新建文件"：`@click="onMenuCommand('createFile')"`
- `FileTreePanel.vue:155-156` — 文件夹右键菜单"新建文件夹"：`@click="onMenuCommand('createDir')"`
- `FileTreePanel.vue:474-477` — `onMenuCommand` 分发：`createFile` → `showCreateAt(data, 'file')`，`createDir` → `showCreateAt(data, 'directory')`

**前端 — ContentPanel 联动：**

- `ContentPanel.vue:47-48` — 文件夹操作区"新建文件夹"和"新建文件"按钮
- `Home.vue:35-36` — 路由：`@create-directory="node => fileTreePanelRef.showCreateAt(node, 'directory')"`，`@create-file="node => fileTreePanelRef.showCreateAt(node, 'file')"`

### 数据流

```
右键菜单触发:
  右键文件夹节点 → contextMenu.data = data → 菜单显示
  → 点击"新建文件" → onMenuCommand('createFile') → showCreateAt(data, 'file')
  → 对话框打开 → 输入名称 → 点击确定 → handleCreate()
  → CreateFile(parentPath, name, '') → Go CreateFile → os.WriteFile
  → 成功 → ElMessage.success + refreshNode + 关闭对话框

ContentPanel 触发:
  选中文件夹 → ContentPanel 显示操作按钮
  → 点击"新建文件" → emit('createFile', selectedNode)
  → Home.vue 路由 → fileTreePanelRef.showCreateAt(node, 'file')
  → 同上流程
```

### 数据契约

```go
// Go → Wails 绑定
CreateDirectory(parentPath string, name string) bool
CreateFile(parentPath string, name string, content string) bool

// 前端调用
const result = await CreateDirectory(parentPath, name)
const result = await CreateFile(parentPath, name, '')
// result: true=成功, false=失败
```

### 架构约束

- **app.go 调度层**：CreateDirectory/CreateFile 方法只调用 `fileOpSvc`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **错误处理链**：service 返回 error → app.go println 日志 → 前端 ElMessage 提示
- **异步防护**：`createLoading` ref + 确定按钮 `:loading="createLoading"`

### 前一个 Story 的经验教训（Story 2-4）

1. **el-tree stub 增强**：需要 mock `store` 属性才能测试 expandAll/collapseAll
2. **ContentPanel.spec.js**：新建测试文件需要完整的 stub 配置和 wailsjs mock
3. **findComponent $attrs**：通过 `wrapper.findComponent('.el-tree').vm.$attrs` 可访问事件处理器

### 更早期的经验教训

1. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`
2. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`
3. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock
4. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock

### 测试注意事项

**后端测试（fileoperation_test.go）：**

- 已有 4 个测试覆盖创建逻辑，无需额外补充
- 覆盖场景：新建目录成功、目录已存在、新建文件成功、文件已存在

**前端测试（FileTreePanel.spec.js 扩展）：**

- `showCreateAt` 测试：调用 `wrapper.vm.showCreateAt(data, 'file')`，验证对话框状态变化
  - 需要注意：`createDialogVisible`、`createType`、`createName`、`createParentData` 是内部 ref，不通过 defineExpose 暴露
  - 替代方案：验证 `CreateDirectory`/`CreateFile` mock 被调用

- `handleCreate` 测试：直接调用 `wrapper.vm.handleCreate()` 需要先设置内部状态
  - 替代方案：通过 `showCreateAt` 设置状态，然后调用 `handleCreate`
  - 但 showCreateAt 和 handleCreate 都不在 defineExpose 中...
  - 实际上 `showCreateAt` 是 defineExpose 的

- 可行的测试路径：
  1. 调用 `wrapper.vm.showCreateAt(data, 'file')` — 这个是暴露的
  2. 但 `handleCreate` 不暴露，无法直接调用
  3. 替代：通过对话框 UI 交互触发（找到对话框中的确定按钮并点击）

- 更实际的方案：测试数据流（showCreateAt → 状态设置 → 对话框渲染）

### 关键验证点

1. **存在性检查**：创建前 `os.Stat` 检查，已存在返回 `os.ErrExist`，不覆盖
2. **空名验证**：前端 `createName.value.trim()` 为空时提示警告，不发送请求
3. **loading 防护**：`createLoading` 在创建期间为 true，确定按钮显示 loading
4. **刷新父节点**：创建成功后 `refreshNode(createParentData.value.path)` 更新文件树
5. **路径拼接安全性**：`filepath.Join` 自动规范化路径
6. **右键菜单仅文件夹可见**："新建文件"和"新建文件夹"仅在文件夹节点右键菜单中显示

### References

- [Source: service/fileoperation.go:23-31] — CreateDirectory 方法
- [Source: service/fileoperation.go:34-42] — CreateFile 方法
- [Source: app.go:144-161] — CreateDirectory/CreateFile Wails 绑定
- [Source: frontend/src/components/FileTreePanel.vue:50-72] — 创建对话框模板
- [Source: frontend/src/components/FileTreePanel.vue:304-310] — 对话框状态
- [Source: frontend/src/components/FileTreePanel.vue:525-531] — showCreateAt 方法
- [Source: frontend/src/components/FileTreePanel.vue:533-560] — handleCreate 方法
- [Source: frontend/src/components/FileTreePanel.vue:474-477] — onMenuCommand createFile/createDir 分发
- [Source: frontend/src/components/FileTreePanel.vue:152-156] — 右键菜单新建文件/文件夹项
- [Source: frontend/src/components/ContentPanel.vue:47-48] — ContentPanel 新建按钮
- [Source: frontend/src/views/Home.vue:35-36] — ContentPanel → FileTreePanel 路由
- [Source: service/fileoperation_test.go:50-108] — 现有后端创建测试
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证后端 CreateDirectory/CreateFile 服务：filepath.Join 路径拼接 + os.Stat 存在性检查 + util 调用
- 验证 app.go Wails 绑定调度层：标准调度模式，≤10 行
- 验证前端创建对话框：showCreateAt → 对话框状态设置 → handleCreate → API 调用 → 成功刷新/失败提示
- 验证右键菜单和 ContentPanel 联动触发路径
- FileTreePanel.spec.js 新增 6 个前端测试：showCreateAt(file/directory) 对话框打开、CreateDirectory 成功、CreateFile 成功、空名警告、创建失败
- 全量测试通过：Go 全绿，前端组件测试 45 个全绿（FileTreePanel 21 + ContentPanel 3 + Home 1 + ContextMenu 20）
- 测试要点：handleCreate 未通过 defineExpose 暴露，通过 UI 交互触发（找到确定按钮并点击）

### File List

- `frontend/src/components/__tests__/FileTreePanel.spec.js` — 新增 6 个测试
