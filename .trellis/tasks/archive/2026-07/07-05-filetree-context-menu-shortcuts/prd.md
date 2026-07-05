# 工作目录树右键复制路径 + 重命名/删除快捷键

## Goal

为 WorkBench 文件管理界面做两项小优化：
1. 在「工作目录列表」(DirectoryTree) 右键菜单新增"复制路径"，方便复制工作目录绝对路径。
2. 为「工作目录列表」和「文件树」(FileTreePanel) 右键菜单中的"重命名"、"删除"操作增加键盘快捷键（默认 F2 / Del），并支持在设置面板自定义。

## What I already know（来自代码勘察）

### "复制路径"现状
- **FileTreePanel.vue（文件树）**：目录右键菜单（225-227 行）与文件右键菜单（286-288 行）**均已存在**"复制路径"，实现于 `onMenuCommand('copyPath')`（756-758 行），调用 `copyToClipboard(data.path.replaceAll('\\','/'), '路径')`。
- **DirectoryTree.vue（工作目录列表，标题"工作目录"）**：右键菜单（66-92 行）含 重命名/设为默认/各类打开/更新仓库/删除，**没有"复制路径"**。每个目录项持有 `dir.path`。

### 删除语义（关键安全事实）
- 文件树删除链路：`App.DeleteFile` → `FileOperationService.Delete` → `util.RemovePath` → **`os.RemoveAll(path)`**，即**永久删除，不进回收站**（`util/file.go:97-99`）。
- 工作目录删除链路：`DirectoryService.Delete` 仅从配置移除记录，**不动磁盘文件**（`service/directory.go:128`，UI 已声明"此操作不会删除实际文件"）。
- 两处删除前端均有 `ElMessageBox.confirm` 二次确认。

