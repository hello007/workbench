# 工作目录拖拽排序设计

## 概述

支持用户通过拖拽调整工作目录在列表中的显示顺序，排序结果持久化到配置文件。

## 方案

前端使用 `vuedraggable`（Vue 3 版本）实现拖拽交互，拖拽结束后调用新增的后端方法将新顺序持久化到 `data/directories.json`。JSON 数组的元素顺序即代表排序，无需新增数据字段。

## 后端改动

### DirectoryService.Reorder

```go
func (s *DirectoryService) Reorder(ids []string) error
```

- 接收按新顺序排列的 id 列表
- 加载现有目录，按 id 重排数组
- id 数量与实际不符时返回错误

### App.ReorderDirectories

```go
func (a *App) ReorderDirectories(ids []string) bool
```

- 绑定方法，前端调用入口

### 改动文件

- `service/directory.go`
- `app.go`

## 前端改动

- 安装 `vuedraggable`（Vue 3 版本）
- `DirectoryTree.vue` 中用 `<draggable>` 组件包裹现有 `v-for` 列表
- 拖拽结束（`@end`）时提取排序后的 id 数组，调用 `ReorderDirectories(ids)`
- 整个目录项可拖拽，无额外手柄
- 使用 `vuedraggable` 内置的占位符和阴影视觉反馈

### 改动文件

- `frontend/package.json`
- `frontend/src/components/DirectoryTree.vue`

## 错误处理

- 后端校验 id 数量与实际目录数一致，不一致返回错误
- 前端调用失败时 `ElMessage.error` 提示，并重新加载目录列表回滚本地状态

## 测试

- **后端**：`service/directory_test.go` 新增 `TestReorder`，覆盖正常排序、id 缺失、id 重复
- **前端**：验证拖拽后调用 `ReorderDirectories` 传入正确 id 顺序

## 影响范围

| 文件 | 改动 |
|------|------|
| `service/directory.go` | 新增 `Reorder` 方法 |
| `app.go` | 新增 `ReorderDirectories` 绑定 |
| `frontend/package.json` | 新增 `vuedraggable` 依赖 |
| `frontend/src/components/DirectoryTree.vue` | 用 `<draggable>` 包裹列表 |
| `service/directory_test.go` | 新增排序测试 |
