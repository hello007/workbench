# 设置弹窗垂直布局紧凑化

## Goal

用户反馈设置弹窗（SettingsPanel.vue）内上下布局过于松散：段落标题与设置项之间、设置项卡片之间的垂直间距偏大，弹窗整体高度固定 620px 导致内容显空。目标：压缩垂直间距，使布局更紧凑，不改动任何功能逻辑。

## What I already know

- 组件：`frontend/src/components/SettingsPanel.vue`（单文件，改动仅涉及 `<style>` 块，模板/逻辑不动）
- 弹窗结构：`el-dialog`（宽 `min(960px, 86vw)`）内 `.settings-body` 左 nav（200px）+ 右 content
- 当前垂直间距链（token：xs=4px / sm=8px / md=16px / lg=24px）：
  - `.settings-body` 高度固定 `min(620px, 78vh)`
  - `.settings-section-title`：18px 字号 + `margin-bottom: 24px`（lg）
  - `.settings-section-title--spaced`：`margin-top: 24px`（lg）
  - `.settings-item`：`padding: 16px` + `margin-bottom: 16px`（md）
  - `.settings-item-desc`：`margin-top: 4px`
- 相关规范：frontend-visual-conventions（四档间距内紧外松就近归并、section-title 字重梯度 h1-h3 用 600）+ design-tokens（禁硬编码色值，间距走 token）

## Assumptions (temporary)

- 仅调 CSS 间距与弹窗高度，不改 tabs 结构、不改 nav 宽度
- 间距仍使用既有 token（可降档使用 sm/xs，不新增 token 值）

## Open Questions

- ~~收紧幅度/范围~~（已决策，见 ADR-lite）

## Requirements (evolving)

- 压缩 `.settings-section-title` 与设置项之间、设置项卡片之间的垂直间距
- 遵守 frontend-visual-conventions 间距四档（就近归并，不发明新值）
- 具体调整（适度收紧档）：
  - `.settings-section-title`：font-size 18→16px、margin-bottom 24→12px
  - `.settings-section-title--spaced`：margin-top 24→16px
  - `.settings-item`：padding 16→12px、margin-bottom 16→8px
  - `.settings-body`：height `min(620px, 78vh)`→`min(560px, 78vh)`
  - `.settings-section-header`：margin-bottom 16→12px（跟随标题间距）

## Decision (ADR-lite)

**Context**: 垂直间距收紧幅度有三档可选（适度/深度/仅卡片），需在信息密度与呼吸感间取舍。
**Decision**: 适度收紧（推荐档）。标题降档 + 间距降半 + 弹窗高度微降；不做深度紧凑（去卡片感、收窄 nav）以免破坏 frontend-visual-conventions 卡片层级规范。用户在问答环节未选择，按推荐档执行。
**Consequences**: 纯 CSS 改动、可逆；若仍显松散可再评估深度档。

## Acceptance Criteria (evolving)

- [ ] 设置弹窗各页（通用/终端/搜索/快捷键）视觉上明显更紧凑，无重叠/裁剪
- [ ] 浅色/暗色主题下间距表现一致（间距与主题无关，回归验证即可）
- [ ] 前端测试通过（`cd frontend && npm test`）

## Definition of Done (team quality bar)

- Lint / 前端测试 green
- 若间距用法形成可复用约定，评估是否更新 docs/spec/frontend-visual-conventions.md

## Out of Scope (explicit)

- 不改设置项功能逻辑、store、Wails 绑定
- 不动其他弹窗（UpdateDialog / AiFunctionConfigDialog 等）

## Technical Notes

- 已检查文件：`frontend/src/components/SettingsPanel.vue`（全文）、`frontend/src/style.css`（spacing token 定义）
- 间距 token：`--spacing-xs: 4px / sm: 8px / md: 16px / lg: 24px`（style.css:61-64）