### 快捷键体系现状
- 已有 `frontend/src/composables/useShortcuts.js`（已落地），提供 `parseShortcut / matchShortcut / formatDisplay / loadShortcuts / saveShortcuts / shortcutFromEvent / isValidShortcut / checkConflict`。
- `model/settings.go` 的 `AppSettings` 已有 `ShortcutCommandPalette`（默认 Ctrl+P）、`ShortcutToggleTerminal`（默认 Ctrl+`）。
- 现有"固定快捷键"清单：F5 刷新、Ctrl+C/X/V。**未含 重命名/删除**。
- **关键约束**：现有 `isValidShortcut` 要求"必须含修饰键(Ctrl/Alt/Shift) + 按键"，会**拒绝 F2/Del 单键**。支持 F2/Del 必须放开该校验（允许功能键/单键）。
- 菜单快捷键提示样式 `.context-menu-shortcut` 已存在于 FileTreePanel（1309-1316 行），DirectoryTree 暂无。

### 键盘监听/选中上下文现状
- FileTreePanel 内部维护 `currentSelectedPath`（374 行），el-tree 设 `highlight-current`，可通过 `fileTreeRef.value.getCurrentNode()` 取当前高亮节点。
- DirectoryTree 的当前选中靠 `props.selectedId` 与 `contextMenu.targetDir`。
- `Home.vue` 全局 `handleGlobalKeydown` 是现有全局快捷键入口。

## Decisions（已确认）

- **D1（作用域）**：复制路径新增到 DirectoryTree；F2/Del 在 DirectoryTree 与 FileTreePanel 两棵树各自上下文都生效。
- **D2（可自定义）**：F2/Del 纳入自定义体系——扩展 `AppSettings` 新增 `ShortcutRename`、`ShortcutDelete` 字段，扩展 `useShortcuts`，设置面板增加两个录制项。
- **D3（单键校验）**：放开 `isValidShortcut`，允许 F1-F12、Del、方向键等单功能键作为合法快捷键（仍允许带修饰键组合）。

## Assumptions（待最终确认）

- A2：复制路径的目标值是工作目录绝对路径 `dir.path`，与文件树一致用 `/` 规范化。
- A4：Del 在文件树为永久删除（不进回收站），ElMessageBox 二次确认为最后兜底；需额外的"焦点判定"避免误触。

## Open Questions

- （已全部解决，见 Decisions）

## Requirements

- R1：DirectoryTree 右键菜单新增"复制路径"项，点击后将该工作目录绝对路径（`/` 规范化）复制到剪贴板，并 toast 提示。
- R2：DirectoryTree 与 FileTreePanel 右键菜单的"重命名"项右侧显示 `F2`、"删除"项右侧显示 `Del`（提示文本随自定义绑定动态变化）。
- R3：选中节点后按 F2 → 打开重命名对话框（与右键→重命名同流程）；按 Del → 弹出现有删除确认（与右键→删除同流程）。
- R4：F2/Del 默认值可自定义——`AppSettings` 增 `ShortcutRename`、`ShortcutDelete` 字段，设置面板录制 tab 增对应两项，持久化 + 重启生效，并参与冲突检测。
- R5（焦点判定，待 Q4 确认方案）：避免在输入框/终端/对话框打开时按 F2/Del 误触树操作。

## Acceptance Criteria

- [ ] DirectoryTree 右键某工作目录 → 出现"复制路径" → 点击后剪贴板为该目录绝对路径，toast 成功。
- [ ] DirectoryTree 选中某项按 F2 → 打开重命名对话框；按 Del → 弹删除确认。
- [ ] FileTreePanel 选中某节点按 F2 → 打开重命名对话框；按 Del → 弹删除确认（永久删除）。
- [ ] 两树右键菜单的重命名/删除项右侧显示当前绑定的快捷键。
- [ ] 设置面板 → 快捷键 tab → 可重命名/删除快捷键点击录制 → 按新键生效 → 重启后保持。
- [ ] 在终端/输入框/对话框聚焦时按 F2/Del **不**触发树操作。
- [ ] 录制时与其它快捷键冲突 → 提示且不生效。

## Definition of Done

- 前端单测（Vitest）：`DirectoryTree.spec.js` / `FileTreePanel.spec.js` 覆盖新菜单项、F2/Del 触发、焦点判定跳过。
- 后端：`go test ./...` 通过（AppSettings 新字段）。
- 手动验证：复制路径、F2/Del 触发、自定义持久化、重启生效、误触防护。
- README / 功能说明 / docs/功能说明.md 行为变化更新。

## Out of Scope

- 将已有固定快捷键（F5 / Ctrl+C/X/V）升级为可自定义。
- 改造删除为"进回收站"（属另一独立议题）。
- 重新设计右键菜单视觉。
- Mac Cmd 键跨平台适配。

## Technical Notes

- 关键文件：
  - `frontend/src/components/DirectoryTree.vue`（加"复制路径"项 + F2/Del 监听 + 菜单提示）
  - `frontend/src/components/FileTreePanel.vue`（F2/Del 监听 + 菜单提示）
  - `frontend/src/composables/useShortcuts.js`（放开单键校验 + 增 rename/delete 项 + 冲突检测）
  - `frontend/src/components/SettingsPanel.vue`（录制 tab 增两项）
  - `frontend/src/views/Home.vue`（全局 keydown 调度 F2/Del 到当前激活面板）
  - `model/settings.go`（`ShortcutRename` / `ShortcutDelete` 字段）
- 相关设计文档：
  - `docs/superpowers/specs/2026-06-08-custom-shortcuts-design.md`
  - `docs/superpowers/plans/2026-06-08-custom-shortcuts.md`
- 安全要点：Del 文件树 = `os.RemoveAll` 永久删除；必须靠焦点判定 + ElMessageBox 双重防护误触。

## Decision (ADR-lite)

- **Context**：需为两棵树加重命名/删除快捷键且可自定义；现有 useShortcuts 不支持单键；全局 keydown 需避免误触永久删除。
- **Decision**：
  1. `AppSettings` 增 `ShortcutRename`（默认 `F2`）、`ShortcutDelete`（默认 `Del`）。
  2. 放开 `isValidShortcut`，允许 F1-F12、Del 等功能键单键作为合法绑定。
  3. `Home.vue` 全局 `handleGlobalKeydown` 做严格焦点判定：先排除 input/textarea/contenteditable 聚焦、`el-dialog` 打开、终端面板聚焦；再分派到当前激活面板（工作目录列表 vs 文件树）的当前选中/高亮节点。
  4. 两树右键菜单的重命名/删除项右侧展示当前绑定（动态，随自定义变化）。
  5. `SettingsPanel` 快捷键录制 tab 增 rename/delete 两项，参与 `checkConflict`。
- **Consequences**：
  - 与现有 Ctrl+P / Ctrl+` 自定义体系统一，复用 useShortcuts。
  - 焦点判定 + ElMessageBox 双重防护误删。
  - 校验放开后理论上可录制任意单键（含字母），需录制 UI 文案引导合理绑定。
  - 全局 keydown 复杂度上升，须补足单测。

## Implementation Plan（small PRs）

- **PR1（基建）**：`AppSettings` 加 `ShortcutRename`/`ShortcutDelete`；`useShortcuts` 放开单键校验 + 增 rename/delete 默认值 + 扩展 `loadShortcuts/saveShortcuts/checkConflict`；Go 测试通过。
- **PR2（复制路径）**：`DirectoryTree.vue` 右键菜单加"复制路径"项（复用 `copyToClipboard` 思路）+ Vitest 单测。
- **PR3（F2/Del 触发 + 菜单提示）**：`Home.vue` 严格焦点判定调度；两树组件加 F2/Del 监听（作用于当前选中节点，走原有对话框/确认流程）；两树菜单项加动态快捷键提示；单测覆盖触发与跳过路径。
- **PR4（自定义 UI）**：`SettingsPanel.vue` 录制 tab 增 rename/delete 两项 + 冲突检测；单测。

