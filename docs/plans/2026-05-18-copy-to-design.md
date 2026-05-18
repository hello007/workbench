# 拷贝到功能设计文档

**日期：** 2026-05-18
**状态：** 已批准
**分支：** feature/copy-to

## 概述

新增"拷贝到"功能，支持将文件或文件夹复制到指定目录。用户可通过右键菜单或操作面板按钮打开对话框，输入源路径和目标路径后执行复制操作。

## 后端设计

### 新增方法

**Service 层** — `service/fileoperation.go`：

```go
func (s *FileOperationService) CopyTo(sourcePath, targetPath string, copyWholeDir bool) (string, error)
```

**校验链（按顺序执行）：**

1. `os.Stat(sourcePath)` — 不存在则返回错误
2. `os.Stat(targetPath)`：
   - 是文件 → 返回错误（目标必须是目录）
   - 不存在 → `os.MkdirAll` 自动创建目录
   - 是目录 → 继续
3. 执行拷贝：
   - `copyWholeDir=true` + 源是文件夹 → `CopyItem(sourcePath, targetPath)`，将整个文件夹拷到目标下
   - `copyWholeDir=false` + 源是文件夹 → 列出 sourcePath 下所有子项，逐个 `CopyItem` 到 targetPath
   - 源是文件 → 直接 `CopyItem(sourcePath, targetPath)`

**App 层** — `app.go`：

```go
func (a *App) CopyTo(sourcePath, targetPath string, copyWholeDir bool) string
```

- 成功返回目标路径字符串
- 失败返回 `"错误: xxx"` 格式
- 模式与 `CopyItem`、`MoveItem` 一致

## 前端设计

### 对话框

在 `FileTreePanel.vue` 内新增 `el-dialog`：

```
┌─────────────── 拷贝到 ─────────────────┐
│  原地址:   [ D:/workspace/src        ] │
│  目标地址:  [                          ] │
│  ☑ 对原地址目录整体操作                  │
│  （原地址是文件时禁用）                    │
│          [ 取消 ]  [ 确定 ]              │
└─────────────────────────────────────────┘
```

- 原地址：选中节点时自动填入，可编辑
- 目标地址：用户手动输入
- 复选框：原地址是文件时禁用
- 确定：原地址为空时禁用

### 入口点

1. **右键菜单**：在"复制"和"粘贴"之间插入"拷贝到..."菜单项（目录和文件菜单均添加）
2. **操作面板**：`ContentPanel.vue` 剪切/复制/粘贴按钮组旁新增"拷贝到"按钮

### 数据流

```
FileTreePanel / ContentPanel
  → emit('copyTo', sourceData)
  → Home.vue handleCopyTo(sourceData)
  → 后端 CopyTo(sourcePath, targetPath, copyWholeDir)
  → 刷新文件树 + ElMessage 反馈
```

## 改动文件清单

| 文件 | 改动 |
|------|------|
| `service/fileoperation.go` | 新增 `CopyTo` 方法 |
| `app.go` | 新增 `CopyTo` 绑定方法 |
| `frontend/src/components/FileTreePanel.vue` | 右键菜单加"拷贝到" + 新增对话框 |
| `frontend/src/components/ContentPanel.vue` | 按钮和事件 |
| `frontend/src/views/Home.vue` | 事件处理函数 |
| `frontend/src/test/setup.js` | 新增 `CopyTo` mock |

## 测试

- **后端**：表驱动测试覆盖 6 个场景（文件→已有目录、文件→新建目录、文件夹整体、文件夹内容、原地址不存在、目标是文件）
- **前端**：`setup.js` 添加 mock
