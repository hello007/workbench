# Story 3.2: 重命名和删除

Status: review

## Story

As a 开发者,
I want 重命名或删除文件和文件夹,
So that 我可以在文件树中直接管理文件。

## Acceptance Criteria

1. **AC1 - 重命名（FR14）**：右键点击文件或文件夹节点，选择"重命名"并输入新名称，该文件/文件夹更新为新名称
2. **AC2 - 删除（FR15）**：右键点击文件或文件夹节点，选择"删除"，弹出确认对话框，用户确认后执行删除
3. **AC3 - 删除二次确认（NFR9）**：删除操作需弹出 ElMessageBox.confirm 确认，用户取消时不执行删除

## Tasks / Subtasks

- [x] Task 1: 验证后端重命名/删除逻辑（AC: #1, #2）
  - [x] 1.1 阅读 `service/fileoperation.go:45-54` 的 `Rename(oldPath, newName)` 方法：`filepath.Dir` 提取目录 + `filepath.Join` 拼新路径 + `os.Stat` 冲突检查 + `util.RenamePath`
  - [x] 1.2 阅读 `service/fileoperation.go:57-59` 的 `Delete(path)` 方法：直接委托 `util.RemovePath`（底层 `os.RemoveAll`）
  - [x] 1.3 验证 `app.go:164-171` 的 `RenameFile` Wails 绑定（标准调度层 ≤10 行，返回 bool）
  - [x] 1.4 验证 `app.go:174-181` 的 `DeleteFile` Wails 绑定（同上模式）
  - [x] 1.5 确认 `fileoperation_test.go:110-176` 已有 4 个测试：TestRename_File、TestRename_TargetExists、TestDelete_File、TestDelete_Directory

- [x] Task 2: 验证前端重命名/删除交互（AC: #1, #2, #3）
  - [x] 2.1 阅读 `FileTreePanel.vue:75-99` 的重命名对话框模板：当前名称(disabled) + 新名称输入 + 确定/取消按钮
  - [x] 2.2 阅读 `FileTreePanel.vue:313-317` 的重命名状态：`renameDialogVisible`、`renameName`、`renameLoading`、`renameInputRef`、`renameNode`
  - [x] 2.3 阅读 `FileTreePanel.vue:563-574` 的 `showRenameAt(data)`：预填当前名称 + 延迟聚焦选中
  - [x] 2.4 阅读 `FileTreePanel.vue:576-603` 的 `handleRename()`：空名校验 → RenameFile API → 成功刷新父节点
  - [x] 2.5 阅读 `FileTreePanel.vue:606-637` 的 `handleDeleteAt(data)`：ElMessageBox.confirm 确认 → DeleteFile API → 成功刷新父节点
  - [x] 2.6 验证右键菜单触发：文件和文件夹节点均有重命名/删除菜单项（`FileTreePanel.vue:159-164, 200-205`）
  - [x] 2.7 验证 onMenuCommand 分发：`rename` → `showRenameAt(data)`，`delete` → `handleDeleteAt(data)`（`FileTreePanel.vue:480-484`）
  - [x] 2.8 验证 ContentPanel 联动：文件操作区重命名/删除按钮 → Home.vue 路由到 FileTreePanel（`Home.vue:37-38, 137-139, 142-174`）

- [x] Task 3: 编写前端测试（AC: #1, #2, #3）
  - [x] 3.1 编写 `showRenameAt` 测试：验证调用后对话框打开，新名称输入框预填当前名称
  - [x] 3.2 编写 `handleRename` 成功测试：验证 RenameFile 调用参数正确 + ElMessage.success
  - [x] 3.3 编写 `handleRename` 空名测试：验证 ElMessage.warning
  - [x] 3.4 编写 `handleRename` 失败测试：验证 RenameFile 返回 false 时 ElMessage.error
  - [x] 3.5 编写 `handleDeleteAt` 确认后删除测试：验证 ElMessageBox.confirm → DeleteFile → ElMessage.success
  - [x] 3.6 编写 `handleDeleteAt` 取消测试：验证用户取消确认时不调用 DeleteFile
  - [x] 3.7 编写 `handleDeleteAt` 失败测试：验证 DeleteFile 返回 false 时 ElMessage.error
  - [x] 3.8 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**重命名和删除功能已完整实现并投产。** 本 Story 属于验证性质，确认后端 `Rename`/`Delete` 和前端对话框/确认交互满足 FR14/FR15/NFR9 的所有要求，并补充前端测试覆盖。

