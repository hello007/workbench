# Story 1.2: 工作目录添加与移除

Status: done

## Story

As a 开发者,
I want 添加本地目录到工作目录列表，或从中移除,
so that 我可以管理常用的工作目录集合。

## Acceptance Criteria

1. **AC1 - 添加目录**：用户输入目录名称和路径并确认添加，系统验证路径有效性（NFR8: 路径规范化），有效则添加到列表；无效路径提示错误信息
2. **AC2 - 移除目录**：用户选择移除某个目录，目录从列表中移除，持久化配置同步更新（FR4）
3. **AC3 - 路径安全（NFR8）**：所有用户输入路径必须经过 `filepath.Clean` + `filepath.Abs` 规范化处理，重复路径检测（"该目录已添加"）

## Tasks / Subtasks

- [x] Task 1: 验证 Go 后端目录添加逻辑（AC: #1, #3）
  - [x] 1.1 阅读 service/directory.go 的 Create 方法，确认路径验证流程（FileExists → Abs → 重复检测）
  - [x] 1.2 阅读 app.go 的 AddDirectory 方法，确认调度层逻辑（调用 service → 错误处理 → 返回值）
  - [x] 1.3 编写 service/directory_test.go 测试：添加成功、路径不存在、重复路径、路径规范化
  - [x] 1.4 运行 Go 测试确认通过

- [x] Task 2: 验证 Go 后端目录删除逻辑（AC: #2）
  - [x] 2.1 阅读 service/directory.go 的 Delete 方法，确认删除和持久化逻辑
  - [x] 2.2 阅读 app.go 的 DeleteDirectory 方法，确认调度层逻辑
  - [x] 2.3 编写 service/directory_test.go 测试：删除成功、删除不存在的目录、删除后持久化验证
  - [x] 2.4 运行 Go 测试确认通过

- [x] Task 3: 验证前端 DirectoryTree 组件添加功能（AC: #1）
  - [x] 3.1 阅读 DirectoryTree.vue 的 handleAdd 方法，确认表单验证和 Wails 调用流程
  - [x] 3.2 检查 setup.js mock 中 AddDirectory 的配置是否正确
  - [x] 3.3 编写 DirectoryTree.spec.js 测试：添加对话框渲染、空名称/空路径校验、添加成功流程、添加失败处理
  - [x] 3.4 运行前端测试确认通过

- [x] Task 4: 验证前端 DirectoryTree 组件删除功能（AC: #2）
  - [x] 4.1 阅读 DirectoryTree.vue 的 handleDelete 方法，确认确认对话框和 Wails 调用流程
  - [x] 4.2 编写 DirectoryTree.spec.js 测试：删除确认对话框、删除成功流程、取消删除、删除失败处理
  - [x] 4.3 运行前端测试确认通过

- [x] Task 5: 验证路径安全（AC: #3）
  - [x] 5.1 阅读 service/directory.go Create 方法中的 filepath.Abs 调用，确认路径规范化
  - [x] 5.2 编写 Go 测试验证路径规范化（相对路径转绝对路径、路径分隔符统一）
  - [x] 5.3 运行全量测试确认无回归

- [x] Task 6: 运行全量测试并修复发现的问题（AC: #1, #2, #3）
  - [x] 6.1 运行 `go test ./...` 确认后端无回归
  - [x] 6.2 运行 `cd frontend && npm test` 确认前端无回归（预存失败除外）
  - [x] 6.3 修复任何新引入的测试失败

## Dev Notes

### 棕地项目背景

**此 Story 的功能已全部实现并投产。** 本 Story 的目标是验证现有实现满足 AC 要求，补充测试覆盖。

### 现有实现分析

**Go 后端（三层架构）：**

1. **model/models.go** — `Directory` 结构体
   ```go
   type Directory struct {
       ID         string    `json:"id"`
       Name       string    `json:"name"`
       Path       string    `json:"path"`
       IsDefault  bool      `json:"isDefault"`
       CreateTime time.Time `json:"createTime"`
   }
   ```

