# 右键菜单"用 VSCode 打开"功能设计

**日期**：2026-05-09
**状态**：已确认
**方案**：方案 A — 直接调用 `code` 命令

---

## 摘要

在文件树右键菜单中新增"用 VSCode 打开"选项，支持对文件和文件夹分别调用 `code` 命令。

## 需求

- 右键菜单新增"用 VSCode 打开"选项
- 文件夹：执行 `code <path>`，VSCode 以项目方式打开该文件夹
- 文件：执行 `code <path>`，VSCode 打开该文件
- 仅支持 VSCode，不做多编辑器扩展

## 设计

### 后端变更

#### 1. `service/fileoperation.go` — 新增 `OpenInVSCode` 方法

```go
func (s *FileOperationService) OpenInVSCode(path string) error {
    cmd := exec.Command("code", path)
    util.HideCommandWindow(cmd)
    return cmd.Start()
}
```

- 不区分文件/文件夹，`code` 命令本身支持两种类型
- 使用 `HideCommandWindow` 隐藏命令行窗口（Windows 平台）

#### 2. `app.go` — 新增 `OpenInVSCode` 绑定方法

```go
func (a *App) OpenInVSCode(path string) bool {
    err := a.fileOpSvc.OpenInVSCode(path)
    if err != nil {
        println("Error:", err.Error())
        return false
    }
    return true
}
```

- 与现有 `OpenInExplorer` 方法模式一致

### 前端变更

#### 3. `frontend/src/views/Home.vue`

**右键菜单模板**：在文件和文件夹的右键菜单中都添加"用 VSCode 打开"选项，放置在"在资源管理器中打开"之后。

```html
<li class="context-menu-item" @click="onMenuCommand('openInVSCode')">
  <el-icon><EditPen /></el-icon>用 VSCode 打开
</li>
```

**菜单命令处理**：在 `onMenuCommand` 的 switch 中增加 `openInVSCode` case，调用后端方法。

```javascript
case 'openInVSCode':
    handleOpenInVSCode(data.path)
    break
```

**新增处理函数**：

```javascript
const handleOpenInVSCode = async (path) => {
    try {
        const result = await OpenInVSCode(path)
        if (!result) {
            ElMessage.error('打开 VSCode 失败，请确认已安装 VSCode 并将 code 命令加入 PATH')
        }
    } catch (error) {
        ElMessage.error('打开 VSCode 失败: ' + (error.message || String(error)))
    }
}
```

## 变更文件清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `service/fileoperation.go` | 新增方法 | `OpenInVSCode(path)` |
| `app.go` | 新增方法 | `OpenInVSCode(path) bool` |
| `frontend/src/views/Home.vue` | 修改 | 右键菜单新增选项 + 命令处理 |