### 现有实现分析

**Go 后端 — 重命名服务：**

- `service/fileoperation.go:45-54` — `Rename(oldPath, newName)`：
  ```go
  func (s *FileOperationService) Rename(oldPath, newName string) error {
      dir := filepath.Dir(oldPath)
      newPath := filepath.Join(dir, newName)
      if _, err := os.Stat(newPath); err == nil {
          return os.ErrExist
      }
      return util.RenamePath(oldPath, newPath)
  }
  ```
  - `filepath.Dir` 从旧路径提取父目录
  - `filepath.Join` 拼接新名称得到新路径
  - 目标冲突检查：`os.Stat(newPath)` 存在则返回 `os.ErrExist`
  - 委托 `util.RenamePath`（底层 `os.Rename`）

**Go 后端 — 删除服务：**

- `service/fileoperation.go:57-59` — `Delete(path)`：
  ```go
  func (s *FileOperationService) Delete(path string) error {
      return util.RemovePath(path)
  }
  ```
  - 无额外校验，直接委托 `util.RemovePath`（底层 `os.RemoveAll`，递归删除）

**Go 后端 — Wails 绑定：**

- `app.go:164-171` — `RenameFile(oldPath, newName) bool`：标准调度层，调用 `fileOpSvc.Rename`
- `app.go:174-181` — `DeleteFile(path) bool`：标准调度层，调用 `fileOpSvc.Delete`

**现有后端测试覆盖（4 个）：**

- `TestRename_File`（`fileoperation_test.go:110-128`）— 重命名文件，验证旧文件消失、新文件存在
- `TestRename_TargetExists`（`fileoperation_test.go:130-141`）— 目标名冲突返回 `os.ErrExist`
- `TestDelete_File`（`fileoperation_test.go:143-158`）— 删除文件
- `TestDelete_Directory`（`fileoperation_test.go:160-176`）— 递归删除目录及其内容

**前端 — 重命名对话框：**

- `FileTreePanel.vue:75-99` — 重命名对话框模板：
  - `<el-input :model-value="renameNode?.name" disabled />` 显示当前名称
  - `<el-input v-model="renameName" @keyup.enter="handleRename" />` 新名称输入
  - 确定按钮 `:loading="renameLoading"` + `:disabled="renameLoading"`

- `FileTreePanel.vue:313-317` — 重命名状态：
  - `renameDialogVisible`、`renameName`、`renameLoading`、`renameInputRef`、`renameNode`

- `FileTreePanel.vue:563-574` — `showRenameAt(data)`：
  ```javascript
  const showRenameAt = (data) => {
    renameNode.value = data
    renameName.value = data.name
    renameDialogVisible.value = true
    setTimeout(() => {
      const input = renameInputRef.value?.input
      if (input) { input.focus(); input.select() }
    }, 100)
  }
  ```
  - 预填当前名称，100ms 延迟后聚焦并选中文本
  - `showRenameAt` 已通过 `defineExpose` 暴露

- `FileTreePanel.vue:576-603` — `handleRename()`：
  1. 空名校验：`!renameName.value.trim()` → `ElMessage.warning('请输入名称')`
  2. `renameLoading.value = true`
  3. `await RenameFile(renameNode.value.path, renameName.value.trim())`
  4. 成功：`ElMessage.success('重命名成功')` + 关闭对话框 + `refreshNode(parentPath)`
  5. 失败：`ElMessage.error('重命名失败')`
  6. 父路径计算：`lastIndexOf('\\')` 优先，空则 `lastIndexOf('/')`
  - `handleRename` 未通过 `defineExpose` 暴露

