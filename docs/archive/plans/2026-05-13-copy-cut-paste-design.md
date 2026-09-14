# 文件复制/剪切/粘贴功能设计

**日期：** 2026-05-13
**状态：** 已确认

## 摘要

在文件树右键菜单和操作面板中新增剪切、复制、粘贴三个操作，支持文件和文件夹的复制与移动。采用应用内闭环模式，前端维护剪贴板状态，后端新增 `CopyItem` 和 `MoveItem` 接口处理文件系统操作。

## 方案选型

选定**方案一：前端状态 + 后端新增接口**。

| 维度 | 说明 |
|------|------|
| 剪贴板范围 | 应用内闭环，不与系统剪贴板交互 |
| 前端职责 | 维护剪贴板状态，解析目标路径，调用后端接口 |
| 后端职责 | 递归复制/移动，同名冲突自动重命名 |

排除方案：
- 方案二（纯前端）：无法处理二进制文件
- 方案三（单接口）：语义不清晰，扩展性差

## 数据流与状态

### 前端剪贴板状态

```js
const clipboard = reactive({
  mode: null,       // 'copy' | 'cut' | null
  sourcePath: '',   // 源文件/文件夹完整路径
  sourceName: '',   // 源文件/文件夹名称
  sourceType: ''    // 'file' | 'directory'
})
```

### 操作规则

| 操作 | 行为 |
|------|------|
| 复制 | 设置 `mode = 'copy'`，记录源信息，提示"xxx 路径复制成功" |
| 剪切 | 设置 `mode = 'cut'`，记录源信息，提示"xxx 路径剪切成功" |
| 粘贴 | 根据 `mode` 调用后端 `CopyItem` 或 `MoveItem`；剪切模式粘贴成功后清除状态 |
| 覆盖 | 再次执行剪切/复制时覆盖之前的状态 |

### 目标路径解析

| 右键对象 | 目标目录 |
|----------|----------|
| 文件夹 | 该文件夹路径 |
| 文件 | 该文件所在父目录 |

## UI 变更

### 右键菜单

文件夹和文件的右键菜单中，在"复制路径"之前统一插入：

```
─────────────
剪切    图标: Scissor
复制    图标: CopyDocument
粘贴    图标: DocumentCopy（clipboard 为空时禁用）
─────────────
复制路径      （已有）
```

### 操作面板

在 `ContentPanel.vue` 中，选中文件或文件夹时，在已有按钮组前增加：

```
[剪切] [复制] [粘贴]
```

粘贴按钮在剪贴板为空时禁用。操作对象为当前 `selectedNode`，逻辑与右键菜单一致。

## 后端实现

### 新增服务方法

**CopyItem(sourcePath, targetDir string) (string, error)**

- 判断 sourcePath 是文件还是目录
- 目标路径 = `filepath.Join(targetDir, filepath.Base(sourcePath))`
- 同名冲突调用 `findAvailableName` 自动追加 `(1)`、`(2)`...
- 文件：复制文件内容（保留权限）
- 目录：递归创建子目录 + 递归复制子文件
- 返回最终实际路径

**MoveItem(sourcePath, targetDir string) (string, error)**

- 同样的同名冲突处理
- `os.Rename` 移动（同磁盘原子操作）
- 跨盘失败时降级为 CopyItem + 删除源
- 返回最终实际路径

**findAvailableName(targetPath string) string**

- 检测目标路径是否存在
- 存在则追加 `(1)`，仍存在追加 `(2)`，以此类推
- 文件名：`file.txt` → `file(1).txt`
- 文件夹名：`folder` → `folder(1)`

### Wails 绑定

```go
func (a *App) CopyItem(sourcePath, targetDir string) string
func (a *App) MoveItem(sourcePath, targetDir string) string
```

成功返回实际路径，失败返回 `"错误: ..."` 格式（与现有风格一致）。

## 交互流程

```
用户右键 → 点击"剪切"或"复制"
  → 设置 clipboard 状态
  → 提示 "xxx 路径剪切/复制成功"
  → 粘贴按钮变为可用

用户右键目标位置 → 点击"粘贴"
  → 解析目标目录
  → 调用 CopyItem 或 MoveItem
  → 成功后：
    - 提示 "粘贴成功：xxx"
    - 刷新目标目录文件树节点
    - 剪切模式：额外刷新源目录节点，清除 clipboard
    - 复制模式：保留 clipboard（可继续粘贴）
```

## 边界情况

| 场景 | 处理方式 |
|------|----------|
| 剪贴板为空时点击粘贴 | 按钮禁用，不可点击 |
| 复制粘贴到同一目录 | 后端自动重命名为 `xxx(1)` |
| 剪切粘贴到源文件自身所在目录 | 后端检测源路径与目标路径相同，提示错误 |
| 连续剪切不同文件 | 覆盖剪贴板，之前的不受影响 |
| 切换工作目录 | 清除剪贴板状态 |
| 剪切后源文件不存在 | 后端报错，前端提示 |

## 涉及文件

| 文件 | 变更 |
|------|------|
| `service/fileoperation.go` | 新增 CopyItem、MoveItem、findAvailableName |
| `util/file.go` | 新增递归复制辅助函数 |
| `app.go` | 新增 CopyItem、MoveItem 绑定方法 |
| `frontend/src/components/FileTreePanel.vue` | 剪贴板状态、右键菜单新增项、粘贴逻辑 |
| `frontend/src/components/ContentPanel.vue` | 操作面板新增按钮 |
| `frontend/src/views/Home.vue` | 传递剪贴板状态、协调粘贴操作 |
| `frontend/wailsjs/go/main/App.js` | Wails 自动生成绑定 |
| `frontend/wailsjs/go/main/App.d.ts` | Wails 自动生成类型 |
