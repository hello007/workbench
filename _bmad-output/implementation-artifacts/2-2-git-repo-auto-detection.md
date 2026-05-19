# Story 2.2: Git 仓库自动检测

Status: done

## Story

As a 开发者,
I want 系统自动检测目录是否为 Git 仓库并标识,
so that 我可以一眼识别哪些目录是 Git 项目。

## Acceptance Criteria

1. **AC1 - Git 仓库标识（FR8）**：文件树加载完成后，包含 `.git` 子目录的目录节点显示 Git 仓库标识
2. **AC2 - 检测缓存**：检测结果使用缓存，避免重复检测同一目录
3. **AC3 - 前端展示**：Git 仓库目录在文件树节点上显示绿色对勾图标（`SuccessFilled`，`#67C23A`）
4. **AC4 - 内容面板联动**：选中 Git 仓库目录后，内容面板显示"拉取更新"按钮和 Git 信息标签页（仓库信息 + 提交历史）
5. **AC5 - worktree 检测**：`.git` 为文件（git worktree）时也识别为 Git 仓库

## Tasks / Subtasks

- [x] Task 1: 验证后端 isGitRepoDir 检测逻辑（AC: #1, #2, #5）
  - [x] 1.1 阅读 `service/filetree.go:28-36` 的 `isGitRepoDir` 方法，确认当前实现：`os.Stat(filepath.Join(dir, ".git"))` + `info.IsDir()` 检查
  - [x] 1.2 识别缺陷：当前 `IsDir()` 检查排除了 git worktree（`.git` 为文件），需要修复为同时支持目录和文件
  - [x] 1.3 修复 `isGitRepoDir`：将 `info.IsDir()` 条件改为 `err == nil`（即 `.git` 存在即可，不论目录或文件）
  - [x] 1.4 验证 `GetChildren`（`service/filetree.go:64-66`）中 `isGitRepoDir` 调用仅对 `entry.IsDir()` 为 true 的节点执行
  - [x] 1.5 验证 `sync.Map` 缓存机制：`gitRepoCache.Load/Store` 正确缓存检测结果

- [x] Task 2: 验证前端 Git 仓库标识展示（AC: #1, #3）
  - [x] 2.1 阅读 `FileTreePanel.vue:41-43` 的 `isGitRepo` 图标展示：`<el-icon v-if="data.isGitRepo" color="#67C23A"><SuccessFilled /></el-icon>`
  - [x] 2.2 确认 `SuccessFilled` 图标已正确导入（`FileTreePanel.vue:250`）
  - [x] 2.3 验证 `loadTreeNode`（`FileTreePanel.vue:364-367`）后处理中 `isGitRepo` 字段从后端数据传递到前端

- [x] Task 3: 验证内容面板 Git 仓库联动（AC: #4）
  - [x] 3.1 阅读 `ContentPanel.vue:11-15` 的 `v-if="selectedNode.isGitRepo"` 条件渲染"拉取更新"按钮
  - [x] 3.2 阅读 `ContentPanel.vue:20-35` 的 `el-tabs` 标签页（仓库信息 + 提交历史），确认 `v-if="selectedNode.isGitRepo"` 控制
  - [x] 3.3 确认 `GitInfo.vue` 组件通过 `GetGitRemoteURL` 获取仓库信息，使用 `gitCache.js` 前端缓存