**前端 — 删除确认：**

- `FileTreePanel.vue:606-637` — `handleDeleteAt(data)`：
  1. `ElMessageBox.confirm('确定要删除 "${data.name}" 吗？此操作不可撤销。', '警告', {...})`
  2. 用户取消 → catch 中 return，不执行删除
  3. `await DeleteFile(targetPath)`
  4. 成功：`ElMessage.success('删除成功')` + `refreshNode(parentPath)`
  5. 失败：`ElMessage.error('删除失败')`
  6. 父路径计算方式与 handleRename 相同
  - `handleDeleteAt` 未通过 `defineExpose` 暴露

**前端 — 右键菜单：**

- 文件夹右键菜单（`FileTreePanel.vue:159-164`）：重命名、删除
- 文件右键菜单（`FileTreePanel.vue:200-205`）：重命名、删除
- onMenuCommand 分发（`FileTreePanel.vue:468-523`）：`rename` → `showRenameAt(data)`，`delete` → `handleDeleteAt(data)`

**前端 — ContentPanel 联动：**

- `ContentPanel.vue:64-68` — 文件操作区"重命名"和"删除"按钮（仅文件类型显示，文件夹无此按钮）
- `Home.vue:37-38` — 事件路由：`@rename="onRenameFromContent"`，`@delete="onDeleteFromContent"`
- `Home.vue:137-139` — `onRenameFromContent(node)`：转发到 `fileTreePanelRef.value?.showRenameAt(node)`
- `Home.vue:142-174` — `onDeleteFromContent(node)`：**独立实现删除逻辑**（重复 ElMessageBox.confirm + DeleteFile 调用），额外清空 `selectedNode.value = null`

### 数据流

```
重命名（右键菜单触发）:
  右键节点 → contextMenu.data = data → 菜单显示
  → 点击"重命名" → onMenuCommand('rename') → showRenameAt(data)
  → 对话框打开（预填当前名称） → 修改名称 → 点击确定 → handleRename()
  → RenameFile(oldPath, newName) → Go Rename → os.Rename
  → 成功 → ElMessage.success + refreshNode + 关闭对话框

重命名（ContentPanel 触发）:
  选中文件 → ContentPanel 显示"重命名"按钮
  → 点击 → emit('rename', selectedNode) → Home 路由 → showRenameAt(node)
  → 同上流程

删除（右键菜单触发）:
  右键节点 → 点击"删除" → onMenuCommand('delete') → handleDeleteAt(data)
  → ElMessageBox.confirm → 用户确认
  → DeleteFile(targetPath) → Go Delete → os.RemoveAll
  → 成功 → ElMessage.success + refreshNode

删除（ContentPanel 触发）:
  选中文件 → 点击"删除" → emit('delete', selectedNode) → Home.onDeleteFromContent(node)
  → ElMessageBox.confirm → DeleteFile → 成功后额外清空 selectedNode
```

### 数据契约

```go
// Go → Wails 绑定
RenameFile(oldPath string, newName string) bool
DeleteFile(path string) bool

// 前端调用
const result = await RenameFile(nodePath, newName)
const result = await DeleteFile(nodePath)
// result: true=成功, false=失败
```

### 架构约束

- **app.go 调度层**：RenameFile/DeleteFile 方法只调用 `fileOpSvc`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **错误处理链**：service 返回 error → app.go println 日志 → 前端 ElMessage 提示
- **异步防护**：`renameLoading` ref + 确定按钮 `:loading` + `:disabled`
- **删除确认**：ElMessageBox.confirm 二次确认（NFR9）

### 前一个 Story 的经验教训（Story 3-1）

