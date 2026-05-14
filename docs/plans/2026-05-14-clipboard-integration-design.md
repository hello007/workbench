# Windows 系统剪贴板双向互通设计

**日期：** 2026-05-14
**状态：** 已确认

## 摘要

扩展现有应用内闭环的复制/剪切/粘贴功能，实现与 Windows 系统剪贴板的双向互通。后端通过 Win32 API 封装 CF_HDROP 格式的读写，前端在现有流程中无缝集成系统剪贴板调用。

## 方案选型

选定**方案一：Go 后端封装 Win32 剪贴板 API**。

| 维度 | 说明 |
|------|------|
| 剪贴板范围 | 双向互通（应用 ↔ Windows 资源管理器） |
| 粘贴优先级 | 应用内剪贴板优先，无内容时读取系统剪贴板 |
| 快捷键 | 支持 Ctrl+C/X/V |

排除方案：
- 方案二（前端 Clipboard API）：Web API 不支持 CF_HDROP 文件格式，无法与资源管理器互通
- 方案三（混合方案）：增加复杂度但无实际收益

## 后端：Win32 剪贴板 API 封装

### 新增 `util/clipboard_windows.go`

| 函数 | 签名 | 说明 |
|------|------|------|
| WriteClipboardFiles | `(paths []string, isCut bool) error` | 写入 CF_HDROP + 剪切标记 |
| ReadClipboardFiles | `() (paths []string, isCut bool, error)` | 读取文件路径列表 + 是否剪切 |
| ClearClipboardFiles | `() error` | 清空系统剪贴板 |

**技术要点：**
- `DROPFILES` 结构体：`pFiles`（偏移量）+ 20 字节保留字段 + 文件路径列表（双 null 结尾）
- 剪切标记：`RegisterClipboardFormat("Preferred DropEffect")`，值为 `DROPEFFECT_MOVE(2)`
- 内存分配：`GlobalAlloc(GMEM_MOVEABLE)` + `GlobalLock`

### 新增 `service/clipboard.go`

在 `FileOperationService` 上新增：

- `CopyToSystemClipboard(paths []string) error`
- `CutToSystemClipboard(paths []string) error`
- `ReadFromSystemClipboard() ([]string, bool, error)`

### 新增 Wails 绑定 `app.go`

- `CopyToSystemClipboard(path string) string` — 单路径包装
- `CutToSystemClipboard(path string) string` — 单路径包装
- `ReadFromSystemClipboard() string` — 返回 JSON `{paths: [...], isCut: bool}` 或空字符串

## 前端：交互流程

### 复制/剪切流程（应用 → 系统）

```
触发复制或剪切
  → 现有逻辑：设置 clipboard reactive 状态 + 提示成功
  → 新增：调用后端 CopyToSystemClipboard/CutToSystemClipboard
```

后端调用静默执行，失败不影响应用内闭环功能。

### 粘贴流程（系统 → 应用）

```
触发粘贴
  → 检查 clipboard.mode（应用内剪贴板）
    → 有内容：使用现有应用内粘贴逻辑（不变）
    → 无内容：调用后端 ReadFromSystemClipboard()
      → 返回空：提示"剪贴板中没有可粘贴的内容"
      → 返回文件列表：
        → 根据目标位置解析目标目录
        → 遍历文件列表，逐个调用 CopyItem/MoveItem
        → 刷新文件树
```

### 快捷键支持

在 `FileTreePanel.vue` 的 `el-tree` 上监听键盘事件：

| 快捷键 | 条件 | 行为 |
|--------|------|------|
| Ctrl+C | 有选中节点 | 复制选中节点 |
| Ctrl+X | 有选中节点 | 剪切选中节点 |
| Ctrl+V | 有选中节点 + 剪贴板有内容 | 粘贴到选中节点 |

快捷键操作对象为当前选中节点（`selectedNode`），通过 Home.vue 协调。

## 边界情况

| 场景 | 处理 |
|------|------|
| 应用内复制后资源管理器又复制其他文件 | 系统剪贴板被覆盖，但应用内剪贴板保留（粘贴时应用内优先） |
| 资源管理器复制后在应用内粘贴 | 应用内剪贴板为空时读取系统剪贴板 |
| 系统剪贴板读取失败 | 静默降级，不影响应用内功能 |
| 切换工作目录 | 清除应用内剪贴板，不清除系统剪贴板 |

## 涉及文件

| 文件 | 变更 |
|------|------|
| `util/clipboard_windows.go` | 新增 Win32 剪贴板 API 封装 |
| `service/clipboard.go` | 新增系统剪贴板服务方法 |
| `service/clipboard_test.go` | 新增测试 |
| `app.go` | 新增 3 个 Wails 绑定方法 |
| `frontend/src/components/FileTreePanel.vue` | 新增键盘事件监听 |
| `frontend/src/components/ContentPanel.vue` | 无变更 |
| `frontend/src/views/Home.vue` | 粘贴逻辑扩展（读取系统剪贴板）+ 快捷键协调 |
| `frontend/wailsjs/go/main/App.js` | Wails 自动生成绑定 |
