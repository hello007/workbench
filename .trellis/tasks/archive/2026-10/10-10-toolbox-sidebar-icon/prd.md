# brainstorm: 替换 sidebar 工具箱图标

## Goal

将左侧 ActivityBar（sidebar）与工具箱面板标题栏的「工具箱」图标，由 Element Plus 内置 SVG 图标组件 `SetUp` 替换为 `frontend/src/assets/icons/工具箱.svg` 的图形，并保证三态（常态/hover/active）可见性与其余面板图标一致。

## Requirements

1. 新建 `frontend/src/components/icons/ToolboxIcon.vue`：内联 SVG 组件
   * `viewBox="0 0 1024 1024"`，`fill="currentColor"`，不带 `width`/`height` 属性（尺寸交由 `el-icon` 的 `1em` 控制）
   * 仅保留工具箱图形 `path`（原文件第二个 path）
   * 丢弃原文件第一个 path（`fill="#202425" opacity=".01"` 的占位矩形，iconfont 导出残留）
   * 丢弃 `t` / `class="icon"` / `p-id` / `xmlns:xlink` / DOCTYPE 等导出残留
2. `ActivityBar.vue`：`panels` 数组 toolbox 项 `icon: SetUp` → `icon: ToolboxIcon`；import 行移除 `SetUp`
3. `ToolboxPanel.vue`：标题栏 `<SetUp />` → `<ToolboxIcon />`；import 行移除 `SetUp`，新增 `ToolboxIcon`
4. `ToolboxPanel.spec.js`：`vi.mock('@element-plus/icons-vue')` 工厂内移除已无引用的 `SetUp` 项，并同步第 22 行注释
5. 保留 `frontend/src/assets/icons/工具箱.svg` 作为源文件（与 icons/ 目录其余图标同处，不删）

## Acceptance Criteria

* [ ] sidebar 工具箱图标显示为 工具箱.svg 的图形（非 SetUp 内置图标）
* [ ] 工具箱面板标题栏图标显示为同一图形
* [ ] 三态下图标清晰可见：常态 `--sidebar-text` #94a3b8 / hover `--sidebar-text-hover` #cbd5e1 / active `--sidebar-active-text` #ffffff（经 `currentColor` 继承，零额外 CSS）
* [ ] 面板标题栏内图标继承 `--text-primary` 显示正常
* [ ] 其余 5 个面板图标、设置图标、终端图标不受影响
* [ ] `ActivityBar.spec.js`（8 项）与 `ToolboxPanel.spec.js` 全部通过
* [ ] `npm run build` 通过（无 MISSING_EXPORT 等绑定报错）

## Definition of Done

* 前端测试通过：`cd frontend && npm test`
* 覆盖率门禁通过（前端 ≥70% 硬失败，exclude wailsjs）
* 无新增 lint/typecheck 报错
* 视觉三态实测确认（`wails dev` 或浏览器通道目视）

## Technical Approach

内联 Vue SFC 图标组件 + `currentColor`。

`ActivityBar.vue` 的 `panels` 数组持有组件引用并经 `<component :is="item.icon" />` 渲染，新增一个本地组件即可零结构改动接入。项目已有内联 SVG + `currentColor` 先例（`ContentPanel.vue:264`），且无任何 SVG 相关 vite 插件依赖，故不引入新依赖。

`el-icon` 通过 `.el-icon svg { width:1em; height:1em }` 控制尺寸，`:size="20"` / `:size="18"` 设置 `font-size`，SVG 的 viewBox 保证等比缩放；`el-icon` 自身 `fill: currentColor` 与 SVG 根节点 `fill="currentColor"` 同值，无冲突。

## Decision (ADR-lite)

**Context**：原始需求为使用 `工具箱.png`，但经像素分析该 PNG 为纯黑单色 + 透明底，而 sidebar 恒为深色底（`--sidebar-bg` = `#0f172a` / 暗色 `#020617`），黑图不可见；且 `<img>` 不吃 `currentColor`，无法跟随三态变色。用户随后将资产更换为 `工具箱.svg`。

**Decision**：
1. 采用 SVG 方案（用户选择），放弃 PNG。
2. 以新建 `ToolboxIcon.vue` 内联组件接入（用户选择），而非模板 v-if 分叉或引入 svg 插件。
3. 面板标题栏 `ToolboxPanel.vue` 一并替换（用户选择），保证同一功能图标一致。

**Consequences**：
* 三态变色自动生效，与其余 6 个图标风格完全一致，零额外 CSS
* 新增 1 个 SFC 文件；`SetUp` 从 element-plus 导入中移除（该图标在本项目不再被引用）
* `assets/icons/工具箱.svg` 成为未被 import 的源文件（保留作溯源，非运行时依赖）
* 若后续换回 PNG 方案，需重新评估深色底可见性问题

## Out of Scope

* 其余面板图标替换
* 文件树 / 内容面板图标体系（`fileIconMap.js`）改动
* 引入任何 SVG vite 插件或新依赖

## Technical Notes

* 目标文件：
  * `frontend/src/components/ActivityBar.vue`（:16-18 渲染，:49 import，:60 panels 数组）
  * `frontend/src/components/ToolboxPanel.vue`（:4 模板，:91 import）
  * `frontend/src/components/__tests__/ToolboxPanel.spec.js`（:22-30 图标 mock）
  * `frontend/src/components/icons/ToolboxIcon.vue`（新建）
* `SetUp` 全项目引用点仅 ActivityBar.vue:49,60 与 ToolboxPanel.vue:4,91 —— 替换后无残留引用
* sidebar 令牌：`frontend/src/style.css:24-29`（亮色）、`113-118`（暗色）
* 面板标题栏 `.toolbox-title { color: var(--text-primary) }`，el-icon 继承该色
* `ActivityBar.spec.js` 仅断言 `.activity-bar-item` 数量与 index 态，不断言图标组件类型 —— 图标替换不破坏现有用例
* 无测试断言 `.i-setup`，移除该 mock 项安全
* 相关规范：`docs/spec/frontend-visual-conventions.md`、`docs/spec/design-tokens.md`