2. **service/directory.go** — `DirectoryService` 业务逻辑
   - `Create(name, path, isDefault)` → FileExists 校验 → filepath.Abs 规范化 → 重复路径检测 → 生成 ID → 保存到 JSON
   - `Delete(id)` → 加载列表 → 过滤目标 ID → 保存剩余列表
   - `SetDefault(id)` → 加载列表 → 切换 isDefault 标志 → 保存
   - `Update(id, name, path, isDefault)` → 路径校验 → 更新字段 → 保存
   - 持久化：`util.LoadJSON` / `util.SaveJSON`，配置文件 `data/directories.json`

3. **app.go** — Wails 绑定调度层（~10 行/方法）
   - `AddDirectory(name, path, isDefault)` → 调用 service.Create → 错误日志 + 返回 nil
   - `DeleteDirectory(id)` → 调用 service.Delete → 返回 bool
   - `UpdateDirectory(id, name, path, isDefault)` → 调用 service.Update
   - `SetDefaultDirectory(id)` → 调用 service.SetDefault
   - `GetDirectories()` → 调用 service.Load

**前端 DirectoryTree.vue：**

- **添加目录**：工具栏 `+` 按钮 → `showAddDialog()` → `el-dialog` 表单（名称/路径/默认开关） → `handleAdd()` → 前端校验（空名称/空路径） → `AddDirectory()` Wails 调用 → `emit('change')` 通知父组件刷新
- **删除目录**：右键菜单 → `handleDelete()` → `ElMessageBox.confirm` 二次确认 → `DeleteDirectory()` Wails 调用 → `emit('change')`
- **重命名**：右键菜单 → `showRenameDialog()` → `el-dialog` 表单 → `UpdateDirectory()` Wails 调用 → `emit('change')`
- **设为默认**：右键菜单 → `SetDefaultDirectory()` Wails 调用 → `emit('change')`
- **事件监听清理**：`onMounted` 注册 `document.addEventListener('click', onGlobalClick)` → `onBeforeUnmount` 移除
- **防重复提交**：`addLoading` / `renameLoading` ref 控制按钮 loading 状态

**Home.vue 数据流：**

- `DirectoryTree` 通过 `emit('select', dirId)` 通知目录选中
- `DirectoryTree` 通过 `emit('change')` 通知目录列表变更
- Home.vue 监听 `@change="loadDirectories"` 重新加载目录列表

### 架构约束

- **三层架构**：app.go（调度层 ≤10行/方法）→ service（业务层，不依赖 Wails）→ util（工具层）
- **Service 独立性**：service/directory.go 不导入 Wails 包
- **JSON 持久化**：`util.LoadJSON/SaveJSON` 统一处理配置读写
- **组件命名**：PascalCase.vue
- **测试框架**：前端 Vitest + Vue Test Utils（jsdom），Go testing
- **测试 Mock**：`frontend/src/test/setup.js` 全局 mock Wails 绑定
- **禁止**：引入新依赖、TypeScript、CSS 框架

### 测试注意事项

**Go 测试（service/directory_test.go，需新建）：**
- 使用 `t.TempDir()` 创建真实文件系统
- 表驱动测试模式：`[]struct{ name string; ... }`
- 断言用 `t.Errorf`，格式 `"funcName(input): got %v, want %v"`
- 测试文件与源码同包（`package service`）

**前端测试（DirectoryTree.spec.js，需新建）：**
- 测试文件路径：`frontend/src/components/__tests__/DirectoryTree.spec.js`
- Element Plus 组件通过 `stubs` 替换（el-button, el-dialog, el-form, el-input 等）
- Wails 绑定已在 `setup.js` 全局 mock（AddDirectory, DeleteDirectory, UpdateDirectory, SetDefaultDirectory）
- **必须** 使用 `afterEach(() => wrapper.unmount())` 清理
- ElMessageBox 需在测试文件中额外 mock：`vi.mock('element-plus', ...)`
- 异步操作需 `await flushPromises()` 等待