1. **handleCreate 未暴露**：通过 UI 交互触发（找到确定按钮并点击），`handleRename` 同理未暴露
2. **createWrapperWithStore**：已有 `createWrapperWithStore` 辅助函数可复用，用于测试 refreshNode
3. **el-input stub 区分**：通过 `element.value` 区分不同输入框（有值为父路径，空值为名称输入）
4. **findComponent by name 不适用于 stub**：直接用 `wrapper.findAll('button')` 查找按钮

### 更早期的经验教训

1. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`
2. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`
3. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock（RenameFile/DeleteFile 已存在）
4. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
5. **ElMessageBox 已在 FileTreePanel.spec.js 中 mock**：`ElMessageBox: { confirm: vi.fn() }`，可直接控制返回值
6. **onNodeClick 通过 $attrs 访问**：`wrapper.findComponent('.el-tree').vm.$attrs.onNodeClick`

### 测试注意事项

**后端测试（fileoperation_test.go）：**

- 已有 4 个测试覆盖重命名/删除基本场景，无需额外补充
- 覆盖场景：重命名文件成功、目标名冲突、删除文件成功、删除目录成功

**前端测试（FileTreePanel.spec.js 扩展）：**

- `showRenameAt` 测试：`wrapper.vm.showRenameAt(data)` 是暴露的，调用后验证对话框出现
  - 验证方式：`wrapper.findAll('input')` 中能找到预填了节点名称的输入框

- `handleRename` 测试：未暴露，需通过 UI 交互触发
  - 调用 `showRenameAt` 打开对话框 → 修改 `renameName` 输入值 → 点击"确定"按钮
  - 注意：重命名对话框有两个 input（当前名称 disabled + 新名称可编辑），通过 value 区分
  - 当前名称输入框 value 为原始名称，新名称输入框 value 为可修改的值
  - 由于 el-input stub 是 `<input :value="modelValue">`，disabled 不影响 HTML value
  - **区分方式**：调用 showRenameAt 后，新名称输入框预填了与当前名称相同的值
  - 修改时：找到第二个 el-form-item 中的 input 并 setValue
  - 实际方案：使用 `wrapper.findAll('input')`，找到值等于节点名称的 input（新名称），setValue 新名称

- `handleDeleteAt` 测试：未暴露，通过 `onMenuCommand('delete')` 间接调用
  - 或者直接在测试中调用（虽然未暴露，但可在 onMenuCommand 中通过右键菜单分发触发）
  - **可行路径**：通过 `onMenuCommand` 分发触发（但 onMenuCommand 也未暴露）
  - **替代方案**：直接 mock `ElMessageBox.confirm` 返回 resolved/rejected，然后通过右键菜单分发触发

- **ElMessageBox.confirm mock 控制**：
  - `ElMessageBox.confirm` 已在 FileTreePanel.spec.js 中 mock 为 `vi.fn()`
  - 测试删除确认：`ElMessageBox.confirm.mockResolvedValueOnce('confirm')`（确认）
  - 测试删除取消：`ElMessageBox.confirm.mockRejectedValueOnce('cancel')`（取消）

- `createWrapperWithStore` 辅助函数：已定义，可复用于 refreshNode 测试

### 关键验证点

1. **重命名冲突检查**：后端 `os.Stat(newPath)` 检查，已存在返回 `os.ErrExist`，不覆盖
2. **空名验证**：前端 `!renameName.value.trim()` 为空时提示警告，不发送请求
3. **删除二次确认**：`ElMessageBox.confirm` 必须弹出，用户取消时不执行删除（NFR9）
4. **loading 防护**：`renameLoading` 在重命名期间为 true，确定按钮 loading + disabled
5. **刷新父节点**：重命名/删除成功后 `refreshNode(parentPath)` 更新文件树
6. **文件和文件夹均可操作**：右键菜单中文件和文件夹节点都有重命名/删除选项
7. **ContentPanel 删除额外清空选中**：Home.vue `onDeleteFromContent` 成功后设置 `selectedNode.value = null`

