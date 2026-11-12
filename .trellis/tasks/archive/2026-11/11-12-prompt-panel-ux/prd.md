# 提示词面板弹窗尺寸与替换预览优化

## Goal

两处体验问题：新建/编辑弹窗偏小，长提示词编辑不便；复制时填写占位符看不到最终内容，用户无法确认替换效果。本次调大编辑弹窗、在填写弹窗内增加实时替换预览并高亮替换值，同时适配浅色/深色主题。

## Requirements

1. **编辑弹窗尺寸**：新建/编辑提示词弹窗加宽至 `min(1040px, 94vw)`，正文输入区高度由 10 行提升到 18 行，小屏仍按视口比例收缩。
2. **填写弹窗尺寸**：占位符填写弹窗加宽至 `min(820px, 92vw)`，为预览区留出空间。
3. **实时替换预览**：填写弹窗内新增只读预览区，随输入实时展示「最终复制内容」。
4. **高亮规则**：
   - 用户填写的值 → 高亮（主色背景 + 主色文字）；
   - 使用默认值（未填且有 `{{名称:默认值}}`）→ 高亮且样式可区分（主色虚线边框）；
   - 未填且无默认值、保留原文的占位符 → 弱化标记（灰色虚线）；
   - 普通文本保持原样。
5. **主题适配**：高亮与预览区颜色全部使用 `frontend/src/style.css` 既有 CSS 变量，浅色/深色自动切换，不硬编码颜色。
6. **预览结构**：保留换行与空格（`white-space: pre-wrap`），最大高度约 32vh，超出滚动。
7. 复制原文、替换并复制、占位符提取与三级回退语义均不变。

## Acceptance Criteria

- [ ] 编辑弹窗宽度 ≥ 原 720px 基准，正文可视行数明显增加，94vw 内自适应
- [ ] 填写弹窗内出现「实际内容预览」区，输入变化即时更新
- [ ] 填写「张三」后预览中「张三」带高亮 class，且文本与复制结果一致
- [ ] 未填但有默认值时，预览显示默认值并带可区分的高亮 class
- [ ] 未填且无默认值时，预览保留 `{{名称}}` 并带弱化标记
- [ ] 深色主题下高亮对比度可辨（颜色来自 CSS 变量，无硬编码）
- [ ] 预览与复制文本一致（同一分段函数生成，不出现两套逻辑漂移）
- [ ] 现有 `PromptLibraryPanel` / `promptPlaceholders` 单测与 `prompt-library.spec.js` E2E 全绿，新增预览相关断言
- [ ] 前端构建通过

## Technical Approach

- `frontend/src/utils/promptPlaceholders.js`
  - 新增导出 `segmentPlaceholders(text, values)`：返回 `[{ kind, text }]`，`kind ∈ 'text' | 'filled' | 'default' | 'unfilled'`；
  - `applyPlaceholders` 改为 `segmentPlaceholders(...).map(s => s.text).join('')`，**复制结果与预览共用同一分段单点**，消除双实现漂移；
  - 分段规则与现有三级回退完全一致；无效占位符按 `text` 片段保留原文。
- `frontend/src/components/PromptLibraryPanel.vue`
  - 编辑弹窗 `width` 调整、正文 `rows` 调整；
  - 填写弹窗 `width` 调整，新增预览区（`data-preview`），`v-for` 渲染分段 `<span>`，按 kind 加 class，不使用 `v-html`；
  - 预览 computed 依赖 `fillTarget` 与 `fillValues`，实时更新；
  - 样式走主题变量：`--primary-bg` / `--primary-color` / `--border-light` / `--text-tertiary` / `--bg-tertiary`。
- 测试：
  - `promptPlaceholders.spec.js`：分段函数的文本/填写值/默认值/未填/混合/换行/无效占位符用例，并断言 `applyPlaceholders` 与分段拼接一致；
  - `PromptLibraryPanel.spec.js`：预览区渲染、三类 class、输入更新后刷新；
  - `prompt-library.spec.js`：填写后断言预览文本与高亮 class。
- 文档：`docs/spec/prompt-library.md` §3 补预览与弹窗尺寸约定。

## Out of Scope

- 列表卡片上的全文预览（现有展开/收起不变）。
- 预览区富文本、代码高亮。
- 弹窗尺寸的用户可配置化。
- 移动端专门布局（仅按 vw 收缩）。

## Technical Notes

- 当前弹窗尺寸：编辑 `min(720px, 86vw)`、填写 `min(640px, 85vw)`（`PromptLibraryPanel.vue:84/117`）
- 主题变量：`frontend/src/style.css:5-156`（`--primary-bg` 亮 `#eff6ff` / 暗 `rgba(59,130,246,.15)`）
- 占位符单点：`frontend/src/utils/promptPlaceholders.js`
- 视觉规范：`docs/spec/frontend-visual-conventions.md`（配色语义、深色对比度）