- [x] Task 4: 编写测试（AC: #1-5）
  - [x] 4.1 编写 `TestIsGitRepoDir_WithGitDir` 测试：含 `.git` 目录的目录返回 true
  - [x] 4.2 编写 `TestIsGitRepoDir_WithGitFile_Worktree` 测试：`.git` 为文件（worktree）时返回 true（修复后）
  - [x] 4.3 编写 `TestIsGitRepoDir_NoGit` 测试：无 `.git` 的目录返回 false
  - [x] 4.4 编写 `TestIsGitRepoDir_CacheHit` 测试：第二次调用同一目录应命中缓存（不重复 `os.Stat`）
  - [x] 4.5 编写 `TestGetChildren_IsGitRepoField` 测试：Git 仓库目录节点 `IsGitRepo=true`，普通目录 `IsGitRepo=false`
  - [x] 4.6 在 `FileTreePanel.spec.js` 补充 `isGitRepo` 图标渲染测试：验证 `data.isGitRepo=true` 时显示图标
  - [x] 4.7 在 `FileTreePanel.spec.js` 补充 `loadTreeNode` 中 `isGitRepo` 字段透传测试
  - [x] 4.8 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**Git 仓库自动检测已完整实现并投产。** 本 Story 属于验证+修复性质，确认后端 `isGitRepoDir`、前端 `isGitRepo` 图标、内容面板联动满足 FR8 的所有要求，修复 git worktree 检测缺陷，并补充测试覆盖。

### 现有实现分析

**Go 后端 — 文件树服务：**

- `service/filetree.go:28-36` — `isGitRepoDir(dir)` 方法：
  - `sync.Map` 缓存检查：`gitRepoCache.Load(dir)`，命中直接返回
  - 未命中时：`os.Stat(filepath.Join(dir, ".git"))` 检测
  - **缺陷**：`info.IsDir()` 条件排除了 git worktree（`.git` 为文件而非目录）
  - `gitRepoCache.Store(dir, isRepo)` 写入缓存

- `service/filetree.go:64-66` — `GetChildren` 中调用：
  - 仅对 `entry.IsDir()` 为 true 的节点调用 `isGitRepoDir`
  - 设置 `node.IsGitRepo = s.isGitRepoDir(fullPath)`

**Go 后端 — 数据模型：**

- `model/models.go:45` — `IsGitRepo bool \`json:"isGitRepo"\`` 字段
- `model/models.go:58` — `NewFileTreeNode` 默认 `IsGitRepo: false`

**前端 — 文件树组件：**

- `FileTreePanel.vue:41-43` — Git 图标：
  ```html
  <el-icon v-if="data.isGitRepo" color="#67C23A" style="margin-left: 5px;">
    <SuccessFilled />
  </el-icon>
  ```
- `FileTreePanel.vue:250` — `SuccessFilled` 已从 `@element-plus/icons-vue` 导入

**前端 — 内容面板：**

- `ContentPanel.vue:11-15` — "拉取更新"按钮：`v-if="selectedNode.isGitRepo"`
- `ContentPanel.vue:20-35` — Git 信息标签页：`v-if="selectedNode.isGitRepo"` + `el-tabs`
- `GitInfo.vue` — 使用 `GetGitRemoteURL` API + `gitCache.js` 前端缓存（5 分钟过期）

### 需要修复的缺陷

**isGitRepoDir 不识别 git worktree（Story 2-1 代码审查已记录为 deferred）：**

- 当前代码：`isRepo := err == nil && info.IsDir()`
- 问题：git worktree 的 `.git` 是一个文件（包含 `gitdir:` 指向实际仓库），不是目录
- 修复：`isRepo := err == nil`（即 `.git` 存在即可，不论目录或文件）
- 风险评估：极低。`os.Stat` 对文件和目录都返回 `err == nil`，唯一变化是 worktree 也被识别为 Git 仓库
- 影响范围：仅 `isGitRepoDir` 方法 1 行代码

### 数据契约

```go
type FileTreeNode struct {
    IsGitRepo bool `json:"isGitRepo"` // 是否 Git 仓库
}
```

前端通过 `data.isGitRepo` 布尔值判断，与 Go `json` 标签一致。

### 架构约束

- **app.go 调度层**：`GetFileTree` 方法只调用 `fileTreeSvc.GetChildren`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **sync.Map 缓存**：进程内缓存，无淘汰/过期机制（Story 2-1 已记录为 deferred，不在本 Story 范围）
- **ElTree 刷新**：修改 `node.data` 不触发视图更新，必须使用 `treeNode.loaded = false; treeNode.expand()`

### 前一个 Story 的经验教训（Story 2-1）

1. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock
2. **Home.spec.js mock 路径**：使用 `vi.importMock('../../../wailsjs/go/main/App')` 获取 mock（三级 `../`）
3. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
4. **Go 测试使用 t.TempDir()**：使用真实文件系统
5. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`，本 Story 可复用
6. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`，避免 `data.type` 未定义错误

### 测试注意事项

**Go 测试（filetree_test.go 扩展）：**

- 现有测试：`TestGetChildren_OnlyGitSkipped`（.git 过滤 + 隐藏目录可见）已间接验证 `.git` 不在节点列表中
- 需补充：`isGitRepoDir` 单元测试（含缓存行为验证）、`GetChildren` 中 `IsGitRepo` 字段测试
- worktree 测试：创建 `.git` 文件（非目录）验证识别
- 缓存测试：调用两次 `isGitRepoDir`，验证 `sync.Map` 命中（可通过内部缓存状态间接验证）

**前端测试（FileTreePanel.spec.js 扩展）：**

- 现有 `loadTreeNode` 测试已验证 `isGitRepo` 字段从后端透传
- 需补充：`isGitRepo=true` 节点应渲染 `SuccessFilled` 图标
- 注意：el-tree stub 当前无 slot，测试 `isGitRepo` 图标需要验证数据透传而非 DOM 渲染

### 关键验证点

1. **isGitRepoDir 修复**：`err == nil` 替代 `err == nil && info.IsDir()`，使 worktree 也被识别
2. **缓存语义**：`sync.Map` 存储路径到布尔值的映射，无过期，进程生命周期内有效
3. **数据流**：Go `IsGitRepo` → JSON `isGitRepo` → 前端 `data.isGitRepo` → `v-if` 渲染
4. **内容面板条件**：`selectedNode.isGitRepo` 控制整个 Git 信息区域的显示
5. **GitInfo.vue**：使用独立的 `GetGitRemoteURL` API + `gitCache.js`（前端 5 分钟缓存），与文件树的 `isGitRepoDir` 缓存独立

### References

- [Source: service/filetree.go:28-36] — isGitRepoDir 缓存检测
- [Source: service/filetree.go:39-79] — GetChildren 方法
- [Source: model/models.go:40-62] — FileTreeNode 结构体和构造函数
- [Source: frontend/src/components/FileTreePanel.vue:41-43] — Git 图标展示
- [Source: frontend/src/components/FileTreePanel.vue:250] — SuccessFilled 导入
- [Source: frontend/src/components/FileTreePanel.vue:364-367] — loadTreeNode isGitRepo 透传
- [Source: frontend/src/components/ContentPanel.vue:11-15] — 拉取更新按钮
- [Source: frontend/src/components/ContentPanel.vue:20-35] — Git 信息标签页
- [Source: frontend/src/components/GitInfo.vue] — Git 仓库信息组件
- [Source: frontend/src/utils/gitCache.js] — 前端 Git 信息缓存
- [Source: service/filetree_test.go] — 现有测试
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证后端 isGitRepoDir 检测逻辑：修复 `info.IsDir()` 条件为 `err == nil`，支持 git worktree
- 验证前端 isGitRepo 图标展示：SuccessFilled + #67C23A 颜色
- 验证内容面板 Git 仓库联动：拉取按钮 + Git 信息标签页均由 isGitRepo 控制
- 新增 5 个 Go 测试：WithGitDir、WithGitFile_Worktree、NoGit、CacheHit、IsGitRepoField
- FileTreePanel.spec.js 新增 2 个前端测试：isGitRepo 字段透传（git 仓库 vs 普通目录、文件节点）
- 全量测试通过：Go 全绿，FileTreePanel 10 个前端测试全绿

### File List

- `service/filetree.go` — 修复 isGitRepoDir：移除 `info.IsDir()` 条件，支持 worktree
- `service/filetree_test.go` — 新增 5 个测试
- `frontend/src/components/__tests__/FileTreePanel.spec.js` — 新增 2 个测试
