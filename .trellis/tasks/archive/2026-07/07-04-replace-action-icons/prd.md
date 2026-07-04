# 用 assets 图标替换操作按钮图标

## Goal

用 `frontend/src/assets/icons/` 下的图片按名字对应替换操作按钮/菜单项的图标，提升识别度：`explorer.png`/`vscode.ico`/`warp.ico` 替换"打开资源管理器/用 VSCode 打开/用 Warp 打开"的 Element Plus 图标（`obsidian.png` 已用，不动）；`git.png` 替换 git 仓库标记的 `SuccessFilled` 绿对勾。

## What I already know（已探明的事实）

### icons 目录

`explorer.png` / `vscode.ico` / `warp.ico` / `obsidian.png`（已用）/ `git.png`。

### 现有 img 图标模式（obsidian，复用）

- import：`import obsidianIcon from '../assets/icons/obsidian.png'`
- 按钮：`<img :src="obsidianIcon" class="btn-img-icon" alt="Obsidian" />用 Obsidian 打开`
- 右键菜单：`<img :src="obsidianIcon" class="context-menu-img-icon" alt="Obsidian" />用 Obsidian 打开`
- 全局样式：`style.css:251 .btn-img-icon { ... }`；`context-menu-img-icon` 在各组件 scoped 样式里。

### 替换点（按名字一一对应）

| 图标 | 目标 | 当前图标 | 位置 |
|---|---|---|---|
| `explorer.png` | "打开资源管理器"/"在资源管理器中打开" | EP `Monitor` | ContentPanel.vue（文件夹分支 + 文件分支，2 处按钮）、DirectoryTree.vue（右键菜单 1 处）、FileTreePanel.vue（目录右键 + 文件右键，2 处）= **5 处** |
| `vscode.ico` | "用 VSCode 打开" | EP `EditPen` | 同上 **5 处** |
| `warp.ico` | "用 Warp 打开" | EP `Promotion` | 同上 **5 处** |
| `git.png` | git 仓库标记 | EP `SuccessFilled`（绿对勾）| DirectoryTree.vue 工作目录项（1 处）、FileTreePanel.vue 文件树节点（1 处）= **2 处** |
| `obsidian.png` | "用 Obsidian 打开" | 已用 img | — |

### 误匹配排除

- `CommandPalette.vue` / `SettingsPanel.vue`：仅注释/设置描述文字匹配，**无打开按钮**，不纳入范围。

## Assumptions（待验证）

- `.ico` 在 Vite 下作为静态资源 import 返回 URL，浏览器 `<img>` 能正常显示（Chrome/Edge/Firefox 支持 ico 解码）——实现时验证。
- 图标尺寸沿用现有 `btn-img-icon` / `context-menu-img-icon` 的样式（约 14px），与 obsidian 一致；git 标记 img 尺寸对齐 SuccessFilled 当前视觉（约 14px）。

## Open Questions（仅阻塞/偏好类）

1. **[已定]** `git.png` 用于 **git 仓库标记**（替换 DirectoryTree + FileTreePanel 的 SuccessFilled 绿对勾）。✅

## Requirements

- `explorer.png` / `vscode.ico` / `warp.ico` 分别替换 5 处"打开 X"按钮/菜单项图标，复用 obsidian 的 `<img class="btn-img-icon|context-menu-img-icon">` 模式。
- `git.png` 替换 DirectoryTree 工作目录项 + FileTreePanel 文件树节点的 git 仓库标记（SuccessFilled → img）。
- 图标尺寸/对齐与现有 obsidian img 一致；alt 文案合理。
- 不改功能逻辑，仅替换视觉图标。
- 移除替换后不再使用的 EP 图标 import（Monitor/EditPen/Promotion 在涉及组件中若无其他用处则清理；SuccessFilled 在 ContentPanel 仍有其他用途——拉取结果/状态栏——保留）。

## Acceptance Criteria（演进中）

- [ ] ContentPanel 两处查看操作组的"打开资源管理器/VSCode/Warp"按钮显示对应 png/ico 图标。
- [ ] DirectoryTree 右键菜单的"打开资源管理器/VSCode/Warp"项显示对应图标。
- [ ] FileTreePanel 目录右键 + 文件右键两处菜单的"打开资源管理器/VSCode/Warp"项显示对应图标。
- [ ] DirectoryTree 工作目录项的 git 仓库标记为 git.png（非 SuccessFilled）。
- [ ] FileTreePanel 文件树节点的 git 仓库标记为 git.png（非 SuccessFilled）。
- [ ] 图标尺寸/对齐与 obsidian 一致，视觉不突兀；`.ico` 在运行时正常显示。
- [ ] `npm test` / `npm run build` 通过；既有用例无回归。

## Definition of Done

- 上述 AC 全过；
- 清理无用的 EP 图标 import；
- `npm test`（含组件快照/用例）/ `npm run build` 全绿；
- 视觉变更无需更新文档（README 功能不变）。

## Out of Scope（明确排除）

- CommandPalette / SettingsPanel（无打开按钮）。
- ActivityBar / 工具箱等其他图标。
- git 操作按钮（拉取/切换分支）的图标（保持纯文字）。
- 图标图片本身的美术修改。

## Technical Notes

- 关键文件：`ContentPanel.vue`、`DirectoryTree.vue`、`FileTreePanel.vue`、`style.css`（.btn-img-icon 全局样式）。
- import 路径：`import explorerIcon from '../assets/icons/explorer.png'` 等；`.ico` 同理。
- 注意 ContentPanel 的 `SuccessFilled` **保留**（用于拉取结果/状态栏，第 304/339 行），不删除其 import。
- DirectoryTree/FileTreePanel 的 `SuccessFilled` 若仅用于 git 标记，替换后可从 import 移除（需 grep 确认无其他用处）。
- `.ico` 显示验证：Vite 把 `.ico` 当 asset 处理（与 png 一致返回 URL）；若构建时报错需补 asset 配置，否则正常。
