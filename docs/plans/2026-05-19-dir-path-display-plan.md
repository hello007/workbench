# 工作目录路径显示与面板拖拽调整宽度 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在工作目录列表项名称下方显示完整路径，并将三栏布局改为可拖拽调整宽度

**Architecture:** 两个独立功能。功能一修改 DirectoryTree.vue 模板和样式，在目录项名称下方增加路径行。功能二在 Home.vue 中引入 splitpanes 库替换 el-container 固定宽度布局，实现实时拖拽调整面板宽度。

**Tech Stack:** Vue 3, Element Plus, splitpanes, Vitest, Vue Test Utils

---

### Task 1: 创建 worktree 并安装 splitpanes 依赖

**Files:**
- Modify: `frontend/package.json`

**Step 1: 创建 worktree**

```bash
git worktree add .claude/worktrees/dir-path-display feature/dir-path-display
```

**Step 2: 在 worktree 中安装 splitpanes**

```bash
cd frontend && npm install splitpanes
```

**Step 3: 验证安装成功**

```bash
cd frontend && npm ls splitpanes
```

Expected: `splitpanes@x.x.x` 出现在依赖列表中

**Step 4: Commit**

```bash
git add frontend/package.json frontend/package-lock.json
git commit -m "chore: 安装 splitpanes 依赖"
```

---

### Task 2: DirectoryTree 路径显示 — 编写测试

**Files:**
- Modify: `frontend/src/components/__tests__/DirectoryTree.spec.js`

**Step 1: 在 `目录列表渲染` describe 块中添加路径显示测试**

```javascript
it('应该显示目录路径', () => {
  wrapper = createWrapper()
  const paths = wrapper.findAll('.dir-path')
  expect(paths.length).toBe(2)
  expect(paths[0].text()).toBe('/path/a')
  expect(paths[1].text()).toBe('/path/b')
})

it('路径应该有 title 属性显示完整路径', () => {
  wrapper = createWrapper()
  const paths = wrapper.findAll('.dir-path')
  expect(paths[0].attributes('title')).toBe('/path/a')
})

it('路径样式应该为小字灰色', () => {
  wrapper = createWrapper()
  const paths = wrapper.findAll('.dir-path')
  expect(paths.length).toBeGreaterThan(0)
})
```

**Step 2: 运行测试确认失败**

```bash
cd frontend && npx vitest run src/components/__tests__/DirectoryTree.spec.js
```

Expected: `dir-path` 相关测试 FAIL（元素不存在）

**Step 3: Commit**

```bash
git add frontend/src/components/__tests__/DirectoryTree.spec.js
git commit -m "test: 添加目录路径显示测试（预期失败）"
```

---

### Task 3: DirectoryTree 路径显示 — 实现

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue:11-26`（模板）
- Modify: `frontend/src/components/DirectoryTree.vue:339-375`（样式）

**Step 1: 修改模板 — 重构目录项结构**

将原目录项模板（第11-26行）：

```html
<div
  v-for="dir in directories"
  :key="dir.id"
  class="dir-item"
  :class="{ 'dir-item--active': dir.id === selectedId }"
  @click="handleSelect(dir.id)"
  @contextmenu.prevent="onContextMenu($event, dir)"
>
  <el-icon class="dir-item-icon" color="#909399">
    <Folder />
  </el-icon>
  <span class="dir-item-name" :title="dir.name">{{ dir.name }}</span>
  <el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c">
    <Star />
  </el-icon>
</div>
```

改为：

```html
<div
  v-for="dir in directories"
  :key="dir.id"
  class="dir-item"
  :class="{ 'dir-item--active': dir.id === selectedId }"
  @click="handleSelect(dir.id)"
  @contextmenu.prevent="onContextMenu($event, dir)"
