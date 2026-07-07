# brainstorm: markdown frontmatter 预览优化

## Goal

优化文件预览（右侧操作面板）中 markdown 的 YAML frontmatter 展示。当前 `markdown-it` 默认不识别 frontmatter，文档开头的 `---\n...\n---` 块被当作普通 markdown 渲染（`---` 变 `<hr>`，`key: value` 变段落文本），观感混乱且丢失元数据语义。改为结构化属性面板展示，提升可读性。

## What I already know

- 渲染组件：`frontend/src/components/FilePreviewRenderer.vue`，用 `markdown-it` v14.2.0（`html:false` 防 XSS、`linkify:true`、`breaks:false`）+ `highlight.js`（已注册 yaml 语言）+ `mermaid` + TOC。
- `markdown-it` 默认不解析 frontmatter，是其被渲染为普通文本的根因。
- 依赖现状（`frontend/package.json`）：已装 `markdown-it`、`highlight.js`、`mermaid`；**未装** `js-yaml`（本次需引入）。
- 上一次 md 预览增强（归档任务 `07-06-md-preview-enhance`）做了 mermaid / TOC / 编辑态快捷键，未涉及 frontmatter。
- frontmatter 渲染入口：`renderedMarkdown` computed（约 319-326 行）调用 `md.render(props.content)`；`md` 实例构造在 300-317 行。
- TOC 提取用 `md.parse` token 流（357-378 行），frontmatter 剥离后需确保 TOC 传入剥离后的正文。

## Assumptions (temporary)

- frontmatter 必须出现在文档**最开头**（首字符即 `---`，允许前导 BOM 被 strip），否则视为正文。
- 仅 `.md` / `.markdown` 文件涉及（`isMarkdown` computed 已限定）。
- 预览为只读，无需在预览态编辑 frontmatter。

## Open Questions

- 无（需求已收敛，待最终确认）。

## Requirements (evolving)

- 引入 `js-yaml`（^4）依赖，解析 frontmatter 为对象。
- 用正则 `/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/` 提取 frontmatter 原文并从正文剥离，正文再交 `md.render`，避免 `---` 变 `<hr>`。
- frontmatter 解析成功 → 渲染为结构化属性面板（key-value 表格），置于 `.markdown-body` 正文上方。
  - 数组值 → `el-tag` 徽章。
  - 标量值（字符串/数字/布尔/日期）→ 原值文本展示，不做类型转换。
  - 嵌套对象值 → 降级为 `JSON.stringify(v)` 文本展示（MVP 不递归子表格）。
  - 多行字符串值 → 原样文本展示（换行由 HTML 折叠，不特殊处理）。
- frontmatter 解析失败 → 降级为带 hljs yaml 高亮的代码块，附轻量提示。
- 无 frontmatter（文档不以 `---\n` 开头，或 `---` 无匹配结束符）→ 不显示面板，正常渲染正文。
- `md.parse`（TOC 提取）也传入剥离后的正文，保持 TOC 行为一致。
- 属性面板默认展开（用户选择"结构化属性面板"而非"可折叠"方案）。

## Acceptance Criteria (evolving)

- [ ] 含 frontmatter 的 markdown 在正文上方显示结构化属性面板，数组值显示为标签徽章。
- [ ] frontmatter 中的 `---` 不再被渲染为 `<hr>`，`key: value` 不再显示为段落文本。
- [ ] frontmatter 解析失败时降级为 YAML 高亮代码块，不影响正文渲染。
- [ ] 无 frontmatter 的 markdown 渲染行为与现状一致（无面板、无回归）。
- [ ] TOC 标题提取不受 frontmatter 影响。
- [ ] Vitest 前端测试通过（含 frontmatter 解析、降级、无 frontmatter 用例）；`wails build` 正常。

## Definition of Done (team quality bar)

- Vitest 前端测试通过（含 frontmatter 解析与展示新增用例）。
- `wails build` 构建通过，关注 js-yaml 引入后的打包体积（~33KB，可接受）。
- 不破坏既有 markdown 预览（mermaid / TOC / 链接跳转 / 右键复制）功能。
- 更新 `docs/功能说明.md` / README 中文件预览相关说明。

## Out of Scope (explicit)

- 后端逻辑、Wails 绑定变更。
- frontmatter 编辑态实时预览（本次仅预览态）。
- TOML / JSON frontmatter（本次仅 YAML）。
- frontmatter 字段交互（如 tags 点击搜索同类）。
- 类型化展示（日期格式化、布尔图标等过度类型化）。

## Technical Approach

- **依赖**：新增 `js-yaml@^4`。
- **提取**：正则剥离 frontmatter 原文 + 正文，computed 无副作用，与 Vue 响应式解耦。
- **解析**：`yaml.load(fmRaw)`，try/catch 包裹，失败标记降级。
- **展示**：属性面板置于 `.markdown-body` 上方，key-value 表格；数组用 `el-tag`，标量原值。
- **降级**：解析失败复用已注册 hljs yaml 高亮渲染代码块。
- **TOC 兼容**：`md.parse` 传入剥离后正文。

## Decision (ADR-lite)

- **Context**：markdown 预览中 frontmatter 被当作普通文本（`---` 变 `<hr>`、`key: value` 变段落），观感混乱。
- **Decision**：采用方案 B —— 正则剥离 + js-yaml 解析 + 结构化属性面板，解析失败降级为 hljs yaml 代码块。仅引入 js-yaml 一个依赖。
- **Consequences**：js-yaml 增加 ~33KB 打包体积（桌面应用可接受）；不依赖 markdown-it 插件，避免回调与 Vue 响应式时序耦合；结构化展示提升可读性，接近 Obsidian Properties 体验。

## Technical Notes

- 关键文件：`frontend/src/components/FilePreviewRenderer.vue`。
- 相关测试：`frontend/src/components/__tests__/FilePreviewRenderer.spec.js`（待确认路径）。
- 现有基建：`md` 实例（300-317）、`renderedMarkdown`（319-326）、`toc`（357-378）、`onMarkdownClick`（695-723）、hljs yaml 注册（198,212）。

## Research References

- [`research/frontmatter-display.md`](research/frontmatter-display.md) — 推荐方案 B（正则剥离 + js-yaml + 结构化属性面板，解析失败降级为 hljs yaml 代码块），仅新增 js-yaml 一个依赖。

## Research Notes

### 主流工具惯例

- **隐藏**：GitHub、VitePress、Docusaurus —— frontmatter 仅作元数据，不渲染到正文。
- **结构化属性面板**：Obsidian Properties —— 解析为字段卡片，支持类型化展示。
- **YAML 代码块**：Typora 默认 —— 带语法高亮的原文代码块。
- **可折叠区块**：Hexo/VuePress 部分主题 —— 默认收起，点击展开。

### 解析方案

- **正则剥离**（零依赖）：`/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/` 提取 frontmatter 原文，正文剥离后再 `md.render`，与 Vue computed 管线解耦。
- **js-yaml**：YAML 解析事实标准（~33KB，v4+ 默认 safe schema），支持数组、嵌套、多行字符串、类型推断。
- 不采用 `markdown-it-front-matter` 插件：其回调在 `md.render` 期间执行，与 Vue 响应式存在时序耦合风险。

### 与 TOC 的兼容

- 正则剥离后，`md.parse` / `md.render` 均传入剥离后的正文，TOC 行为一致，frontmatter 中的 `---` 不再被解析为 hr token。
