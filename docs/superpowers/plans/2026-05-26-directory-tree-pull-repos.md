# 工作目录树右键新增"更新仓库" — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `DirectoryTree.vue` 的目录右键菜单中新增"更新仓库"项，复用现有 `Home.onBatchPull` → `ScanAndPullRepos` 链路。

**Architecture:** 纯前端改动。`DirectoryTree` 通过新增 `batchPull` emit 把目录路径上抛；`Home.vue` 把已存在的 `onBatchPull` 处理器接到 `DirectoryTree` 上。无需 Go 后端改动，无需新增 Wails 绑定。

**Tech Stack:** Vue 3 (Composition API + `<script setup>`), Element Plus, Vitest + Vue Test Utils.

**Spec:** `docs/superpowers/specs/2026-05-26-directory-tree-pull-repos-design.md`

---

## 文件结构

| 文件 | 操作 | 责任 |
|---|---|---|
| `frontend/src/components/DirectoryTree.vue` | 修改 | 新增菜单项 + emit 上抛 + 引入 Refresh 图标 |
| `frontend/src/views/Home.vue` | 修改 | 在 `<DirectoryTree>` 上接 `@batch-pull="onBatchPull"` |
| `frontend/src/components/__tests__/DirectoryTree.spec.js` | 修改 | 新增 1 条用例验证点击菜单后 emit `batchPull` |

---

## Task 1: 测试驱动 — 为"更新仓库"菜单项写失败测试

**Files:**
- Modify: `frontend/src/components/__tests__/DirectoryTree.spec.js`（在文件末尾 `describe('版本号显示', ...)` 之后追加新 `describe` 块）

- [ ] **Step 1: 在 stubs 中注册 Refresh 图标**

修改 `defaultStubs` 对象（约第 33-59 行），在 `Delete` 后追加一项：

```javascript
const defaultStubs = {
  // ... 已有内容保持不变
  Folder: { template: '<span>folder</span>' },
  Star: { template: '<span>star</span>' },
  Plus: { template: '<span>plus</span>' },
  Edit: { template: '<span>edit</span>' },
  Delete: { template: '<span>del</span>' },
  Refresh: { template: '<span>refresh</span>' }
}
```

- [ ] **Step 2: 追加测试用例**

在 `DirectoryTree.spec.js` 的 `describe('DirectoryTree.vue', ...)` 内、`describe('版本号显示', ...)` 之后追加：

```javascript
  describe('更新仓库', () => {
    it('点击"更新仓库"菜单项应该 emit batchPull 携带目录 path', async () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      await items[1].trigger('contextmenu', { clientX: 10, clientY: 10 })

      const menuItems = wrapper.findAll('.context-menu-item')
      const pullItem = menuItems.find(el => el.text().includes('更新仓库'))
      expect(pullItem).toBeTruthy()

      await pullItem.trigger('click')

      expect(wrapper.emitted('batchPull')).toBeTruthy()
      expect(wrapper.emitted('batchPull')[0][0]).toEqual({ path: '/path/b' })
    })

    it('点击"更新仓库"后菜单应该关闭', async () => {
      wrapper = createWrapper()
      const items = wrapper.findAll('.dir-item')
      await items[0].trigger('contextmenu', { clientX: 10, clientY: 10 })

      const menuItems = wrapper.findAll('.context-menu-item')
      const pullItem = menuItems.find(el => el.text().includes('更新仓库'))
      await pullItem.trigger('click')

      expect(wrapper.find('.context-menu').exists()).toBe(false)
    })
  })
```

- [ ] **Step 3: 运行测试确认失败**

```bash
cd frontend && npm test -- --run src/components/__tests__/DirectoryTree.spec.js
```

预期：新增的 2 条测试 FAIL（菜单中找不到"更新仓库"项，`pullItem` 为 undefined）。其它已有失败（`mousedown` vs `click` 监听器）属于预存在问题，不影响本计划。

- [ ] **Step 4: 提交失败测试**

```bash
git add frontend/src/components/__tests__/DirectoryTree.spec.js
git commit -m "test: DirectoryTree 增加更新仓库菜单失败测试"
```

---

## Task 2: 在 DirectoryTree 中实现"更新仓库"菜单项

**Files:**
- Modify: `frontend/src/components/DirectoryTree.vue`

- [ ] **Step 1: 引入 Refresh 图标**

修改约第 127 行的 import 语句，在末尾追加 `Refresh`：

```javascript
import { Folder, Star, Plus, Edit, Delete, FolderOpened, Monitor, EditPen, Promotion, Refresh } from '@element-plus/icons-vue'
```

- [ ] **Step 2: 模板新增菜单项**

在右键菜单（约第 52-79 行）的"用 Warp 打开"项后、原有 "── 分隔线 ── + 删除" 之前插入新区块。

修改前（第 72-78 行）：

```html
      <li class="context-menu-item" @click="onMenuCommand('openWarp')">
        <el-icon><Promotion /></el-icon>用 Warp 打开
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
      </li>
```

修改后：

```html
      <li class="context-menu-item" @click="onMenuCommand('openWarp')">
        <el-icon><Promotion /></el-icon>用 Warp 打开
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('pullRepos')">
        <el-icon><Refresh /></el-icon>更新仓库
      </li>
      <li class="context-menu-divider" />
      <li class="context-menu-item" @click="onMenuCommand('delete')">
        <el-icon><Delete /></el-icon>删除
      </li>
```

