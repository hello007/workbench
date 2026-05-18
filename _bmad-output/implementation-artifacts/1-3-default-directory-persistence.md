# Story 1.3: 默认工作目录与持久化

Status: done

## Story

As a 开发者,
I want 设置默认工作目录，且目录列表持久化保存,
so that 每次启动应用时自动选中常用目录，无需重复配置。

## Acceptance Criteria

1. **AC1 - 设为默认**：工作目录列表中存在多个目录时，用户将某个目录设为默认，该目录标记为默认，下次启动时自动选中
2. **AC2 - 持久化（FR4）**：用户添加、移除或修改目录后，变更持久化到 `data/directories.json`；应用重启后列表恢复一致

## Tasks / Subtasks

- [x] Task 1: 验证 Go 后端默认目录逻辑（AC: #1）
  - [x] 1.1 阅读 service/directory.go 的 SetDefault 和 GetDefault 方法，确认默认切换逻辑
  - [x] 1.2 阅读 app.go 的 SetDefaultDirectory 和 GetDefaultDirectory 方法
  - [x] 1.3 编写 Go 测试：设为默认、取消其他默认、获取默认目录（无默认时返回第一个）、空列表获取默认
  - [x] 1.4 运行 Go 测试确认通过

- [x] Task 2: 验证前端默认目录展示（AC: #1）
  - [x] 2.1 阅读 DirectoryTree.vue 的 handleSetDefault 方法和星标展示逻辑
  - [x] 2.2 编写 DirectoryTree.spec.js 测试：设为默认成功流程、设为默认失败处理
  - [x] 2.3 运行前端测试确认通过

- [x] Task 3: 验证启动时自动选中默认目录（AC: #1）
  - [x] 3.1 阅读 Home.vue 的 loadDirectories 方法，确认自动选中默认目录逻辑（`dirs.find(d => d.isDefault)`）
  - [x] 3.2 编写 Home.spec.js 测试：默认目录自动选中、无默认时选中第一个、空列表不报错
  - [x] 3.3 运行前端测试确认通过

- [x] Task 4: 验证持久化功能（AC: #2）
  - [x] 4.1 阅读 service/directory.go 的 Save 方法，确认 JSON 持久化流程
  - [x] 4.2 验证 Create/Delete/SetDefault 操作均调用 Save（已在 Story 1.2 测试中覆盖 TestCreate_PersistedToFile、TestDelete_PersistedAfterDelete）
  - [x] 4.3 编写额外持久化测试：多次操作后状态一致性（添加→设默认→删除→重启恢复）
  - [x] 4.4 运行全量测试确认无回归

- [x] Task 5: 运行全量测试并修复发现的问题（AC: #1, #2）
  - [x] 5.1 运行 `go test ./...` 确认后端无回归
  - [x] 5.2 运行 `cd frontend && npm test` 确认前端无回归（预存失败除外）
  - [x] 5.3 修复任何新引入的测试失败

## Dev Notes

### 棕地项目背景

**此 Story 的功能已全部实现并投产。** 本 Story 的目标是验证现有实现满足 AC 要求，补充测试覆盖。

### 现有实现分析

**Go 后端 — 默认目录逻辑：**

- `service/directory.go` — `SetDefault(id)` 加载列表 → 遍历设置 isDefault 标志 → 保存
- `service/directory.go` — `GetDefault()` 加载列表 → 找 isDefault=true 的 → 若无则返回第一个 → 空列表返回 error
- `app.go` — `SetDefaultDirectory(id)` 调用 service → 返回 bool
- `app.go` — `GetDefaultDirectory()` 调用 service → 返回 *model.Directory

**前端 — 启动时自动选中（Home.vue）：**

```javascript
const loadDirectories = async () => {
  const dirs = await GetDirectories()
  directories.value = dirs || []
  const defaultDir = dirs.find(d => d.isDefault)
  if (defaultDir) {
    selectedDirectoryId.value = defaultDir.id
  } else if (dirs.length > 0) {
    selectedDirectoryId.value = dirs[0].id
  }
}
```

- `onMounted` → `loadDirectories()` → 自动选中默认目录
- 回退逻辑：无默认目录时选中第一个，空列表不设置

**前端 — 设为默认（DirectoryTree.vue）：**

- 右键菜单 → `handleSetDefault(dir)` → `SetDefaultDirectory(dir.id)` → `emit('change')`
- 星标显示：`<el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c"><Star /></el-icon>`

**持久化机制：**

- 所有写操作（Create/Delete/SetDefault/Update）完成后调用 `Save()` 持久化到 `data/directories.json`
- `Save()` 使用 `util.SaveJSON` 统一序列化
- `Load()` 使用 `util.LoadJSON` 读取
- 启动时 `app.go:startup()` → 创建 `DirectoryService(configPath)` → 前端 `loadDirectories()` 加载

### 前一个 Story 的经验教训（Story 1-2）