**setup.js 已有的 Mock：**
```javascript
AddDirectory: vi.fn(() => Promise.resolve(true)),
// 注意：DeleteDirectory, UpdateDirectory, SetDefaultDirectory 未在 setup.js 中
// 需要在测试文件中补充 mock 或更新 setup.js
```

### 路径安全验证策略（NFR8）

- `filepath.Abs()` 将相对路径转为绝对路径
- `filepath.Clean()` 由 `filepath.Abs` 内部调用，处理 `..`、`.`、多余分隔符
- 重复路径检测：遍历已保存目录列表，比较 `dir.Path == absPath`
- 当前实现**未使用** `filepath.Clean` 单独调用，依赖 `filepath.Abs` 的隐式清理

### References

- [Source: service/directory.go] — 目录 CRUD 业务逻辑
- [Source: model/models.go] — Directory 数据模型
- [Source: app.go] — Wails 绑定调度层（AddDirectory, DeleteDirectory, UpdateDirectory, SetDefaultDirectory, GetDirectories）
- [Source: frontend/src/components/DirectoryTree.vue] — 目录管理前端组件
- [Source: frontend/src/views/Home.vue] — 主页面，@change="loadDirectories" 事件监听
- [Source: frontend/src/test/setup.js] — 全局 mock 配置
- [Source: docs/project-context.md] — 项目编码规范和约束

## Dev Agent Record

### Agent Model Used

glm-5-turbo

### Debug Log References

- Go 测试 11/11 全部通过
- 前端新增 16 个 DirectoryTree 测试全部通过
- Pre-existing 测试失败（19/66）与本次变更无关
- 补充 setup.js 缺失的 DeleteDirectory/UpdateDirectory/SetDefaultDirectory mock

### Completion Notes List

- ✅ AC1 添加目录：service.Create 实现路径验证（FileExists → Abs 规范化 → 重复检测），前端 handleAdd 实现表单验证（空名称/空路径）+ AddDirectory Wails 调用 + loading 防重复。Go 测试 6 个 + 前端测试 6 个覆盖全部场景
- ✅ AC2 移除目录：service.Delete 实现删除 + 持久化，前端 handleDelete 实现二次确认 + DeleteDirectory 调用。Go 测试 3 个 + 前端测试 3 个覆盖全部场景
- ✅ AC3 路径安全（NFR8）：filepath.Abs 将相对路径转绝对路径，TestCreate_PathNormalization 验证路径规范化
- 补充 setup.js mock：新增 DeleteDirectory、UpdateDirectory、SetDefaultDirectory
- 新增 service/directory_test.go（11 个用例：Create 6 + Delete 3 + Load 1 + SetDefault 1）
- 新增 DirectoryTree.spec.js（16 个用例：渲染 5 + 添加 6 + 删除 3 + 选中 1 + 清理 1）
- 代码审查后修复：setup.js AddDirectory mock 返回值类型修正（true → 对象），service/directory.go Create 方法中 Load() 错误处理

### File List

- `service/directory_test.go` — 新增 Go 测试（11 个用例）
- `frontend/src/components/__tests__/DirectoryTree.spec.js` — 新增前端测试（16 个用例）
- `frontend/src/test/setup.js` — 补充 mock + 修正 AddDirectory 返回值类型
- `service/directory.go` — Create 方法 Load() 错误处理修复

### Review Findings

- [x] [Review][Patch] setup.js 中 AddDirectory mock 返回值已修正为对象（匹配 app.go *model.Directory 返回类型） [setup.js:10]
- [x] [Review][Patch] Create 方法中 Load() 错误已正确处理，不再忽略 [service/directory.go:58]
- [x] [Review][Defer] Update 方法缺少测试覆盖 — deferred, 非本 Story AC 范围（重命名/更新属于附加功能）
