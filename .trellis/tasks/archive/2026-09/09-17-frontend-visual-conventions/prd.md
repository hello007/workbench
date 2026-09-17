# 前端页面风格开发规范沉淀

## Goal

将 WorkBench 当前前端页面风格、配色、布局、组件视觉模式、弹窗尺寸策略等沉淀为开发规范，供后续新增页面遵守，保持视觉一致性。并在 CLAUDE.md 增加索引入口。

## Requirements

### 交付物
1. **新建 `docs/spec/frontend-visual-conventions.md`** — 前端页面风格开发规范
2. **改 `docs/spec/README.md`** — 索引表加 frontend-visual-conventions 行
3. **改 `CLAUDE.md`** — 关键规则表加索引行 + 版本号升级

### 规范覆盖范围（核心）
1. **配色用法** — 主色靛蓝系应用场景（primary/success/warning/danger/info 语义）、蓝灰 Slate 中性色阶层级（bg/text/border 三档）、暗色阴影着色规则
2. **布局模式** — VSCode 式三列（ActivityBar 侧边栏 + FileTreePanel + ContentPanel）；面板内 nav+content 二分（nav 固定宽 + content flex:1）；Dashboard 表格 + 卡片化异常提示
3. **组件视觉模式**（从已改组件提取，供新组件复用）：
   - nav active 左侧指示条（`::before` left:0 防 overflow 裁剪，参照 ActivityBar/SettingsPanel）
   - 卡片 hover 阴影层级（`--shadow-sm` → `--shadow-md` 过渡，参照 DashboardView `.card`/SettingsPanel `.settings-item`）
   - section-title 字重梯度（h1-h3 用 600 + `letter-spacing:-0.01em`，h4-h6 用 500）
   - kbd 键帽字体族（`'Geist','Consolas','Monaco',monospace`）
   - 行内 style 禁用（全迁语义 class，守 redesign-skill Code Quality）
4. **字号/间距/圆角用法** — 四档变量（`--spacing-xs/sm/md/lg/xl`、`--radius-sm/md/lg/xl`）应用规则：内紧外松（容器 `--radius-lg` 较软、内元素 `--radius-md` 较紧）、间距就近归并档位
5. **弹窗尺寸策略** — `min()` 响应式（全屏卡上限保聚焦、小窗缩不溢出）；尺寸分档（设置弹窗 960×620、命令面板 720×480、内容多的大弹窗 900+、小弹窗 420-600）；`top` 用 vh 响应式

### 与 design-tokens.md 互补边界（避免重复）
- design-tokens.md 管：变量定义/取值/同步规则/正反例（技术契约层）— 本文引用不重述
- 本文管：页面怎么用这些变量搭出一致风格（指南层）— 配色用法/布局模式/组件视觉模式/尺寸策略

## Acceptance Criteria

- [ ] `docs/spec/frontend-visual-conventions.md` 写入，覆盖 5 项核心范围
- [ ] 与 design-tokens.md 互补无重复（引用变量名不重述取值定义）
- [ ] 含正反示例（正确用法 vs 反模式）
- [ ] `docs/spec/README.md` 索引表加 frontend-visual-conventions 行
- [ ] `CLAUDE.md` 关键规则表加索引行（指向 frontend-visual-conventions.md）
- [ ] CLAUDE.md 版本号升级 + 最后更新日期
- [ ] 新增页面可据此规范实现一致风格

## Definition of Done

- 文档结构与现有 spec 一致（摘要/适用范围/规范/正反例/关联文档）
- CLAUDE.md + docs/spec/README 索引同步
- README.md 主文档无需改（spec 体系自洽）
- 纯文档沉淀，无代码改动，无测试影响

## Technical Approach

### 决策（ADR-lite）

**Context**: 前三任务沉淀了视觉模式但散在组件代码中，新增页面无统一规范参照，易风格漂移。

**Decision**:
1. 放置 = `docs/spec/frontend-visual-conventions.md`（与 design-tokens 配套，守 CLAUDE.md 规范沉淀规则）
2. 范围 = 核心 5 项（配色/布局/组件模式/字号间距圆角/弹窗尺寸），紧扣"风格/配色/布局"
3. 与 design-tokens 互补：本文管指南，design-tokens 管契约
4. CLAUDE.md 关键规则加索引行 + docs/spec/README 索引

**Consequences**:
- 新增页面有统一参照，降低风格漂移
- 与 design-tokens 形成双子（契约 + 指南），检索路径完整
- 图标/动效/无障碍未纳入（Out of Scope，后续可补）

### 实施计划（单 PR）
- PR1：新建 frontend-visual-conventions.md + 改 docs/spec/README.md + 改 CLAUDE.md

## Out of Scope

- 不改代码（纯文档沉淀）
- 不重构 design-tokens.md（保持现状，新文档引用它）
- 不改现有组件视觉（仅沉淀规范）
- 图标规范（@element-plus/icons-vue 单一库，无须长篇）
- 动效/过渡规范（transition 变量用法，后续可补）
- 无障碍规范（focus-visible/对比度，后续可补）
- 不改 README.md 主文档

## Technical Notes

- 关键文件：
  - 新建 `docs/spec/frontend-visual-conventions.md`
  - 改 `docs/spec/README.md`（索引表）
  - 改 `CLAUDE.md`（关键规则表 + 版本号 + 日期）
- 参照：`docs/spec/design-tokens.md`（配套技术契约，本文引用）、`docs/开发规范.md`（现有代码规范体系）
- 素材源：已改组件 ActivityBar/DashboardView/SettingsPanel/CommandPalette 的实际视觉模式
- 文档结构参照 design-tokens.md（摘要/适用范围/触发条件/规范/正反例/关联文档/沉淀来源）

## Research References

无独立研究阶段。规范素材来自项目已确立的视觉模式（前三任务沉淀）。