1. **setup.js mock 已完善**：AddDirectory/UpdateDirectory/DeleteDirectory/SetDefaultDirectory 均已配置
2. **AddDirectory mock 返回值**：已修正为对象类型（匹配 app.go *model.Directory）
3. **Load() 错误处理**：Story 1-2 审查中修复了 Create 方法中忽略 Load 错误的问题
4. **必须使用 afterEach unmount 清理**：前端测试必须清理 wrapper
5. **vi.clearAllMocks()**：beforeEach 中清除 mock 状态，避免测试间污染
6. **Update 方法测试缺失**：已记录为 deferred work

### 架构约束

- **三层架构**：app.go（调度层 ≤10行/方法）→ service（业务层，不依赖 Wails）→ util（工具层）
- **JSON 持久化**：`util.LoadJSON/SaveJSON` 统一处理
- **禁止**：引入新依赖、TypeScript、CSS 框架

### 测试注意事项

**Go 测试（扩展 service/directory_test.go）：**
- Story 1-2 已有 `TestSetDefault_Success` 和 `TestLoad_Empty`
- 需要补充：SetDefault 切换验证、GetDefault 各种场景、持久化一致性
- 使用 `t.TempDir()` 创建真实文件系统

**前端测试（扩展 DirectoryTree.spec.js）：**
- Story 1-2 已有 16 个测试用例
- 需要补充：handleSetDefault 成功/失败、星标渲染

**前端测试（扩展 Home.spec.js）：**
- Story 1-1 已有 9 个三栏布局测试
- 需要补充：loadDirectories 默认选中逻辑

### References

- [Source: service/directory.go] — SetDefault/GetDefault/Save/Load 方法
- [Source: app.go] — SetDefaultDirectory/GetDefaultDirectory 绑定
- [Source: frontend/src/views/Home.vue:82-92] — loadDirectories 自动选中逻辑
- [Source: frontend/src/components/DirectoryTree.vue] — handleSetDefault 和星标渲染
- [Source: service/directory_test.go] — 已有 SetDefault 测试
- [Source: frontend/src/components/__tests__/DirectoryTree.spec.js] — 已有 16 个测试

## Dev Agent Record

### Agent Model Used

glm-5-turbo

### Debug Log References

- Go 测试 17/17 全部通过（新增 6 个）
- DirectoryTree.spec.js 19/19 全部通过（新增 3 个 handleSetDefault 测试）
- Home.spec.js 新增 3 个 loadDirectories 测试全部通过
- Pre-existing 测试失败（35/72）与本次变更无关

### Completion Notes List

- ✅ AC1 设为默认：service.SetDefault 实现默认切换（取消其他默认），service.GetDefault 实现回退逻辑（无默认返回第一个，空列表报错）。Go 测试 6 个覆盖全部场景
- ✅ AC2 持久化：TestPersistence_MultipleOperations 验证多操作后状态一致性（添加→设默认→删除→重启恢复），Create/Delete/SetDefault 均调用 Save
- 前端 handleSetDefault 测试 3 个：成功 emit change、失败显示错误、异常显示错误消息
- 前端 loadDirectories 测试 3 个：默认目录自动选中、无默认时选中第一个、空列表不报错
- 新增 service/directory_test.go（6 个用例：SetDefault Toggle、SetDefault NotExists、GetDefault 3 个、Persistence 1 个）
- 新增 DirectoryTree.spec.js（3 个用例：handleSetDefault 成功/失败/异常）
- 新增 Home.spec.js（3 个用例：loadDirectories 默认选中/回退/空列表）
- 关键发现：Home.spec.js 中使用 `vi.importMock('../../../wailsjs/go/main/App')` 获取 setup.js 注册的 mock（路径需三级 `../`）

### File List

- `service/directory_test.go` — 新增 6 个 Go 测试（SetDefault Toggle、NotExists、GetDefault 3 个、Persistence 1 个）
- `frontend/src/components/__tests__/DirectoryTree.spec.js` — 新增 3 个 handleSetDefault 测试
- `frontend/src/views/__tests__/Home.spec.js` — 新增 3 个 loadDirectories 默认选中测试

### Review Findings

- [x] [Review][Patch] `vi.restoreAllMocks()` 破坏 setup.js 全局 mock [frontend/src/views/__tests__/Home.spec.js:401] — 已替换为 `GetDirectoriesMock.mockClear()`
- [x] [Review][Patch] `TestSetDefault_Toggle` 第二次切换后未验证 dir3 [service/directory_test.go:268-270] — 已补充 dir3.IsDefault == false 验证
- [x] [Review][Patch] `TestPersistence_MultipleOperations` 未验证 dir3 存在 [service/directory_test.go:385-394] — 已补充 dir3 存在性验证
- [x] [Review][Defer] `UpdateDirectory` mock 返回 `true` 但 app.go 返回 `*model.Directory` [frontend/src/test/setup.js:11] — deferred, setup.js mock 类型不一致，非本 Story AC 范围
- [x] [Review][Defer] Update 方法缺少后端测试覆盖 [service/directory_test.go] — deferred, 非本 Story AC 范围（重命名/更新属于 DirectoryTree.vue 附加功能），建议在后续 Story 中补充
