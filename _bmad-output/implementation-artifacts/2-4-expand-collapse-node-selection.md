# Story 2.4: 全部展开/收起与节点选中

Status: done

## Story

As a 开发者,
I want 全部展开或收起文件树，以及选中节点查看信息,
so that 我可以快速定位文件或概览目录结构。

## Acceptance Criteria

1. **AC1 - 全部展开（FR10）**：用户点击"全部展开"按钮后，文件树递归展开所有目录节点，展开期间按钮显示 loading 状态，完成后提示"已全部展开"
2. **AC2 - 全部收起（FR10）**：用户点击"全部收起"按钮后，文件树收起至根节点
3. **AC3 - 节点选中（FR11）**：用户点击文件或文件夹节点，节点高亮选中，右侧面板展示节点信息

## Tasks / Subtasks

- [x] Task 1: 验证全部展开功能（AC: #1）
  - [x] 1.1 阅读 `FileTreePanel.vue:401-429` 的 `expandAll` 方法，确认递归展开逻辑：`node.expand(callback)` + `Promise.all` 递归子节点
  - [x] 1.2 验证 `expanding` ref 控制 loading 状态：`<el-button :loading="expanding">`
  - [x] 1.3 验证展开完成后 `ElMessage.success('已全部展开')` 提示
  - [x] 1.4 验证 `expandNode` 函数对 `isLeaf` 节点跳过展开
  - [x] 1.5 验证错误处理：try-catch 包裹，失败时 `ElMessage.error`

- [x] Task 2: 验证全部收起功能（AC: #2）
  - [x] 2.1 阅读 `FileTreePanel.vue:432-443` 的 `collapseAll` 方法，确认遍历 `nodesMap` 设置 `node.expanded = false`
  - [x] 2.2 验证收起完成后 `ElMessage.success('已全部收起')` 提示
  - [x] 2.3 验证 `fileTreeRef.value` 空值保护

- [x] Task 3: 验证节点选中与信息联动（AC: #3）
  - [x] 3.1 阅读 `FileTreePanel.vue:379-381` 的 `onNodeClick` 方法，确认 `emit('select', data)` 向父组件传递节点数据
  - [x] 3.2 阅读 `Home.vue:114-117` 的 `onNodeSelect` 处理，确认 `selectedNode.value = data` 触发 ContentPanel 更新
  - [x] 3.3 阅读 `ContentPanel.vue:3-8`，确认节点信息展示（名称、路径、类型）
  - [x] 3.4 验证 ElTree `@node-click` 事件绑定（`FileTreePanel.vue:18`）
  - [x] 3.5 验证 ElTree `node-key="path"` 配置（`FileTreePanel.vue:15`）支持节点高亮

- [x] Task 4: 编写测试（AC: #1-3）
  - [x] 4.1 编写 `expandAll` 方法测试：验证递归展开所有非叶节点、loading 状态、成功提示
  - [x] 4.2 编写 `collapseAll` 方法测试：验证所有节点收起、成功提示
  - [x] 4.3 编写 `onNodeClick` 测试：验证 emit 'select' 事件携带节点数据
  - [x] 4.4 编写 ContentPanel 节点信息展示测试：验证选中节点后显示名称、路径、类型
  - [x] 4.5 运行全量测试确认无回归

## Dev Notes

### 棕地项目背景

**全部展开/收起与节点选中功能已完整实现并投产。** 本 Story 属于验证性质，确认 `expandAll`、`collapseAll`、`onNodeClick` 满足 FR10 和 FR11 的所有要求，并补充测试覆盖。

### 现有实现分析

**前端 — 全部展开：**

- `FileTreePanel.vue:401-429` — `expandAll` 方法：
  ```javascript
  const expanding = ref(false)
  const expandAll = async () => {
    if (!fileTreeRef.value) return
    expanding.value = true
    try {
      const expandNode = (node) => {
        return new Promise(resolve => {
          if (node.isLeaf) { resolve(); return }
          node.expand(() => {
            Promise.all(node.childNodes.map(child => expandNode(child))).then(resolve)
          })
        })
      }
      const root = fileTreeRef.value.store.root
      await Promise.all(root.childNodes.map(child => expandNode(child)))
      ElMessage.success('已全部展开')
    } catch (error) {
      ElMessage.error('展开失败: ' + (error.message || String(error)))
    } finally {
      expanding.value = false
    }
  }
  ```
  - `expanding` ref 控制 loading 状态
  - 递归调用 `node.expand(callback)`，callback 在子节点加载完成后触发
  - `Promise.all` 等待所有子节点展开完成
  - 叶节点跳过展开