- [ ] **Step 3: 在 defineEmits 中追加 'batchPull'**

修改约第 146 行：

```javascript
const emit = defineEmits(['select', 'change', 'contextmenu', 'batchPull'])
```

- [ ] **Step 4: 在 onMenuCommand 的 switch 中新增分支**

修改约第 245-269 行的 `onMenuCommand` 函数。在 `case 'delete':` 之前插入：

```javascript
    case 'pullRepos':
      emit('batchPull', { path: dir.path })
      break
```

完整 switch 块应为：

```javascript
  switch (command) {
    case 'rename':
      showRenameDialog(dir)
      break
    case 'setDefault':
      handleSetDefault(dir)
      break
    case 'openExplorer':
      handleOpenExplorer(dir.path)
      break
    case 'openVSCode':
      handleOpenVSCode(dir.path)
      break
    case 'openWarp':
      handleOpenWarp(dir.path)
      break
    case 'pullRepos':
      emit('batchPull', { path: dir.path })
      break
    case 'delete':
      handleDelete(dir)
      break
  }
```

- [ ] **Step 5: 运行测试确认通过**

```bash
cd frontend && npm test -- --run src/components/__tests__/DirectoryTree.spec.js
```

预期：本次新增的 2 条"更新仓库"测试 PASS。原有 1 条预存在失败（`mousedown` vs `click`）保持不变。

- [ ] **Step 6: 提交实现**

```bash
git add frontend/src/components/DirectoryTree.vue
git commit -m "feat: 工作目录树右键新增更新仓库菜单项"
```

---

## Task 3: Home.vue 接线 batchPull 事件

**Files:**
- Modify: `frontend/src/views/Home.vue`

- [ ] **Step 1: 在 DirectoryTree 标签上接事件**

修改第 7-15 行的 `<DirectoryTree>` 标签，在 `@contextmenu="onDirectoryContextMenu"` 后追加 `@batch-pull="onBatchPull"`。

修改后：

```html
            <DirectoryTree
              ref="directoryTreeRef"
              :directories="directories"
              :selected-id="selectedDirectoryId"
              :version="appVersion"
              @select="onDirectorySelect"
              @change="loadDirectories"
              @contextmenu="onDirectoryContextMenu"
              @batch-pull="onBatchPull"
            />
```

- [ ] **Step 2: 运行所有前端测试确认无回归**

```bash
cd frontend && npm test -- --run
```

预期：本计划新增的 2 条测试 PASS；预存在的 2 条失败（DirectoryTree 和 FileTreePanel 的 click/mousedown 测试）保持不变；其他测试保持原状。

- [ ] **Step 3: 启动 wails dev 做视觉验证**

```bash
wails dev
```

打开应用后手动验证：

1. 在工作目录树某项上右键 → 菜单显示"更新仓库"，位置在"用 Warp 打开"和"删除"之间，被分隔线包围
2. 点击"更新仓库"后右键菜单消失，右侧 ContentPanel 进入批量拉取进度
3. 在不存在的目录上验证：删除该目录或改成无效路径前不能直接验证；可使用一个不含 git 仓库的空目录，应弹出"未找到任何 Git 仓库"提示
4. 同时验证从 FileTreePanel 触发"更新仓库"行为不变

按 Ctrl+C 停止 wails dev。

- [ ] **Step 4: 提交接线**

```bash
git add frontend/src/views/Home.vue
git commit -m "feat: Home.vue 接线 DirectoryTree 的 batchPull 事件"
```

---

## Task 4: 更新文档与上下文

**Files:**
- Modify: `docs/功能说明.md`（如有相关章节）
- Modify: `_bmad-output/project-context.md`（如有需要补充的规则）
- Optional: `README.md`（按 CLAUDE.md 项目要求确认是否需要）

- [ ] **Step 1: 检查 docs/功能说明.md 是否包含"工作目录右键操作"章节**

```bash
grep -n "工作目录\|右键\|更新仓库" docs/功能说明.md
```

- [ ] **Step 2: 若有，则补充"更新仓库"条目；若无，跳过**

仅在文档已经明确列出 DirectoryTree 右键菜单项时追加"更新仓库"。否则不动。

- [ ] **Step 3: 询问用户是否需要更新 README.md**

按 `CLAUDE.md` 项目约定，每个功能完成后确认。

- [ ] **Step 4: 提交文档变更（如有）**

```bash
git add docs/功能说明.md
git commit -m "docs: 工作目录树右键新增更新仓库说明"
```

如无变更则跳过此步。

---

## Task 5: 合并到 master 并推送

- [ ] **Step 1: 确认当前分支与状态**

```bash
git status
git log --oneline -5
```

确认无未提交改动。

- [ ] **Step 2: 推送到远程**

```bash
git push
```

---

## 验收清单

完成后逐项核对：

- [ ] `DirectoryTree.vue` 右键菜单显示"更新仓库"项，位置正确（用 Warp 打开 → 分隔线 → 更新仓库 → 分隔线 → 删除）
- [ ] 点击"更新仓库"，ContentPanel 显示批量拉取进度
- [ ] 路径不存在或无 Git 仓库时，提示"未找到任何 Git 仓库"
- [ ] 触发后右键菜单立即关闭
- [ ] 新增 2 条 DirectoryTree 测试通过
- [ ] 其他原本通过的测试保持通过（预存在的 2 条失败属于历史问题，不在本计划范围）
- [ ] FileTreePanel 的"更新仓库"行为未受影响
