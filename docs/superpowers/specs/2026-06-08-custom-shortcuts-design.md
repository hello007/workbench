# 自定义快捷键设计

> **日期**：2026-06-08
> **状态**：待实施
> **类型**：功能增强

## 1. 概述

### 1.1 目标

支持用户自定义部分快捷键（命令面板、切换终端），自定义后快捷键可正常执行对应功能，并在设置面板和右键菜单中展示当前快捷键。

### 1.2 快捷键范围

| 功能 | 可自定义 | 默认值 |
|---|---|---|
| 打开命令面板 | ✅ | `Ctrl+P` |
| 切换终端面板 | ✅ | `` Ctrl+` `` |
| 刷新当前节点 | ❌ 固定 | `F5` |
| 复制选中项 | ❌ 固定 | `Ctrl+C` |
| 剪切选中项 | ❌ 固定 | `Ctrl+X` |
| 粘贴 | ❌ 固定 | `Ctrl+V` |

---

## 2. 数据模型

### 2.1 AppSettings 扩展

在 `model/settings.go` 新增两个字段：

```go
type AppSettings struct {
    // ... 现有字段 ...
    ShortcutCommandPalette string `json:"shortcutCommandPalette"` // 默认 "Ctrl+P"
    ShortcutToggleTerminal string `json:"shortcutToggleTerminal"` // 默认 "Ctrl+`"
}
```

旧配置无这两个字段时，Go 反序列化自动为零值空字符串，前端加载时负责填充默认值。

---

## 3. 快捷键解析与匹配

### 3.1 新增 composable: `useShortcuts`

创建 `frontend/src/composables/useShortcuts.js`：

- **`parseShortcut(str)`** — 将 `"Ctrl+P"` 解析为 `{ ctrlKey: true, altKey: false, shiftKey: false, key: "p" }`
- **`matchShortcut(event, shortcutStr)`** — 比对键盘事件是否匹配快捷键字符串
- **`formatDisplay(str)`** — 格式化为显示用数组，如 `"Ctrl+P"` → `["Ctrl", "P"]`
- **`loadShortcuts()`** — 从后端 AppSettings 加载配置，空值填默认
- **`saveShortcuts(shortcuts)`** — 保存到后端

解析规则：
1. 按 `+` 分割字符串
2. `Ctrl`/`Alt`/`Shift` 映射为修饰键布尔值
3. 剩余部分为 `key`（统一转小写）

---

## 4. 前端交互

### 4.1 设置面板快捷键 tab

可自定义项显示为可点击的录制区域：

```
打开命令面板          [Ctrl+P]  ← 点击进入录制
切换终端面板          [Ctrl+`]  ← 点击进入录制
刷新当前节点          [F5]      ← 灰色，不可编辑
复制选中项            [Ctrl+C]  ← 灰色，不可编辑
剪切选中项            [Ctrl+X]  ← 灰色，不可编辑
粘贴                  [Ctrl+V]  ← 灰色，不可编辑
```

### 4.2 录制流程

1. 点击可自定义项的按键区域 → 进入录制状态，显示"请按下新快捷键..."
2. 用户按下组合键（必须含 Ctrl/Alt/Shift + 字母/数字/符号）
3. 验证：
   - 非空组合键
   - 不与已有快捷键冲突
4. 保存到 AppSettings
5. 按 Escape 取消录制

### 4.3 Home.vue 改动

`handleGlobalKeydown` 中替换硬编码判断：

```js
// 之前
if (e.ctrlKey && e.key === 'p') { ... }

// 之后
if (matchShortcut(e, shortcuts.commandPalette)) { ... }
```

### 4.4 右键菜单快捷键提示

在 FileTreePanel.vue 的右键菜单中，对应操作后追加快捷键提示：

- 「刷新」 → `<span class="shortcut-hint">F5</span>`
- 「剪切」→ `<span class="shortcut-hint">Ctrl+X</span>`
- 「复制」→ `<span class="shortcut-hint">Ctrl+C</span>`
- 「粘贴」→ `<span class="shortcut-hint">Ctrl+V</span>`

---

## 5. 实施要点

1. 新增 `useShortcuts` composable（解析、匹配、加载、保存）
2. 扩展 `AppSettings` 模型
3. 设置面板快捷键 tab 支持录制
4. Home.vue `handleGlobalKeydown` 使用动态快捷键
5. FileTreePanel 右键菜单追加快捷键提示
