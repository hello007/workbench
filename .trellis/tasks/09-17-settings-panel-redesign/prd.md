# 设置弹窗界面视觉现代化

## Goal

SettingsPanel.vue（侧边栏设置按钮点击后的弹窗）沿用旧视觉：硬编码 fallback 双值泛滥、el-dialog 默认皮、行内 style 散落、未对齐上一任务（09-16-workbench）沉淀的 design-tokens 契约。基于 redesign-skill 审计 + design-tokens.md 契约做组件级视觉精修，与已改的 ActivityBar/DashboardView 模式一致，不破坏现有功能。

## Requirements

### 范围（MVP 收敛）
1. **删硬编码 fallback 双值** — scoped + 全局 style 所有 `var(--x, #硬编码)` 去除 fallback，纯变量驱动（守 design-tokens 契约）
2. **el-dialog 主题适配** — 圆角 `8px` → `--radius-md`/`--radius-lg`；header/body padding 用 `--spacing-*`
3. **settings-item 卡片** — hover 加阴影层级（`--shadow-sm` → `--shadow-md` 过渡）
4. **section-title** — 对齐全局 h3 字重梯度（`font-weight:600` + `letter-spacing:-0.01em`）
5. **kbd 键帽** — 字体族加 Geist 前缀（`'Geist', 'Consolas', 'Monaco', monospace`）；box-shadow 用变量
6. **restart-hint** — `rgba(230,162,60,…)` 硬编码透明度改用 `--warning-color` + 透明度叠加
7. **el-overlay** — `rgba(0,0,0,0.5)` 纯黑遮罩改蓝灰着色 `rgba(15,23,42,0.5)`（暗色更深）
8. **nav active** — 与 ActivityBar 模式对齐：左侧指示条 + 主色填充，hover 去冗余反馈
9. **行内 style 全迁到 class**
   - section-title `margin-top:24px` → `--spacing-lg` 语义类
   - input/select width（280/180/140/240/120px）→ `.input-w-{sm,md,lg,xl}` 语义类
10. **字号/间距硬编码** — `padding:14px 16px` / `gap:12px` 等用 `--spacing-*`

### 不变约束
- script 业务逻辑不动（loadSettings/saveSettings/快捷键录制/diff 工具配置/排除目录管理保持）
- 不触 wailsjs 绑定层
- tab 分组结构保持（general/terminal/search/shortcuts）

## Acceptance Criteria

- [ ] scoped + 全局 style 无 `var(--x, #硬编码)` 双值残留
- [ ] el-dialog 圆角/padding 用 `--radius-*`/`--spacing-*`
- [ ] settings-item hover 有 `--shadow-*` 层级过渡
- [ ] section-title 字重梯度对齐 h3
- [ ] kbd 字体族含 Geist 前缀
- [ ] restart-hint 用 `--warning-color` 变量驱动
- [ ] el-overlay 蓝灰着色非纯黑
- [ ] nav active 与 ActivityBar 模式一致（左侧条 + 主色填充）
- [ ] 12 处行内 style 全迁到 class
- [ ] 深浅双主题视觉正常（手动验证）
- [ ] 现有功能不回归：SettingsPanel.spec.js 通过 + 手动验证各 tab
- [ ] `cd frontend && npm test` 通过

## Definition of Done

- 视觉升级不触发 wailsjs 绑定层变动
- Tests 绿（vitest）
- 守 design-tokens.md 契约（无新硬编码色值）
- README 无需更新（单组件视觉调整，技术栈表已含 Geist）

## Technical Approach

### 决策（ADR-lite）

**Context**: SettingsPanel.vue 视觉未对齐上一任务沉淀的 design-tokens 契约，硬编码 fallback 与行内 style 散落。

**Decision**:
1. 范围 = 单文件 SettingsPanel.vue 视觉精修（template 行内 style 迁移 + scoped/全局 style 重写）
2. 行内 style 全迁到语义 class（`.input-w-*` 宽度类 + `--spacing-*` 间距类）
3. 沿用上一任务模式：纯变量驱动、阴影着色、Geist 字体、nav active 左侧指示条
4. 按 redesign-skill Fix Priority：配色清理 → hover/active → 布局间距 → 组件精修 → 字号收尾

**Consequences**:
- 风险可控（单文件、不改业务逻辑）
- 与 ActivityBar/DashboardView 视觉模式统一
- 行内 style 清零提升可维护性
- 须验证快捷键录制（kbd 录制态 `shortcut-item--recording`）暗色对比度

### 实施计划（单 PR）
- PR1：SettingsPanel.vue template 行内 style 迁移 + scoped/全局 style 重写 + 验证

## Out of Scope

- 不改 script 业务逻辑
- 不触 wailsjs 绑定
- 不重构设置项结构（tab 分组保持）
- 不加新设置项
- 其他组件（仅 SettingsPanel.vue）
- 不改 el-dialog 组件库本身（仅覆盖样式）

## Technical Notes

- 关键文件：`frontend/src/components/SettingsPanel.vue`（~800 行，template + script + scoped style + 全局 el-dialog style）
- 契约参照：`docs/spec/design-tokens.md`（纯变量驱动、阴影着色、Geist 字体、Element Plus --el-* 同步）
- 模式参照：`frontend/src/components/ActivityBar.vue`（nav active 左侧指示条）、`frontend/src/views/DashboardView.vue`（卡片化 + hover 阴影）
- 测试：`frontend/src/components/__tests__/SettingsPanel.spec.js` 须不破
- 行内 style 清单（12 处）：section-title margin-top ×4、input/select width ×8

## Research References

无独立研究阶段。模式已由上一任务（09-16-workbench）确立，直接复用 design-tokens 契约 + ActivityBar/DashboardView 精修模式。
