# markdown 预览增强（mermaid / 目录 / 快捷键）

## Goal

增强文件预览的 markdown 体验：支持 mermaid 图形渲染、提供右侧标题目录（TOC）快速跳转、编辑模式支持 Ctrl+S 保存与 Esc 取消。均为纯前端优化，不改后端。

## Requirements

### 1. Mermaid 渲染（FilePreviewRenderer.vue）
- ```mermaid 代码块渲染为图形（流程图、时序图、类图等常见类型）。
- 新增 `mermaid` 依赖；`startOnLoad:false`，`securityLevel:'strict'`，手动 `run`。
- markdown-it：对 `lang==='mermaid'` 的代码块输出 `<pre class="mermaid">原始代码</pre>`（不经 highlight.js），其余代码块保持 highlight.js 高亮。
- 渲染时机：`renderedMarkdown` 更新后 `nextTick` 对 `.mermaid` 节点调用 `mermaid.run`（watch renderedMarkdown / 内容变化）。
- 渲染失败降级：捕获异常，保留代码文本并显示错误提示，不影响其余内容。

### 2. 标题目录 TOC — 右侧侧边栏（FilePreviewRenderer.vue）
- 仅 markdown 预览显示；预览区右侧固定一列可折叠 TOC（约 180px）。
- 从渲染结果提取 h1–h6 标题，按层级缩进展示；点击滚动定位到对应标题（复用 `scrollToAnchor` / `slugifyHeading`）。
- 无标题时不显示 TOC（或显示空提示）。
- 侧边栏可折叠，避免窄屏挤压正文。

### 3. 编辑模式快捷键 — 标准安全行为（ContentPanel.vue）
- 编辑态（`isEditing`）监听键盘：
  - **Ctrl+S**：阻止浏览器默认保存，触发 `handleSave`；仅在有未保存修改（`isContentModified`）时保存，无修改则无操作。
  - **Esc**：取消编辑；若有未保存修改则二次确认（ElMessageBox），确认后回到只读态，否则保留编辑态。
- 监听范围限编辑态的预览区，避免与全局树操作快捷键（Del 等）冲突；编辑态输入框内不误触树操作（现有 `isTypingContext` 已处理）。

## Acceptance Criteria

- [ ] ```mermaid 代码块渲染为图形；渲染失败降级为代码文本 + 错误提示，不崩溃。
- [ ] 非 mermaid 代码块仍由 highlight.js 正常高亮。
- [ ] markdown 预览右侧显示可折叠 TOC，点击目录项平滑滚动到对应标题。
- [ ] 无标题的 markdown 不显示（或空态）TOC。
- [ ] 编辑态 Ctrl+S 保存（无修改时不触发、不报错），阻止浏览器默认另存。
- [ ] 编辑态 Esc 取消：有未保存修改时二次确认，无修改直接退出。
- [ ] Vitest 前端测试通过（含新增用例）；`wails build` 正常。

## Definition of Done

- 前端测试通过；构建通过，关注 mermaid 引入后的打包体积（预期显著增大，可接受）。
- 不破坏既有预览/编辑/链接跳转功能。
- 更新 `docs/功能说明.md` / README 中文件预览相关说明。

## Technical Approach

- **Mermaid**：新增依赖 `mermaid`。改造 markdown-it `highlight` 回调：`lang==='mermaid'` 时返回 `<pre class="mermaid">${escapeHtml(code)}</pre>`（保留原文供 mermaid 解析），其余走 highlight.js。新增 `watch(renderedMarkdown)` + `nextTick` → `mermaid.run({ nodes: markdownBodyRef 内 .mermaid })`，try/catch 降级。初始化 `mermaid.initialize({ startOnLoad:false, securityLevel:'strict', theme:'default' })`。
- **TOC**：新增 `toc` computed，用 markdown-it 的 token 流或渲染后 DOM 提取标题（层级 + 文本）；模板加 `.preview-toc` 侧边栏 + 折叠开关；点击调用现有 `scrollToAnchor`。布局：`.preview-markdown-wrap` 改为 flex 行，正文 + TOC 侧栏。
- **快捷键**：编辑态挂 `@keydown`（或在预览容器/ textarea 上）监听：`e.key==='s' && (e.ctrlKey||e.metaKey)` → preventDefault + handleSave；`e.key==='Escape'` → Esc 取消逻辑（含二次确认）。绑定/解绑随 `isEditing` 生命周期，避免全局污染。

## Decision (ADR-lite)

- **Context**: markdown 预览缺 mermaid 图形、无目录导航、编辑态无快捷键。
- **Decision**: 引入 mermaid.js 内嵌渲染；TOC 用右侧可折叠侧边栏（复用现有锚点滚动）；Ctrl+S/Esc 采用标准安全行为（无修改不存、Esc 有修改二次确认）。
- **Consequences**: mermaid 使打包体积显著增大（可接受，惰性/按需可后续优化）；TOC 占用预览区右侧宽度（可折叠缓解）；快捷键需精确绑定生命周期防冲突。

## Out of Scope

- 后端逻辑、Wails 绑定变更。
- 非 markdown（CodeMirror 代码预览）的 TOC。
- mermaid 编辑态实时预览、mermaid 主题定制。
- 跨文件锚点跳转（保持现有限制）。
- mermaid 按需/异步加载优化（本次直接依赖引入，体积优化留待后续）。

## Technical Notes

- 关键文件：`FilePreviewRenderer.vue`（mermaid + TOC）、`ContentPanel.vue`（编辑态快捷键）。
- 相关测试：`FilePreviewRenderer.spec.js`、`ContentPanel.spec.js`。
- 现有基建：`scrollToAnchor(anchor)` / `slugifyHeading(text)`（576-592）、`onMarkdownClick`（594）、`handleSave`（787）/`handleCancelEdit`（805）/`isContentModified`（478）。
- markdown-it `highlight` 回调在 270-282；`renderedMarkdown` computed 在 284。
- CLAUDE.md 的 Mermaid v11.x 编写规范可作默认配置参考。
