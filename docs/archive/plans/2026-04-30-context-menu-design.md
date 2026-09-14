# 文件树右键菜单设计文档

## 概述

为 WorkBench 左侧文件树的节点添加右键操作菜单，提供常用文件操作功能。采用 Element Plus `el-dropdown`（`trigger="contextmenu"`）实现，按节点类型区分菜单项。

## 需求总结

- 在文件树节点上右键弹出上下文菜单
- 文件夹节点和文件节点显示不同的菜单项
- 删除操作需要二次确认
- 不包含 Git 相关操作
- 包含基础文件操作 + 实用功能（复制路径等）
- 仅在节点上触发，空白区域无菜单

## 菜单项定义

### 文件夹节点

| 菜单项 | 图标 | 说明 | 后端 API |
|--------|------|------|----------|
| 新建文件 | DocumentAdd | 在该目录下创建文件 | `CreateFile`（已有） |
| 新建文件夹 | FolderAdd | 在该目录下创建子文件夹 | `CreateDirectory`（已有） |
| --- | --- | 分隔线 | --- |
| 重命名 | Edit | 重命名该文件夹 | `RenameFile`（已有） |
| 删除 | Delete | 删除该文件夹（二次确认） | `DeleteFile`（已有） |
| --- | --- | 分隔线 | --- |
| 复制路径 | CopyDocument | 复制完整路径到剪贴板 | 前端实现 |
| 在资源管理器中打开 | Monitor | 用系统资源管理器打开 | `OpenInExplorer`（新增） |

### 文件节点

| 菜单项 | 图标 | 说明 | 后端 API |
|--------|------|------|----------|
| 重命名 | Edit | 重命名该文件 | `RenameFile`（已有） |
| 删除 | Delete | 删除该文件（二次确认） | `DeleteFile`（已有） |
| --- | --- | 分隔线 | --- |
| 复制路径 | CopyDocument | 复制完整路径到剪贴板 | 前端实现 |
| 复制文件名 | DocumentCopy | 仅复制文件名 | 前端实现 |
| 在资源管理器中打开 | Monitor | 用系统资源管理器打开并选中 | `OpenInExplorer`（新增） |

## 交互流程

### 右键触发

```
用户右键点击节点 → el-dropdown 弹出菜单 → 用户点击菜单项 → 执行操作 → 刷新文件树
```

### 各操作交互细节

**新建文件/文件夹：**
- 弹出 `el-dialog` 输入名称
- 输入框自动聚焦，回车确认
- 创建成功后自动刷新该节点的子节点
- 错误提示使用 `ElMessage.error`

**重命名：**
- 弹出 `el-dialog`，输入框预填当前名称且全选
- 回车确认，Escape 取消
- 重命名成功后刷新父节点

**删除：**
- 弹出 `ElMessageBox.confirm` 二次确认
- 确认文案：`确定要删除 "{名称}" 吗？此操作不可撤销。`
- 删除成功后刷新父节点

**复制路径/文件名：**
- 无弹窗，直接复制到剪贴板
- 复制成功后显示 `ElMessage.success("已复制到剪贴板")`

**在资源管理器中打开：**
- Windows 下调用 `explorer /select,"{path}"`（文件）或 `explorer "{path}"`（文件夹）
- 操作失败显示错误提示

### 输入弹窗复用

新建文件、新建文件夹、重命名三个操作共享同一个对话框组件，通过参数区分：
- `title`：对话框标题
- `defaultValue`：输入框默认值（重命名时预填）
- `placeholder`：占位提示文本
- `onConfirm(value)`：确认回调

## 技术实现

### 前端改动（Home.vue）

1. **el-tree 节点内容改造** — 将自定义节点内容包裹在 `el-dropdown` 中

```html
<el-dropdown trigger="contextmenu" @command="handleContextMenu($event, data, node)">
  <!-- 原有的图标+名称渲染 -->
  <template #dropdown>
    <el-dropdown-menu>
      <!-- 根据节点类型条件渲染菜单项 -->
    </el-dropdown-menu>
  </template>
</el-dropdown>
```

2. **输入对话框** — 复用一个 `el-dialog`，通过 `dialogConfig` 响应式对象控制

3. **菜单处理函数** — `handleContextMenu(command, data, node)` 分发到具体操作

### 后端改动

新增 `OpenInExplorer(path string) error`：
- 位置：`service/fileop.go`
- 逻辑：判断 path 是文件还是目录，调用对应的 `explorer` 命令
- 绑定：在 `app.go` 中注册为 Wails 方法

### 不需要改动

- 文件树懒加载逻辑
- 现有 CreateFile、CreateDirectory、RenameFile、DeleteFile API
- Element Plus 默认样式

## 影响范围

| 文件 | 改动类型 | 说明 |
|------|----------|------|
| `frontend/src/views/Home.vue` | 修改 | 节点模板包裹 el-dropdown，新增菜单处理逻辑和对话框 |
| `service/fileop.go` | 新增方法 | `OpenInExplorer` |
| `app.go` | 新增绑定 | 注册 `OpenInExplorer` 为 Wails 方法 |
