# 三列布局重构设计

**日期：** 2026-05-12
**状态：** 已确认

## 摘要

将工作目录下拉框改为左侧平级树型展示，整体布局从「Header + 两列」重构为「三列填满视口」：工作目录树（200px）| 文件树（280px）| 操作面板（自适应）。

## 设计方案

采用**拆分子组件**方式，将 Home.vue 拆分为四个组件：

```
Home.vue（布局容器 + 状态中枢）
├── DirectoryTree.vue    （左侧：工作目录树，200px）
├── FileTreePanel.vue    （中间：文件树，280px）
└── ContentPanel.vue     （右侧：操作面板，自适应）
```

## 组件职责

| 组件 | 职责 |
|------|------|
| Home.vue | 布局容器，持有核心状态，处理子组件 emit 事件 |
| DirectoryTree | 展示工作目录平级列表，支持添加/删除/重命名/设为默认 |
| FileTreePanel | 展示当前目录的文件树，懒加载、右键菜单 |
| ContentPanel | 展示选中节点详情、Git 信息、操作按钮 |

## 数据流

采用 props down, emit up 模式：

- **Home.vue 持有的核心状态：** directories、selectedDirectoryId、selectedNode、latestCommit
- **DirectoryTree emit：** select、add、remove、rename、setDefault
- **FileTreePanel emit：** select
- **ContentPanel → Home → FileTreePanel：** 通过 ref 调用 expose 方法刷新节点

## UI 布局

```
┌──────────────────────────────────────────────────────┐
│ DirectoryTree │      FileTreePanel      │ ContentPanel│
│    200px      │        280px            │   auto      │
│               │                         │             │
│ ┌───────────┐ │ ┌─────────────────────┐ │ ┌─────────┐ │
│ │ + 添加目录│ │ │ 全部收起            │ │ │ 节点详情│ │
│ ├───────────┤ │ ├─────────────────────┤ │ │         │ │
│ │ ● 工作区A│ │ │ ▶ src/              │ │ │ Git信息 │ │
│ │ ○ 工作区B│ │ │ ▶ docs/             │ │ │         │ │
│ │ ○ 工作区C│ │ │   README.md         │ │ │ 操作按钮│ │
│ │           │ │ │   go.mod            │ │ │         │ │
│ └───────────┘ │ └─────────────────────┘ │ └─────────┘ │
└──────────────────────────────────────────────────────┘
```

**面板样式：**

| 面板 | 背景色 | 边框 |
|------|--------|------|
| DirectoryTree | #f5f7fa | 右侧 1px #e6e6e6 |
| FileTreePanel | #f5f7fa | 右侧 1px #e6e6e6 |
| ContentPanel | #fff | 无 |

## 后端新增方法

| 方法 | 说明 |
|------|------|
| RemoveDirectory(id) | 删除工作目录 |
| RenameDirectory(id, newName) | 重命名工作目录 |
| SetDefaultDirectory(id) | 设为默认目录 |

## 对话框归属

| 对话框 | 归属组件 |
|--------|----------|
| 添加目录 | DirectoryTree |
| 删除/重命名目录 | DirectoryTree |
| 新建文件/文件夹 | FileTreePanel |
| 重命名文件 | FileTreePanel |
| 克隆仓库 | ContentPanel |
| 更新仓库进度 | ContentPanel |

## 迁移步骤

1. 后端新增 3 个方法 → 验证: go test ./... 通过
2. 创建 DirectoryTree.vue → 验证: 工作目录树可展示、可选中
3. 创建 FileTreePanel.vue → 验证: 文件树懒加载、右键菜单正常
4. 创建 ContentPanel.vue → 验证: 节点详情、Git 信息、操作按钮正常
5. 重构 Home.vue 为布局容器 → 验证: 三列布局正确，所有功能完整
6. 移除旧 Header 和下拉框 → 验证: 页面干净无残留

## 不做的事情

- 不改后端现有方法的签名或行为，只新增
- 不改依赖版本
- 不引入新的第三方组件库
- 不改变数据持久化方式
