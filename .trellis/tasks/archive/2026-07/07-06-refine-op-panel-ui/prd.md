# 操作面板 UI 优化

## Goal

优化右侧操作面板（`ContentPanel.vue`）与提交历史卡片（`CommitHistory.vue`）的视觉呈现：解决提交卡片左侧异常留白、收紧标题间距、并对操作面板做中度重构，提升信息密度与观感。纯前端 UI 优化，不改动后端逻辑与交互功能。

## Requirements

### 1. 提交历史卡片（CommitHistory.vue）— 去时间轴改卡片列表
- 移除 `el-timeline` / `el-timeline-item`，改为紧凑的卡片列表（`div` 列表容器）。
- 消除轴线占位造成的左侧异常留白。
- 卡片头部单行呈现：短 SHA · 文件数 tag · 作者 · 相对时间（右侧展开箭头）。
- 提交消息紧凑展示，展开区（完整 SHA、邮箱、时间、变更文件）保持原有功能。
- 保留搜索、刷新、加载更多、复制 SHA、展开/收起交互。

### 2. 收紧标题间距（ContentPanel.vue）
- `文件操作`(h3) 与上方内容间距收紧：调整上方 `el-divider` margin 与 `.node-actions` 顶部内边距。
- `文件预览`/`编辑文件`(h4) 顶部间距收紧：调整 `.file-preview` 的 `margin-top`/`padding-top`。

### 3. 操作面板中度重构（ContentPanel.vue）
- 顶部改为紧凑 header：文件类型图标 + 文件名(h2)，下方一行路径（弱化灰字）+ 行尾内联「复制路径 / 复制文件名」图标按钮，去掉 el-descriptions 表格边框，降低占高。
- 操作按钮保留「基本 / 编辑 / 查看」（文件夹另有「高级」）分组，但分组标签更轻（小号、弱化色），按钮更紧凑，分组间留白统一。
- 统一卡片留白、分组标题样式，整体视觉层次清晰。

## Acceptance Criteria

- [ ] 提交历史卡片文本左侧无异常留白，卡片列表紧凑、信息密度合理。
- [ ] `文件操作`/`文件预览`/`编辑文件` 标题顶部间距明显收紧。
- [ ] 操作面板顶部为紧凑 header + 路径行，复制按钮内联可用。
- [ ] 操作按钮分组保留、样式更轻，功能不回退。
- [ ] Vitest 前端测试通过（含 ContentPanel.spec.js 现有断言：h2 文件名、路径文本、文件/文件夹字样）。
- [ ] `wails dev` / `wails build` 正常运行，无控制台报错。

## Definition of Done

- 前端测试通过；构建通过。
- 视觉改动不破坏既有功能与交互（复制、Git 操作、tab、预览、编辑、展开）。
- 若行为/结构变化，评估更新 `docs/功能说明.md` / README。

## Technical Approach

- **CommitHistory.vue**：模板层将 `el-timeline` 结构替换为 `.commit-list > .commit-card` 列表；CSS 移除 timeline 相关样式（`padding-left:12px` hack、wrapper 缩进），新增列表间距。时间戳改为卡片头部内联展示。脚本逻辑（loadCommits/filteredCommits/toggle/copy）不变。
- **ContentPanel.vue（文件分支）**：
  - 顶部 `el-descriptions` 两列表 → 紧凑 header 结构（`.panel-header` + `.panel-path-row`），复制按钮改内联图标按钮，保留 `handleCopyPath`/`handleCopyName`。
  - 收紧 `:deep(.el-divider--horizontal)` margin、`.node-actions` padding-top、`.file-preview` margin-top/padding-top、`.action-label` 样式。
  - 保留 h3「文件操作/文件夹操作」文字（测试依赖「文件/文件夹」字样）。
- 复用现有设计变量（`--spacing-*`/`--radius-*`/`--shadow-*`/`--primary-*`），不新增全局变量。

## Decision (ADR-lite)

- **Context**: 提交卡片左侧留白源于 el-timeline 轴线占位；操作面板信息密度偏低、标题间距偏大。
- **Decision**: 提交历史去时间轴改卡片列表；操作面板做中度重构（紧凑 header + 轻量分组），保留全部功能。
- **Consequences**: 失去时间轴的时序视觉线索（可接受，相对时间仍在卡片内展示）；改动集中在两个组件的模板+样式，回归风险低，需跑现有测试兜底。

## Out of Scope

- 后端逻辑、Wails 绑定、Git 命令行为变更。
- 新增功能（仅重排现有内容）。
- 「查看操作收进下拉」「去分组扁平化」等更激进方案（本次不做）。

## Technical Notes

- 关键文件：`frontend/src/components/ContentPanel.vue`、`frontend/src/components/CommitHistory.vue`。
- 相关测试：`frontend/src/components/__tests__/ContentPanel.spec.js`（stub 了 el-descriptions；断言 h2/路径/类型字样）。
- 设计变量：`frontend/src/style.css`（Element Plus 蓝色系 #409eff）。
