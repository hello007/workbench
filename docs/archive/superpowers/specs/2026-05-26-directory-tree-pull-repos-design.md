---
date: 2026-05-26
topic: 工作目录树右键菜单新增"更新仓库"
status: approved
---

# 工作目录树右键菜单新增"更新仓库"功能 — 设计文档

## 背景

`FileTreePanel`（文件树）的目录右键菜单中已有"更新仓库"操作，链路为：

```
右键菜单 → emit('batchPull', { path }) → Home.onBatchPull
  → ScanAndPullRepos(path) → ContentPanel.startBatchPull(summary)
```

`DirectoryTree`（左侧工作目录树）的右键菜单尚无该操作。用户在工作目录上需要批量更新仓库时，必须先在文件树展开目录、再右键根节点，操作路径较长。

## 需求

在 `DirectoryTree` 的目录节点右键菜单中新增"更新仓库"项，行为与 `FileTreePanel` 中同名菜单完全一致：扫描该目录下所有 Git 仓库并执行 pull，进度展示在右侧 `ContentPanel`。

## 范围

| 项 | 是否改动 |
|---|---|
| Go 后端（model/service/util/app.go） | ❌ 不改 |
| `frontend/wailsjs/` | ❌ 不改（自动生成，无新绑定） |
| `DirectoryTree.vue` | ✅ 改 |
| `Home.vue` | ✅ 改（仅事件接线） |
| `FileTreePanel.vue` | ❌ 不改 |
| 测试 | ✅ 新增 1 条 DirectoryTree 用例 |

## 设计

### 1. 菜单结构

在 `DirectoryTree.vue` 现有右键菜单的"用 Warp 打开"和原有"分隔线 + 删除"之间插入新区块：

```
重命名
设为默认
───────────
在资源管理器中打开
用 VSCode 打开
用 Warp 打开
───────────
更新仓库          ← 新增
───────────
删除
```

语义分层：动作类 → 打开类 → 操作仓库 → 危险操作。与 `FileTreePanel` 中"打开类 → 刷新 / 更新仓库 → 删除"的分组顺序一致。

### 2. 模板改动（DirectoryTree.vue）

在现有"用 Warp 打开"项之后插入：

```html
<li class="context-menu-divider" />
<li class="context-menu-item" @click="onMenuCommand('pullRepos')">
  <el-icon><Refresh /></el-icon>更新仓库
</li>
```

### 3. 事件分发（DirectoryTree.vue）

`defineEmits` 添加 `'batchPull'`：

```javascript
const emit = defineEmits(['select', 'change', 'contextmenu', 'batchPull'])
```

`onMenuCommand` 的 switch 增加 case：

```javascript
case 'pullRepos':
  emit('batchPull', { path: dir.path })
  break
```

### 4. 图标导入（DirectoryTree.vue）

在现有 `@element-plus/icons-vue` 导入中追加 `Refresh`。

### 5. 接线（Home.vue）

`DirectoryTree` 标签上新增事件监听：

```html
<DirectoryTree
  ...
  @contextmenu="onDirectoryContextMenu"
  @batch-pull="onBatchPull"
/>
```

`onBatchPull` 已存在且兼容，无需修改。

## 数据契约

| 字段 | 来源 | 用途 |
|---|---|---|
| `dir.path` | `model.Directory.Path`（Wails 自动绑定） | `ScanAndPullRepos` 扫描根路径 |
| emit payload | `{ path: dir.path }` | 与 `FileTreePanel` 一致的对象结构 |

`Directory.path` 与 `FileTreePanel` 传出的 `data.path` 语义一致（文件系统绝对路径），后端无需区分调用来源。

## 错误处理

完全复用现有链路的错误处理：

- 路径不存在 / 扫描失败 → `ScanAndPullRepos` 返回 error → `Home.onBatchPull` 已有 `catch`，显示 `ElMessage.warning('未找到任何 Git 仓库')`
- 目录下无 Git 仓库 → 返回空 summary → `ContentPanel.startBatchPull` 已能处理空集

## 测试

在 `frontend/src/components/__tests__/DirectoryTree.spec.js` 新增 1 条用例：

- **场景**：在目录节点上触发右键 → 菜单出现 → 点击"更新仓库"项
- **断言**：组件 emit 了 `batchPull` 事件，payload 为 `{ path: <目标目录的 path> }`，且菜单关闭

无需更新 `frontend/src/test/setup.js`（未引入新 Wails 绑定方法）。

## 风险

| 风险 | 评估 |
|---|---|
| 重复定义 `Refresh` 图标导入 | 低，只需检查现有导入列表 |
| `onBatchPull` 接口变化 | 不存在，按 props/events 解耦，签名稳定 |
| 多面板菜单互相覆盖 | 已由 `onDirectoryContextMenu` / `onFileTreeContextMenu` 互斥关闭机制处理 |

## 不做的事（YAGNI）

- 不抽公共菜单 composable —— 两处 emit 各 3 行代码，抽象成本高于收益
- 不做"工作目录无 git 仓库"前端预校验 —— 后端已处理，避免冗余
- 不复用 FileTreePanel 的菜单组件 —— 两组件职责不同，提取代价大于收益
- 不调整 FileTreePanel 已有菜单顺序

## 验收标准

1. 在工作目录树任一节点右键，菜单显示"更新仓库"项，位置位于"用 Warp 打开"和"删除"之间，被分隔线包围
2. 点击"更新仓库"后，右侧 ContentPanel 显示批量拉取进度，行为与从 FileTreePanel 触发完全一致
3. 路径不存在或无 Git 仓库时，提示"未找到任何 Git 仓库"
4. 触发后右键菜单立即关闭
5. 现有所有测试仍通过；新增 DirectoryTree 测试用例通过
