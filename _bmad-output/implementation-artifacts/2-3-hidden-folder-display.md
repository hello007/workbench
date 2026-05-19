# Story 2.3: 隐藏文件夹显示

Status: done

## Story

As a 开发者,
I want 查看隐藏文件夹（如 `.claude`、`.vscode`）,
so that 我可以访问和操作配置目录。

## Acceptance Criteria

1. **AC1 - 隐藏目录可见（FR9）**：文件树加载完成后，以 `.` 开头的子目录（如 `.claude`、`.vscode`、`.github`）正常显示在文件树中
2. **AC2 - 隐藏文件可见**：以 `.` 开头的文件（如 `.env`、`.gitignore`、`.eslintrc`）正常显示
3. **AC3 - .git 目录始终不显示**：`.git` 目录是唯一被过滤的隐藏目录，不出现在文件树中
4. **AC4 - 隐藏项可操作**：隐藏目录和文件与普通项一样可展开、预览、重命名、删除

## Tasks / Subtasks

- [x] Task 1: 验证后端隐藏项过滤逻辑（AC: #1, #2, #3）
  - [x] 1.1 阅读 `service/filetree.go:48-53` 的 `GetChildren` 过滤逻辑：`if name == ".git" { continue }` — 仅过滤 `.git`，其他 `.` 开头项正常显示
  - [x] 1.2 确认 `os.ReadDir` 返回的条目中包含隐藏目录和隐藏文件（Windows 上 `os.ReadDir` 返回所有条目，不依赖 `.` 前缀过滤）
  - [x] 1.3 验证 `buildTree`（`service/filetree.go:82-108`）递归构建时也使用 `GetChildren`，因此递归遍历同样遵循 `.git` 过滤规则

- [x] Task 2: 验证前端隐藏项展示与操作（AC: #1, #2, #4）
  - [x] 2.1 阅读前端 `loadTreeNode`（`FileTreePanel.vue:340-376`）的后处理逻辑，确认无隐藏项过滤
  - [x] 2.2 验证 ElTree 节点模板（`FileTreePanel.vue:22-45`）中 `data.type` 判断不区分隐藏/非隐藏，统一渲染
  - [x] 2.3 验证右键菜单（`FileTreePanel.vue:144-238`）对隐藏项同样可用（新建、重命名、删除、复制路径等）

- [x] Task 3: 编写测试（AC: #1-4）
  - [x] 3.1 编写 `TestGetChildren_HiddenFiles` 测试：`.env`、`.gitignore` 等隐藏文件可见
  - [x] 3.2 编写 `TestGetChildren_HiddenDirVsGitDir` 测试：`.claude` 可见，`.git` 不可见，同时验证两者共存场景
  - [x] 3.3 编写 `TestGetChildren_NestedHiddenDir` 测试：`buildTree` 递归遍历时，嵌套的隐藏目录（如 `project/.vscode/settings.json`）正常显示
  - [x] 3.4 编写 `TestGetChildren_DotGitFile` 测试：非目录的 `.git` 文件（如 git worktree 场景）也应被过滤（`name == ".git"` 条件对文件同样适用）
  - [x] 3.5 在 `FileTreePanel.spec.js` 补充隐藏项数据透传测试：验证后端返回的隐藏目录/文件节点正确透传到前端
  - [x] 3.6 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**隐藏文件夹显示已完整实现并投产。** 本 Story 属于验证性质，确认后端 `GetChildren` 的过滤逻辑和前端展示满足 FR9 的所有要求，并补充测试覆盖。

### 现有实现分析

**Go 后端 — 过滤逻辑：**

- `service/filetree.go:48-53` — `GetChildren` 循环中的唯一过滤：
  ```go
  if name == ".git" {
      continue
  }
  ```
  这意味着：
  - `.claude`、`.vscode`、`.github` 等隐藏目录 → **正常显示**
  - `.env`、`.gitignore`、`.eslintrc` 等隐藏文件 → **正常显示**
  - `.git` 目录 → **被过滤**（唯一例外）

- `os.ReadDir` 行为：在 Windows 上返回目录中的所有条目（包括隐藏属性和 `.` 前缀），Go 不依赖文件系统隐藏属性

- `service/filetree.go:82-108` — `buildTree` 递归调用 `GetChildren`，因此递归遍历也遵循同样的 `.git` 过滤规则

**现有测试覆盖：**

- `TestGetChildren_OnlyGitSkipped`（`filetree_test.go:72-100`）— 已验证 `.hidden` 目录和 `.dotfile` 文件可见，`.git` 被跳过
- `TestGetChildren_HiddenDirectories`（`filetree_test.go:134-165`）— 已验证 `.claude`、`.vscode` 可见，`.git` 被跳过

