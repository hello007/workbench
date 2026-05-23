# DirectoryTree 右键菜单新增打开操作

> **日期：** 2026-05-23
> **状态：** 已确认
> **涉及文件：** `frontend/src/components/DirectoryTree.vue`

## 需求

在左侧工作目录树（DirectoryTree.vue）的右键菜单中新增 3 项"打开"操作：

1. 在资源管理器中打开
2. 用 VSCode 打开
3. 用 Warp 终端打开

后端方法已实现（`OpenInExplorer`、`OpenInVSCode`、`OpenInWarp`），无需后端改动。

## 设计

### 菜单结构

平铺布局，在"设为默认"和"删除"之间插入打开操作：

```
重命名
设为默认
─────────────────
在资源管理器中打开
用 VSCode 打开
用 Warp 打开
─────────────────
删除
```

### 前端改动

仅修改 `DirectoryTree.vue`：

- **模板**：在"设为默认"菜单项后、分隔线前，插入 3 个菜单项
- **Script**：
  - 新增 import：`OpenInExplorer`、`OpenInVSCode`、`OpenInWarp`
  - 新增 import 图标：`FolderOpened`、`Monitor`、`Terminal`
  - `onMenuCommand` 增加 `openExplorer`、`openVSCode`、`openWarp` 三个 case
  - 每个命令调用对应后端方法，传入 `dir.path`

### 后端改动

无。

### 错误处理

调用失败时用 `ElMessage.error` 提示用户。后端已有命令不存在时的错误处理。

## 方案选择

| 方案 | 说明 | 结论 |
|------|------|------|
| A：直接扩展 | 在 DirectoryTree.vue 中追加菜单项 | **采用** |
| B：抽取组件 | 抽取通用 ContextMenu 组件 | 改动大、收益低 |
