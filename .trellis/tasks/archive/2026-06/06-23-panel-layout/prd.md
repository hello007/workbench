# 右侧操作面板布局优化（按钮紧凑化）

## Goal

文件预览态的操作按钮紧凑化，减少垂直占用，使预览区大小更适中、整体更合理。

## Decision

**按钮紧凑化**：3 组操作按钮（基本/编辑/查看）从垂直堆叠改为**横向流动布局**（flex row + wrap），标签紧凑化，垂直高度从 ~6 行压缩到 2-3 行，预览区（flex:1）获得更大空间。

## 实施

- `.action-groups`（文件态 `.node-actions--file` 内）改 `flex-direction: row` + `flex-wrap: wrap`，3 组 button-group 横向流动
- `.action-label` 紧凑化（inline 小字，与按钮同行或紧凑排列），减少每组垂直占用
- 预览区 `.file-preview` 仍 flex:1，因按钮区变矮而获得更大比例
- 保留分组语义（基本/编辑/查看可辨识）、按钮可读可点

## 约束

- 只动 `ContentPanel.vue`（template + CSS），不改其他态（Git 仓库/文件夹）、不改功能（按钮事件）、不改后端
- UI 优化需看效果迭代：实现后用户 `wails dev` 查看，不满意再调

## Acceptance Criteria

* [ ] 文件预览态操作按钮区垂直高度明显减小（3 组横向流动）
* [ ] 预览区获得更大空间
* [ ] 按钮可读可点、分组可辨识
* [ ] 不破坏其他态与功能
* [ ] `npm run build` 通过