### 需要补充的测试场景

1. **隐藏文件（非目录）**：现有测试只覆盖了隐藏目录（`.claude`、`.vscode`）和一个隐藏文件（`.dotfile`），但 `.dotfile` 没有验证 `type` 字段
2. **嵌套隐藏目录**：`buildTree` 递归遍历中隐藏目录的子节点是否正常显示
3. **`.git` 文件过滤**：当 `.git` 是文件而非目录时（如 worktree 根目录），`name == ".git"` 条件仍应生效

### 数据契约

```go
type FileTreeNode struct {
    Name string `json:"name"` // 文件/目录名，包含 . 前缀
    Type string `json:"type"` // "directory" | "file"
}
```

### 架构约束

- **app.go 调度层**：`GetFileTree` 方法只调用 `fileTreeSvc.GetChildren`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **ElTree 刷新**：修改 `node.data` 不触发视图更新，必须使用 `treeNode.loaded = false; treeNode.expand()`

### 前一个 Story 的经验教训（Story 2-2）

1. **isGitRepoDir 修复**：`err == nil` 替代 `err == nil && info.IsDir()`，支持 worktree
2. **Go 测试使用 t.TempDir()**：使用真实文件系统
3. **mustMkdir/mustWriteFile helpers**：已定义在 `service/filetree_test.go`，本 Story 可复用
4. **el-tree stub 无 slot**：`template: '<div class="el-tree"></div>'`，避免 `data.type` 未定义错误
5. **setup.js mock 必须同步更新**：新增 Go 绑定方法后，必须在 setup.js 添加对应 mock

### 前一个 Story 的经验教训（Story 2-1）

1. **Home.spec.js mock 路径**：使用 `vi.importMock('../../../wailsjs/go/main/App')` 获取 mock（三级 `../`）
2. **mockClear 优于 restoreAllMocks**：避免破坏 setup.js 全局 mock
3. **代码审查修复**：测试中需验证所有关键状态，不能只验证部分

### 测试注意事项

**Go 测试（filetree_test.go 扩展）：**

- 现有测试已覆盖基本场景（`.hidden` 可见、`.git` 跳过），但不够全面
- 需补充：隐藏文件（`.env`、`.gitignore`）的 type 字段验证、嵌套隐藏目录、`.git` 文件过滤
- `buildTree` 递归测试：验证 `GetTree` 方法中嵌套隐藏目录的子节点正常返回

**前端测试（FileTreePanel.spec.js 扩展）：**

- 验证后端返回的隐藏目录/文件节点（`name: '.claude'`、`name: '.env'`）正确透传到前端 resolve
- 注意：el-tree stub 当前无 slot，测试数据透传而非 DOM 渲染

### 关键验证点

1. **过滤精准性**：仅 `name == ".git"` 被过滤，其他所有 `.` 开头项正常显示
2. **文件 vs 目录**：`.git` 文件（如 worktree 中）也应被过滤 — `name == ".git"` 不区分类型
3. **递归一致性**：`buildTree` 递归遍历时，所有层级的隐藏项都遵循同一过滤规则
4. **前端无额外过滤**：前端 `loadTreeNode` 后处理不添加任何隐藏项过滤逻辑

### References

- [Source: service/filetree.go:48-53] — GetChildren .git 过滤逻辑
- [Source: service/filetree.go:82-108] — buildTree 递归方法
- [Source: service/filetree_test.go:72-100] — TestGetChildren_OnlyGitSkipped
- [Source: service/filetree_test.go:134-165] — TestGetChildren_HiddenDirectories
- [Source: frontend/src/components/FileTreePanel.vue:340-376] — loadTreeNode 后处理
- [Source: frontend/src/components/FileTreePanel.vue:22-45] — ElTree 节点模板
- [Source: frontend/src/components/FileTreePanel.vue:144-238] — 右键菜单
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证后端 GetChildren 过滤逻辑：仅 `name == ".git"` 被过滤，其他 `.` 开头项正常显示
- 验证 buildTree 递归遍历同样遵循 `.git` 过滤规则
- 验证前端 loadTreeNode 无额外隐藏项过滤，右键菜单对隐藏项同样可用
- 新增 4 个 Go 测试：HiddenFiles（含 type 验证）、HiddenDirVsGitDir、NestedHiddenDir（递归）、DotGitFile（文件也被过滤）
- FileTreePanel.spec.js 新增 1 个前端测试：隐藏目录和隐藏文件数据透传
- 全量测试通过：Go 全绿，FileTreePanel 11 个前端测试全绿

### File List

- `service/filetree_test.go` — 新增 4 个测试
- `frontend/src/components/__tests__/FileTreePanel.spec.js` — 新增 1 个测试
