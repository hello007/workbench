# 使用默认应用程序打开文件

**日期：** 2026-05-11
**状态：** 已批准

## 摘要

在文件右键菜单和右侧"文件操作"面板中，新增"使用默认应用程序打开"按钮，使用 Windows `cmd /c start` 命令调用系统默认程序打开文件。仅对文件生效，不支持文件夹。

## 方案

使用 `exec.Command("cmd", "/c", "start", "", path)`，与现有 `OpenInExplorer`、`OpenInVSCode` 实现风格一致。

## 改动范围

| 文件 | 改动 |
|------|------|
| `service/fileoperation.go` | 新增 `OpenWithDefaultApp(path) error` |
| `app.go` | 新增 `OpenWithDefaultApp(path) bool` 绑定方法 |
| `frontend/src/views/Home.vue` | 右键菜单增加菜单项 + 右侧文件操作区增加按钮 |

## 后端实现

`service/fileoperation.go` 新增方法，校验路径为文件后调用 `cmd /c start "" path`。

## 前端集成

- **右键菜单**：文件节点的"用 VSCode 打开"下方新增"用默认程序打开"
- **右侧面板**：文件操作按钮组新增"打开"按钮