- `FileTreePanel.vue:6` — loading 绑定：`<el-button size="small" @click="expandAll" :loading="expanding">全部展开</el-button>`

**前端 — 全部收起：**

- `FileTreePanel.vue:432-443` — `collapseAll` 方法：
  ```javascript
  const collapseAll = () => {
    if (!fileTreeRef.value) return
    const allNodes = fileTreeRef.value.store.nodesMap
    Object.keys(allNodes).forEach(key => {
      const node = allNodes[key]
      if (node.expanded) { node.expanded = false }
    })
    ElMessage.success('已全部收起')
  }
  ```
  - 遍历 `nodesMap` 将所有 `expanded` 节点设为 `false`
  - 根节点（root）不在 `nodesMap` 中，无需处理

**前端 — 节点选中与信息联动：**

- `FileTreePanel.vue:379-381` — `onNodeClick`：
  ```javascript
  const onNodeClick = (data) => { emit('select', data) }
  ```

- `FileTreePanel.vue:18` — ElTree 事件绑定：`@node-click="onNodeClick"`

- `Home.vue:114-117` — 选中处理：
  ```javascript
  const onNodeSelect = (data) => {
    selectedNode.value = data
    contentPanelRef.value?.clearPreview()
  }
  ```

- `ContentPanel.vue:3-8` — 节点信息展示：
  ```html
  <h2>{{ selectedNode.name }}</h2>
  <el-descriptions :column="2" border>
    <el-descriptions-item label="路径">{{ selectedNode.path }}</el-descriptions-item>
    <el-descriptions-item label="类型">{{ selectedNode.type === 'directory' ? '文件夹' : '文件' }}</el-descriptions-item>
  </el-descriptions>
  ```

- ElTree `node-key="path"` 配置支持节点高亮选中（Element Plus 通过 `current-node-key` 控制）

**后端 — GetTree 递归获取：**

- `service/filetree.go:82-108` — `GetTree`/`buildTree`：递归获取完整树结构，`maxDepth` 控制深度
- `app.go:121-128` — `GetFileTreeRecursive`：Wails 绑定方法，调用 `fileTreeSvc.GetTree`
- 注意：`expandAll` 使用 ElTree 的懒加载机制（`node.expand`），不依赖 `GetFileTreeRecursive`

### 数据流

```
全部展开:
  用户点击按钮 → expandAll() → 递归 node.expand(callback)
  → 每个非叶节点触发 loadTreeNode → GetFileTree(path)
  → 所有目录展开 → ElMessage.success('已全部展开')

全部收起:
  用户点击按钮 → collapseAll() → 遍历 nodesMap 设置 expanded=false
  → 所有节点收起 → ElMessage.success('已全部收起')

节点选中:
  用户点击节点 → @node-click → onNodeClick(data) → emit('select', data)
  → Home.onNodeSelect(data) → selectedNode.value = data
  → ContentPanel 响应式更新 → 显示节点名称、路径、类型
```

### 数据契约

```go
type FileTreeNode struct {
    Name string `json:"name"` // 文件/目录名
    Path string `json:"path"` // 完整路径（也是 node-key）
    Type string `json:"type"` // "directory" | "file"
}
```

### 架构约束

- **app.go 调度层**：`GetFileTree` 方法只调用 `fileTreeSvc.GetChildren`，≤10 行
- **禁止**：引入新依赖、TypeScript、CSS 框架
- **禁止**：在 service 中导入 Wails 包
- **ElTree 刷新**：修改 `node.data` 不触发视图更新，必须使用 `treeNode.loaded = false; treeNode.expand()`
- **ElTree store**：`fileTreeRef.value.store.root` 访问根节点，`store.nodesMap` 访问所有节点