>
  <div class="dir-info">
    <div class="dir-row">
      <el-icon class="dir-item-icon" color="#909399">
        <Folder />
      </el-icon>
      <span class="dir-item-name" :title="dir.name">{{ dir.name }}</span>
      <el-icon v-if="dir.isDefault" class="dir-item-star" color="#e6a23c">
        <Star />
      </el-icon>
    </div>
    <div class="dir-path" :title="dir.path">{{ dir.path }}</div>
  </div>
</div>
```

**Step 2: 修改样式**

将 `.dir-item` 样式改为：

```css
.dir-item {
  padding: 8px 12px;
  cursor: pointer;
  border-left: 3px solid transparent;
  transition: background-color 0.2s ease;
}
```

新增样式：

```css
.dir-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
}

.dir-row {
  display: flex;
  align-items: center;
}

.dir-path {
  font-size: 11px;
  color: #909399;
  line-height: 1.2;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-top: 2px;
  padding-left: 24px;
}
```

**Step 3: 运行测试确认通过**

```bash
cd frontend && npx vitest run src/components/__tests__/DirectoryTree.spec.js
```

Expected: 所有测试 PASS

**Step 4: Commit**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: 目录项名称下方显示完整路径"
```

---

### Task 4: Home.vue 面板拖拽 — 编写测试

**Files:**
- Modify: `frontend/src/views/__tests__/Home.spec.js`

**Step 1: 更新三栏布局测试**

将 `三栏布局验证（AC1）` describe 块中的测试改为验证 splitpanes 布局：

```javascript
describe('splitpanes 三栏布局验证', () => {
  let layoutWrapper

  beforeEach(() => {
    layoutWrapper = mount(Home, {
      global: {
        stubs: {
          Splitpanes: { template: '<div class="splitpanes"><slot /></div>' },
          Pane: { template: '<div class="pane" :data-size="size" :data-min-size="minSize" :data-max-size="maxSize"><slot /></div>', props: ['size', 'minSize', 'maxSize'] },
          DirectoryTree: { template: '<div class="stub-directory-tree" />' },
          FileTreePanel: { template: '<div class="stub-file-tree-panel" />' },
          ContentPanel: { template: '<div class="stub-content-panel" />' }
        }
      }
    })
  })

  afterEach(() => {
    if (layoutWrapper) {
      layoutWrapper.unmount()
      layoutWrapper = null
    }
  })

  it('应该渲染 splitpanes 容器', () => {
    expect(layoutWrapper.find('.splitpanes').exists()).toBe(true)
  })

  it('应该渲染三个 Pane', () => {
    const panes = layoutWrapper.findAll('.pane')
    expect(panes.length).toBe(3)
  })

  it('三个面板应按左-中-右顺序排列', () => {
    const panes = layoutWrapper.findAll('.pane')
    expect(panes[0].find('.stub-directory-tree').exists()).toBe(true)
    expect(panes[1].find('.stub-file-tree-panel').exists()).toBe(true)
    expect(panes[2].find('.stub-content-panel').exists()).toBe(true)
  })

  it('第一个 Pane 尺寸配置正确', () => {
    const panes = layoutWrapper.findAll('.pane')
    expect(panes[0].attributes('data-size')).toBe('15')
    expect(panes[0].attributes('data-min-size')).toBe('10')
    expect(panes[0].attributes('data-max-size')).toBe('30')
  })
})
```

**Step 2: 运行测试确认失败**

```bash
cd frontend && npx vitest run src/views/__tests__/Home.spec.js
```

Expected: splitpanes 相关测试 FAIL

**Step 3: Commit**

```bash
git add frontend/src/views/__tests__/Home.spec.js
git commit -m "test: 添加 splitpanes 布局测试（预期失败）"
```

---

### Task 5: Home.vue 面板拖拽 — 实现

**Files:**
- Modify: `frontend/src/views/Home.vue:1-47`（模板）
- Modify: `frontend/src/views/Home.vue:49-55`（脚本导入）
- Modify: `frontend/src/views/Home.vue:332-348`（样式）

**Step 1: 修改模板 — 替换 el-container 为 splitpanes**

将模板（第1-47行）改为：