### 已知问题（不在本 Story 范围）

1. **删除逻辑重复**：FileTreePanel.handleDeleteAt 和 Home.onDeleteFromContent 是两套独立实现
2. **父路径提取脆弱**：`lastIndexOf('\\')` + `lastIndexOf('/')` 在根目录场景下返回空串
3. **重命名不阻止同名**：仅检查空名，不检查是否与当前名称相同
4. **ContentPanel 文件夹无重命名/删除按钮**：仅文件类型显示这些按钮

### References

- [Source: service/fileoperation.go:45-54] — Rename 方法
- [Source: service/fileoperation.go:57-59] — Delete 方法
- [Source: util/file.go:92-94] — RenamePath (os.Rename)
- [Source: util/file.go:97-99] — RemovePath (os.RemoveAll)
- [Source: app.go:164-171] — RenameFile Wails 绑定
- [Source: app.go:174-181] — DeleteFile Wails 绑定
- [Source: service/fileoperation_test.go:110-176] — 现有后端测试
- [Source: frontend/src/components/FileTreePanel.vue:75-99] — 重命名对话框模板
- [Source: frontend/src/components/FileTreePanel.vue:313-317] — 重命名状态
- [Source: frontend/src/components/FileTreePanel.vue:563-574] — showRenameAt 方法
- [Source: frontend/src/components/FileTreePanel.vue:576-603] — handleRename 方法
- [Source: frontend/src/components/FileTreePanel.vue:606-637] — handleDeleteAt 方法
- [Source: frontend/src/components/FileTreePanel.vue:159-164, 200-205] — 右键菜单重命名/删除项
- [Source: frontend/src/components/FileTreePanel.vue:468-523] — onMenuCommand 分发
- [Source: frontend/src/components/FileTreePanel.vue:726-735] — defineExpose
- [Source: frontend/src/components/ContentPanel.vue:64-68] — ContentPanel 重命名/删除按钮
- [Source: frontend/src/views/Home.vue:37-38] — 事件路由
- [Source: frontend/src/views/Home.vue:137-174] — onRenameFromContent / onDeleteFromContent
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证后端 Rename/Delete 服务：Rename 有冲突检查（os.Stat），Delete 直接 os.RemoveAll
- 验证 app.go Wails 绑定调度层：标准调度模式，≤10 行
- 验证前端重命名对话框：showRenameAt 预填名称+聚焦选中 → handleRename 空名校验+API调用+刷新
- 验证前端删除确认：handleDeleteAt 使用 ElMessageBox.confirm 二次确认，取消时不执行删除
- FileTreePanel.spec.js 新增 7 个前端测试：showRenameAt 对话框打开、handleRename 成功/空名/失败、handleDeleteAt 确认/取消/失败
- 测试要点：重命名对话框有两个同值 input（disabled + v-model），需取最后一个匹配项
- 全量测试通过：Go 全绿，前端 55 个组件测试全绿（FileTreePanel 28 + ContentPanel 3 + Home 1 + ContextMenu 20 通过）

### Code Review 修复（2026-05-21）

- F1/F2/F3 (High): 重写 3 个删除测试，改为通过 `wrapper.vm.handleDeleteAt(data)` 调用组件方法，而非直接调用 mock 函数
- F6 (Medium): 重命名成功测试补充 `expect(mockExpand).toHaveBeenCalled()` 验证 refreshNode 刷新
- 生产代码改动：`handleDeleteAt` 添加到 `defineExpose` 以支持测试访问
- 修复后全量测试通过：77 个前端测试全绿

### File List

- `frontend/src/components/__tests__/FileTreePanel.spec.js` — 新增 7 个测试
- `frontend/src/components/FileTreePanel.vue` — defineExpose 添加 handleDeleteAt