### 前一个 Story 的经验教训（Story 2-3）

1. **隐藏项无额外过滤**：后端 `GetChildren` 仅过滤 `.git`，前端 `loadTreeNode` 不添加任何过滤
2. **嵌套隐藏目录**：`buildTree` 递归遍历时所有层级都遵循同一过滤规则
3. **`.git` 文件也被过滤**：`name == ".git"` 不区分文件/目录类型

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

**前端测试（FileTreePanel.spec.js 扩展）：**

- `expandAll` 测试需要模拟 ElTree store 结构：`fileTreeRef.value.store.root` 返回根节点
- 由于 el-tree stub 是简单 `<div>`，无法直接测试 `expand()` 方法，需要验证方法调用逻辑
- 替代方案：直接调用 `wrapper.vm.expandAll()`，mock `fileTreeRef.value` 的 store 结构
- `collapseAll` 同理，需要 mock `nodesMap`
- `onNodeClick` 测试：调用 `wrapper.vm.onNodeClick(data)` 验证 emit

**前端测试（ContentPanel.spec.js 新增或扩展）：**

- 验证 `selectedNode` prop 传入后，节点名称、路径、类型正确显示
- 需要检查是否已有 ContentPanel 测试文件

### 关键验证点

1. **展开递归性**：`expandAll` 递归展开所有层级目录节点，而非仅一级
2. **loading 防护**：`expanding` ref 在展开期间为 true，按钮显示 loading 状态，防止重复点击
3. **收起彻底性**：`collapseAll` 收起所有节点（包括深层嵌套），不仅是根级
4. **选中联动**：点击节点 → emit → Home 更新 selectedNode → ContentPanel 显示信息
5. **信息展示完整性**：显示节点名称、路径、类型，Git 仓库额外显示拉取按钮和信息标签页
6. **ElTree node-key**：`node-key="path"` 确保节点可被唯一标识和高亮

### References

- [Source: frontend/src/components/FileTreePanel.vue:401-429] — expandAll 方法
- [Source: frontend/src/components/FileTreePanel.vue:432-443] — collapseAll 方法
- [Source: frontend/src/components/FileTreePanel.vue:379-381] — onNodeClick 方法
- [Source: frontend/src/components/FileTreePanel.vue:6] — 全部展开按钮 loading 绑定
- [Source: frontend/src/components/FileTreePanel.vue:7] — 全部收起按钮
- [Source: frontend/src/components/FileTreePanel.vue:15-18] — ElTree 配置（node-key, lazy, load, node-click）
- [Source: frontend/src/views/Home.vue:114-117] — onNodeSelect 处理
- [Source: frontend/src/components/ContentPanel.vue:3-8] — 节点信息展示
- [Source: service/filetree.go:82-108] — GetTree/buildTree 递归方法
- [Source: app.go:121-128] — GetFileTreeRecursive Wails 绑定
- [Source: frontend/src/components/__tests__/FileTreePanel.spec.js] — 现有前端测试
- [Source: frontend/src/test/setup.js] — 全局 mock 配置

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7

### Debug Log References

### Completion Notes List

- 验证 expandAll 递归展开逻辑：Promise-based node.expand(callback) + Promise.all 递归子节点，isLeaf 跳过
- 验证 expanding ref 控制 loading 状态：`<el-button :loading="expanding">`
- 验证 collapseAll 遍历 nodesMap 设置 expanded=false
- 验证 onNodeClick emit('select', data) 向 Home.vue 传递节点数据
- 验证 ContentPanel 节点信息展示：名称(h2)、路径、类型（文件/文件夹）
- FileTreePanel.spec.js 新增 4 个前端测试：expandAll 成功/失败、collapseAll、onNodeClick
- ContentPanel.spec.js 新增 3 个前端测试：文件节点信息、文件夹节点信息、未选中状态
- 全量测试通过：Go 74 个全绿，前端 18 个全绿（FileTreePanel 14 + ContentPanel 3 + Home 1 通过）

### File List

- `frontend/src/components/__tests__/FileTreePanel.spec.js` — 新增 4 个测试
- `frontend/src/components/__tests__/ContentPanel.spec.js` — 新建，3 个测试
