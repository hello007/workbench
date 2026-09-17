# 设置弹窗与命令面板尺寸响应式优化

## Goal

WorkBench 一般全屏运行，但设置弹窗 `width="760px"` + `.settings-body { height: 420px }` 双固定、命令面板 `width="600px"` + `.palette-content { max-height: 400px }` 双固定，全屏下弹窗显得小、内容须滚动才能看完（设置页）/ 路径长截断（命令面板）。改为 `min()` 响应式尺寸，全屏自适应撑大、小窗自适应缩小，减少滚动 + 显完整路径，不改业务逻辑。

## Requirements

### 范围（MVP 收敛 — SettingsPanel + CommandPalette）

#### SettingsPanel（设置弹窗）
1. **el-dialog width 响应式** — `width="760px"` → `width="min(960px, 86vw)"`
   - 全屏（1920宽）：86vw=1655 但 min 卡 960px，保聚焦感不过宽
   - 小窗（1024宽）：86vw=881px，不溢出
2. **.settings-body height 响应式** — `height: 420px` → `height: min(620px, 78vh)`
   - 全屏（1080高）：78vh=842 但 min 卡 620px
   - 小窗（768高）：78vh=599px

#### CommandPalette（命令面板，Ctrl+P）
3. **el-dialog width 响应式** — `width="600px"` → `width="min(720px, 70vw)"`
   - 全屏：70vw=1344 但 min 卡 720px（命令面板宜窄于设置弹窗，保搜索聚焦感；720 显完整长路径）
   - 小窗（1024宽）：70vw=717px，不溢出
4. **.palette-content max-height 响应式** — `max-height: 400px` → `max-height: min(480px, 60vh)`
   - 全屏：60vh=648 但 min 卡 480px（结果列表撑高，但不过半屏保搜索框可见）
   - 小窗（768高）：60vh=461px
5. **top="15vh" 保留** — 已响应式，不动

### 共同约束
- nav/content 布局保持（SettingsPanel nav 200px + content flex:1；CommandPalette 单列）
- content overflow-y:auto 保留 — 极端小屏仍可滚动兜底
- script 业务逻辑不动
- 不触 wailsjs 绑定
- 上一任务 design-tokens 视觉精修成果保持（仅改尺寸值，不动变量驱动部分）

## Acceptance Criteria

- [ ] SettingsPanel el-dialog `width="min(960px, 86vw)"`
- [ ] `.settings-body` `height: min(620px, 78vh)`
- [ ] CommandPalette el-dialog `width="min(720px, 70vw)"`
- [ ] `.palette-content` `max-height: min(480px, 60vh)`
- [ ] SettingsPanel 全屏下 general 页 6 section / shortcuts 页 8 项尽量少滚动（620px 较 420px 多容纳约 50%）
- [ ] CommandPalette 全屏下长路径完整显示（720px 较 600px 宽）
- [ ] 两弹窗小窗下不溢出视口
- [ ] CommandPalette `top="15vh"` 保留
- [ ] 深浅双主题视觉正常
- [ ] SettingsPanel.spec.js + CommandPalette.spec.js 通过（el-dialog stub 接 width prop 不断言具体值）
- [ ] `cd frontend && npm test` 通过

## Definition of Done

- 不触 wailsjs 绑定
- Tests 绿
- 守 design-tokens 契约（无新硬编码色值，尺寸改不涉色值）
- README 无需更新（弹窗尺寸调整）

## Technical Approach

### 决策（ADR-lite）

**Context**: 设置弹窗（760×420）与命令面板（600×400）双固定，全屏下显小/路径截断须滚动。

**Decision**:
1. 范围 = SettingsPanel + CommandPalette（用户点名两弹窗）
2. 尺寸 = `min()` 响应式：
   - SettingsPanel：`min(960px, 86vw)` × `min(620px, 78vh)`
   - CommandPalette：`min(720px, 70vw)` × `min(480px, 60vh)`（窄于设置弹窗保搜索聚焦感）
3. 全屏卡上限保聚焦感，小窗按比例缩不溢出
4. 同类大弹窗（RepoFilterDialog/AiFunctionConfigDialog 等）不纳入本任务

**Consequences**:
- 设置弹窗内容容量 +50%（420→620px），general/shortcuts 页大部分免滚动
- 命令面板宽度 +20%（600→720px），长路径完整显示
- 小窗自动缩不溢出
- min() Chromium 99+ 支持，Wails WebView2 evergreen 满足
- 同类大弹窗仍原尺寸（已知技术债，Out of Scope）

### 实施计划（单 PR）
- PR1：SettingsPanel.vue + CommandPalette.vue 两文件 width/height 改响应式 + 验证

## Out of Scope

- 不改业务逻辑
- 不触 wailsjs 绑定
- 不改其他弹窗（RepoFilterDialog/AiFunctionConfigDialog/AiTaskHistoryPanel/CodeReviewResult 等同类大弹窗保留原尺寸，后续按同模式扩展）
- 不改小弹窗（UpdateDialog/GitBranches/GitRemotes/DirectoryTree/AiFunctionRunner 等合理尺寸）
- 不重构弹窗结构
- 不抽弹窗尺寸变量体系（未来系统性重构时再做）

## Technical Notes

- 关键文件：
  - `frontend/src/components/SettingsPanel.vue`（el-dialog width prop + `.settings-body height`）
  - `frontend/src/components/CommandPalette.vue`（el-dialog width prop + `.palette-content max-height`，top="15vh" 保留）
- 模式参照：`frontend/src/components/FileDiffDialog.vue`（width="80%" 响应式，本任务用 min() 与其拉开档位）
- 测试：`frontend/src/components/__tests__/SettingsPanel.spec.js` + `CommandPalette.spec.js`（el-dialog stub props 接 width，不断言具体值，改 width 不破测试）
- el-dialog width prop 支持任意 CSS 长度字符串（含 min()/vw）
- min() Chromium 99+ 支持，Wails WebView2 evergreen 满足

## Research References

无独立研究阶段。响应式弹窗尺寸为主流 Web 惯例（Linear/VS Code/GitHub Desktop），FileDiffDialog 已有现成模式。
