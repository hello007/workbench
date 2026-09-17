# WorkBench 界面视觉现代化重做

## Goal

WorkBench 当前前端视觉为 Element Plus 默认皮 + 早期手写 CSS 变量，缺乏开发者工具应有的质感与个性。在不更换技术栈（Wails + Vue 3 + Element Plus）的前提下，基于 `redesign-skill` 审计清单做针对性视觉升级，提升界面现代感与专业度，不破坏现有功能。

## Requirements

### 范围（MVP 收敛）
1. **全局 CSS 变量层升级**（`style.css` `:root` + `html.dark`）
   - 配色：主色换靛蓝系 `#3b82f6` → `#2563eb`（VS Code/GitHub 同色系），降饱和、统一冷暖灰为蓝灰色相
   - 阴影：暗色主题阴影由纯黑 `rgba(0,0,0,…)` 改为蓝灰着色 `rgba(15,23,42,…)`
   - 圆角/间距：四档变量保留，组件级使用统一
2. **Element Plus `--el-*` 同步覆盖**
   - `--el-color-primary` 及派生（light/dark 系列）同步靛蓝，否则 el-button/el-tag 割裂
3. **字体升级**
   - 换 Geist（Vercel 出品，开发者工具首选），下载 400/500/600/700 四字重 woff2 入 `assets/fonts/`
   - `style.css` `@font-face` 与 `font-family` 全替换，保留 fallback 链
4. **app.css 样板清理**
   - 删除 Wails 残留（`#logo`/`.result`/`.input-box`/`.input-box .btn` 等无用代码）
5. **验证组件精修**（两组件，验证全局变量效果 + 做组件级示范）
   - `DashboardView.vue`：header 质感、表格行 hover、卡片化状态异常提示
   - `ActivityBar.vue`：hover/active 反馈去重（现均 `scale(1.08)`），active 加左侧指示条

### 联动约束
- 终端 `--terminal-bg` 须与 `useTerminal.js` 的 LIGHT/DARK_TERMINAL_THEME.background 同步
- 右键菜单 `--menu-text-hover` 联动新主色
- 滚动条 thumb 已解耦 `--text-tertiary`，自动跟随

## Acceptance Criteria

- [ ] `style.css` `:root` + `html.dark` 主色为靛蓝系 `#3b82f6`/`#2563eb`，冷暖灰统一蓝灰色相
- [ ] 暗色主题阴影着色为蓝灰 `rgba(15,23,42,…)`，非纯黑
- [ ] Element Plus `--el-color-primary` 及派生同步靛蓝，el-button/el-tag/el-dialog 视觉一致
- [ ] `assets/fonts/` 含 Geist 400/500/600/700 四 woff2 文件
- [ ] `style.css` `@font-face` 与 `font-family` 全替换为 Geist，fallback 链保留
- [ ] `app.css` Wails 样板残留删除
- [ ] `DashboardView.vue` header/表格/异常提示组件级精修完成
- [ ] `ActivityBar.vue` hover/active 反馈去重，active 加左侧指示条
- [ ] `wails dev` 启动无样式报错，深浅双主题切换正常
- [ ] 现有功能不回归（手动验证 Dashboard 加载/跳转、ActivityBar 切换面板）
- [ ] `cd frontend && npm test` 通过

## Definition of Done

- 视觉升级不触发 wailsjs 绑定层变动（纯前端 CSS/组件）
- Tests 绿（vitest）
- Lint/类型检查通过
- README.md 视觉相关章节同步更新（CLAUDE.md 要求）
- spec 沉淀：CSS 变量体系规范变更写入 `docs/spec/`（若产生可复用契约）

## Technical Approach

### 决策（ADR-lite）

**Context**: WorkBench 视觉为 Element Plus 默认皮，缺开发者工具质感。需在不换栈前提下现代化。

**Decision**:
1. MVP 范围 = 全局 CSS 变量层 + Element Plus 同步 + 字体 + app.css 清理 + 2 验证组件（非 31 组件全量精修）
2. 主色 = 靛蓝系 `#3b82f6` → `#2563eb`（VS Code/GitHub 同色系，开发者心智模型最稳）
3. 字体 = Geist 4 字重（400/500/600/700），替换 Nunito
4. 按 redesign-skill Fix Priority：字体 → 配色 → hover/active → 布局 → 组件 → 状态 → 收尾

**Consequences**:
- 杠杆最高（CSS 变量体系结构良好，一次改动影响全部 31 组件）
- 风险可控（不改组件逻辑，仅视觉层）
- 验证组件示范组件级精修模式，后续可按同模式扩展到其余组件（Out of Scope）
- 字体加 +60KB woff2，桌面应用可接受

### 实施计划（小 PR）
- PR1：字体资源 + style.css 变量层 + Element Plus 同步 + app.css 清理（全局基础）
- PR2：DashboardView + ActivityBar 组件级精修（验证 + 示范）
- PR3：README/文档同步 + spec 沉淀

## Out of Scope

- 不迁移到 Tailwind / shadcn-vue
- 不改 Wails 绑定方法签名或 model 字段
- 不重构组件业务逻辑，仅视觉层
- 不引入新图标库（现有 `@element-plus/icons-vue` 保留）
- 第三套高对比度无障碍主题（未来扩展）
- AI 次级强调色体系（未来扩展）
- 31 组件全量 scoped 精修（后续按 PR2 模式扩展）

## Technical Notes

- 关键文件：
  - `frontend/src/style.css`（421 行，变量层主战场）
  - `frontend/src/app.css`（53 行，待清理）
  - `frontend/src/App.vue`（主题切换入口，不改）
  - `frontend/src/views/DashboardView.vue`（验证组件 1）
  - `frontend/src/components/ActivityBar.vue`（验证组件 2）
  - `frontend/src/composables/useTerminal.js`（终端主题同步参照，不改其常量但须核对 `--terminal-bg`）
- 主题机制：`html.dark` class 覆盖 `:root` 变量，特异性 (0,1,1) > (0,1,0)
- 现状字体坑：`assets/fonts/` 仅 regular 一档实际存在，600/700 woff2 文件缺失（404 静默回退），升级时直接换 Geist 四字重一并解决
- redesign-skill 路径：`.claude/skills/redesign-skill/SKILL.md`
- Geist 字体源：Vercel 官方 GitHub（SIL OFL 协议，可商用）

## Research References

无独立研究阶段。三个核心决策（范围/主色/字体）已通过 redesign-skill audit + 开发者工具行业惯例（VS Code/GitHub/Linear 配色）确定，无需 trellis-research 子代理。
