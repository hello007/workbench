# 修复右边栏提交历史/本地变动页滚动条问题

## Goal

修复右侧内容面板（`ContentPanel.vue`）在切换到"提交历史"和"本地变动"tab 时**整个右边栏出现滚动条**的问题，使三个 tab（仓库信息 / 提交历史 / 本地变动）布局行为一致——均不出现整体滚动条，内容超出时由 tab 内部的列表区滚动承载。

## What I already know（已探明的事实）

### 现状高度链（`ContentPanel.vue`，选中仓库节点时）

```
.content-panel { height:100%; overflow-y:auto; display:flex; flex-direction:column }  ← 整体滚动落点
  .content-inner { padding; display:flex; flex-direction:column; flex:1; min-height:0 }
    h2（节点名）                              ┐
    el-descriptions（路径信息）                │
    .git-actions（拉取/切换分支按钮）          │ 约 200px 固定区
    el-divider                                │
    el-tabs（无高度/flex 约束）  ← 断层在这    ┘
      el-tab-pane "仓库信息"    → GitInfo
      el-tab-pane "提交历史" lazy → CommitHistory
      el-tab-pane "本地变动" lazy → LocalChanges
```

### 三个 tab 组件的高度处理

| 组件 | 关键样式 | 行为 |
|---|---|---|
| `GitInfo.vue` | `.git-info-card { margin-bottom }`（**未设 height:100%**，el-card 自然高度） | 内容是 5 行描述列表（约 250–300px），加上方 200px 不超可视高度 → **不滚 ✅** |
| `CommitHistory.vue` | `.commit-history-card { height:100% }`；`.timeline-container { max-height:600px; overflow-y:auto }` | timeline 最多 600px + card header + 上方 200px ≈ 850px，**超过可视高度 → 触发整体滚动 ❌** |
| `LocalChanges.vue` | `.local-changes-card { height:100%; overflow:hidden }`；`el-table max-height="500"` | table 500px + commit 输入框 + 按钮区 + 上方 200px ≈ 850px → **触发整体滚动 ❌** |

### 根因

`el-tabs` 与 `el-tab-pane` **没有参与 flex 高度链**，tab 内容区未"填满剩余高度并内部滚动"，而是用**固定 max-height（600/500）**撑高内容，叠加上方固定区后超出 `.content-panel` 可视高度，触发其 `overflow-y:auto` → 整个右边栏出现滚动条。

## Assumptions（待验证）

- 期望行为：三个 tab 都不出现整体滚动条；内容多时由列表区（timeline / table）在自己区域内滚动。
- `LocalChanges` 底部的 commit 输入框 + 按钮区应**固定在底部**（不随列表滚动消失），仅上方表格区滚动。
- `GitInfo` 保持现状即可（内容少时自然不滚；若未来内容变多，按同一高度链自动内部滚动）。
- 不改功能逻辑，仅调整 CSS 高度链。

## Open Questions（仅阻塞/偏好类）

1. **[已定]** 滚动行为 = **内部列表区滚动**（方案 A）：tab 内容填满剩余高度，内容多时仅列表区（timeline / table）滚动，整个右边栏不出现整体滚动条。✅
2. **[已定]** 范围 = **三个 tab 统一建立 flex 高度链**；`GitInfo` 保持现状（内容少时自然不滚），高度链保证其不整体溢出。✅

## Requirements（演进中）

- 切换到"提交历史""本地变动"tab 时，整个右边栏不再出现整体滚动条。
- 内容超出时，由 tab 内部的列表区（timeline / table）滚动承载。
- `LocalChanges` 的 commit 输入框 + 按钮区固定在卡片底部。
- `GitInfo` 现状不回归（内容少时仍不滚动）。
- 不改变任何功能逻辑，仅 CSS/布局调整。

## Acceptance Criteria（演进中）

- [ ] 选中仓库节点，切到"提交历史"tab：右边栏整体无滚动条；提交条目很多时仅 timeline 列表区内部滚动。
- [ ] 切到"本地变动"tab：右边栏整体无滚动条；变动文件很多时仅表格区内部滚动；commit 输入框与按钮始终可见在底部。
- [ ] 切到"仓库信息"tab：与现状一致，无整体滚动条。
- [ ] 窗口高度变化（拉小/放大、终端面板展开/收起）时，列表区高度自适应，不出现整体滚动条。
- [ ] `npm test` 与 `npm run build` 通过；无功能回归。

## Definition of Done（团队质量基线）

- 布局改动通过手动验证（三个 tab + 不同窗口高度）。
- `npm test` / `npm run build` 全绿。
- 纯 CSS 调整，无需更新文档（不改变功能行为）。

## Out of Scope（明确排除）

- 功能逻辑改动、组件结构重构（仅高度链/overflow 调整）。
- 左侧两栏（DirectoryTree/FileTreePanel）布局调整。
- diff 弹窗（FileDiffDialog）布局（独立 `el-dialog`，不受面板高度影响）。

## Technical Notes

- 关键文件：`frontend/src/components/ContentPanel.vue`、`CommitHistory.vue`、`LocalChanges.vue`、`GitInfo.vue`
- 高度链修复要点（方案A 下）：
  1. `.content-panel` 保持 `overflow:hidden`（不再整体滚）；
  2. `el-tabs` 与 `:deep(.el-tabs__content)`、`el-tab-pane` 建立 `flex:1 + min-height:0` 链；
  3. `CommitHistory`：`.timeline-container` 去掉 `max-height:600px`，改 `flex:1; min-height:0; overflow-y:auto`；
  4. `LocalChanges`：card 内部改为 [header 固定] + [table 区 flex:1 滚动] + [footer 固定]，去掉 `el-table max-height="500"`；
  5. `GitInfo`：可选加 `height:100%` 参与链（内容少仍不滚）。
- Element Plus `el-tabs` 高度链需用 `:deep()` 穿透 `.el-tabs__content`。