```html
<template>
  <div class="home">
    <Splitpanes class="default-theme splitpanes-container">
      <Pane :size="15" :min-size="10" :max-size="30">
        <DirectoryTree
          :directories="directories"
          :selected-id="selectedDirectoryId"
          :version="appVersion"
          @select="onDirectorySelect"
          @change="loadDirectories"
        />
      </Pane>
      <Pane :size="22" :min-size="15" :max-size="35">
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
          @copy-to="handleCopyTo"
        />
      </Pane>
      <Pane :size="63" :min-size="30">
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
          @copy-to="node => fileTreePanelRef.showCopyToDialog(node)"
        />
      </Pane>
    </Splitpanes>
  </div>
</template>
```

**Step 2: 修改脚本导入 — 添加 splitpanes**

在第55行后添加：

```javascript
import { Splitpanes, Pane } from 'splitpanes'
import 'splitpanes/dist/splitpanes.css'
```

**Step 3: 修改样式**

替换原有样式为：

```css
<style scoped>
.home {
  font-family: 'Microsoft YaHei', Arial, sans-serif;
  height: 100vh;
}
.splitpanes-container {
  height: 100%;
}
</style>

<style>
/* splitpanes 全局样式覆盖 */
.splitpanes.default-theme .splitpanes__splitter {
  background-color: #e6e6e6;
  width: 1px !important;
  min-width: 1px !important;
}
.splitpanes.default-theme .splitpanes__splitter:hover {
  background-color: #c0c4cc;
}
.splitpanes.default-theme .splitpanes__pane {
  background-color: #f5f7fa;
}
</style>
```

**Step 4: 运行测试确认通过**

```bash
cd frontend && npx vitest run src/views/__tests__/Home.spec.js
```

Expected: 所有测试 PASS

**Step 5: Commit**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat: 三栏布局改为 splitpanes 可拖拽调整宽度"
```

---

### Task 6: 更新旧布局测试

**Files:**
- Modify: `frontend/src/views/__tests__/Home.spec.js`

**Step 1: 移除或更新旧的 el-container 布局测试**

旧的 `三栏布局验证（AC1）` 测试依赖 `el-aside`、`el-container` 等，改为 Task 4 中的 splitpanes 测试后，旧测试应被替换。

需同时检查 `左侧文件树滚动条` describe 块是否需要更新 stub（`el-aside` 改为 `Pane`）。

**Step 2: 运行全部 Home 测试确认通过**

```bash
cd frontend && npx vitest run src/views/__tests__/Home.spec.js
```

Expected: 所有测试 PASS

**Step 3: Commit**

```bash
git add frontend/src/views/__tests__/Home.spec.js
git commit -m "test: 更新布局测试适配 splitpanes"
```

---

### Task 7: 全量测试与视觉验证

**Step 1: 运行全部前端测试**

```bash
cd frontend && npx vitest run
```

Expected: 所有测试 PASS

**Step 2: 运行全部后端测试**

```bash
go test ./...
```

Expected: 所有测试 PASS

**Step 3: 启动开发环境验证视觉效果**

```bash
wails dev
```

验证点：
- 目录项名称下方显示灰色小字路径
- 路径过长时截断，hover 显示完整路径
- 三个面板可拖拽分隔条调整宽度
- 拖拽实时跟随鼠标
- 分隔条样式为 1px 灰色线

**Step 4: 确认是否需要更新 README.md**

根据 CLAUDE.md 规定，每次功能完成后确认是否需要更新。

---

### Task 8: 合并 worktree 到主分支

**Step 1: 确认 worktree 中所有改动已提交**

```bash
git status
git log --oneline -5
```

**Step 2: 合并到 bmad 分支**

```bash
cd D:/workspace/workspace_ai/demo_OpenSpec/git_tools/workbench
git merge feature/dir-path-display
```

**Step 3: 清理 worktree**

```bash
git worktree remove .claude/worktrees/dir-path-display
git branch -d feature/dir-path-display
```
