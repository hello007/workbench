# 修复：md 预览中相对链接点击导致页面跳转崩溃

## Goal

修复「文件预览」页面在 markdown 文档中点击相对引用（如 `./other.md`、`../readme.md`）时，触发顶层 window 原生导航，最终落到后端 `PreviewHandler` 返回 `{"error":"缺少 path 参数"}`，并导致整个 SPA 被替换、Vue 实例卸载，用户无法返回主界面、只能结束 exe 进程重启的严重缺陷。

目标：让 markdown 中的相对链接在应用内以预期方式打开，杜绝顶层导航，保证主界面始终可用。

## What I already know（已查实）

### bug 根因链

1. `frontend/src/components/FilePreviewRenderer.vue:26` 用 `v-html="renderedMarkdown"` 渲染 markdown-it 输出，相对链接被渲染为原生 `<a href="./other.md">`，**未做任何点击拦截或 href 改写**。
2. 点击后浏览器执行**顶层 window 原生导航**到 `./other.md`，绕过 Vue Router。
3. `main.go:41-43` 配置 `AssetServer.Handler = server.PreviewHandler()`：Wails 对 embed.FS 未命中的请求 fallback 到该 Handler。
4. `server/preview.go:33-36` 仅读取 `?path=` query，为空即返回 `400 {"error":"缺少 path 参数"}`；未区分 URL 路径，也未校验是否为 `/preview-pdf` 路由。
5. 顶层 window 被 JSON 错误文本整体替换 → SPA 卸载 → 无法返回主界面。

### 相关文件

| 文件 | 位置 | 作用 |
|---|---|---|
| `frontend/src/components/FilePreviewRenderer.vue` | markdown 渲染（26/280-287 行）、`<a>` 仅样式（809-816 行） | **修复主战场**：拦截链接点击 |
| `frontend/src/components/ContentPanel.vue` | `previewFile`（629-707 行）、`filePreview.path`（639/650 行） | 持有当前预览文件绝对路径，但未下传给 Renderer |
| `server/preview.go` | `PreviewHandler`（25-69 行） | fallback handler，**可选加固点** |
| `main.go` | `AssetServer` 配置（41-43 行） | fallback 路由注册 |

### 关键约束

- markdown 相对路径以**当前预览 md 文件所在目录**为基准解析（非 SPA URL）。
- `FilePreviewRenderer` 目前只有 `fileName`，无完整路径；需新增 prop（如 `filePath`）从 ContentPanel 传入 `filePreview.value.path`。
- ContentPanel `previewFile` 已能按任意路径加载预览，可复用作为「应用内打开」的执行点。

## Assumptions（待验证）

- 用户的「主诉求」是能在应用内继续预览被引用的文档，而非另起外部程序。
- 相对引用的目标文件大多也位于工作目录内，可被 `PreviewFile` 正常加载。

## Open Questions（仅 Blocking / Preference）

1. **[Preference]** 点击 md 中相对引用的期望行为？（见下方方案 A/B/C）
2. **[Preference]** 覆盖的链接类型范围：仅 `.md` / 所有相对可预览文件 / 含图片等所有相对路径？
3. **[Preference]** 外部链接（`http(s)`、`mailto`）处理：系统浏览器打开 / 窗内打开 / 拦截不跳转？
4. **[Preference]** 同文档锚点（`#section`）是否需要滚动定位？
5. **[Blocking 仅当目标不在文件树]** 若采用「文件树定位」方案，目标文件所在目录未展开时如何处理？

## Requirements（evolving）

- [必须] markdown 预览中任何 `<a>` 点击都**不得触发顶层 window 原生导航**（杜绝 SPA 崩溃）。
- [必须] 相对引用能根据当前预览文件目录正确解析为本地绝对路径。
- [必须] 外部链接与相对链接采用不同处理策略，外部链接不在窗内导航。
- [待定] 应用内打开的具体形态（预览切换 / 文件树选中）——见 Open Question 1。

## Acceptance Criteria（evolving）

- [ ] 在 md 中点击 `./other.md`，不再出现 `{"error":"缺少 path 参数"}`。
- [ ] 点击后主界面仍可正常操作，无需重启进程。
- [ ] 相对路径（含 `./`、`../`）解析到正确的本地文件。
- [ ] 外部 http(s) 链接不导致窗内导航崩溃。
- [ ] 新增前端单测覆盖链接点击分发逻辑（相对路径解析 / 外部链接 / 锚点）。

## Definition of Done

- 前端单测新增/更新并通过（Vitest）。
- `wails dev` 手动验证：相对引用、`../`、外部链接、锚点各路径行为正确。
- 不引入后端回归（PDF 预览 `/preview-pdf` 链路不变）。
- 必要时更新 `docs/功能说明.md` 与 README。

## Out of Scope（explicit）

- 后端 `PreviewHandler` 的路由白名单重构（仅做必要防御，不重写）。
- markdown 渲染引擎替换或 `html:true` 开启（保持 `html:false` 防 XSS）。
- 远程 markdown 文件抓取与渲染。

## Technical Notes

### 方案研判（待用户决策）

**方案 A：预览面板内切换（推荐）**
- 在 `FilePreviewRenderer` 拦截点击，解析相对路径为绝对路径，emit 事件由 ContentPanel 调用 `previewFile` 切换预览内容。
- 优点：体验流畅、改动集中（Renderer + ContentPanel）、不依赖文件树展开状态。
- 缺点：预览的文件与文件树选中态可能不一致；需维护「当前预览路径」。

**方案 B：文件树定位并选中**
- 解析为绝对路径后，在 `FileTreePanel` 中定位节点并触发选中，复用现有 `selectedNode → previewFile` 链路。
- 优点：文件树与预览状态一致，全应用焦点统一。
- 缺点：跨组件改动大；依赖文件树已加载目标节点（深层目录未展开时需自动展开，逻辑复杂）。

**方案 C：外部程序打开**
- 调用 `OpenWithDefaultApp` 用系统默认程序打开目标文件。
- 优点：实现最简。
- 缺点：脱离「预览」语义，体验割裂。

### 路径解析技术要点

- 基准目录 = `filePreview.path` 所在目录（ContentPanel 已持有，需下传）。
- 需兼容 Windows 反斜杠与正斜杠（`path.replaceAll('\\','/')` 后按 `/` 拼接，再 `path.normalize`）。
- 锚点 `#xxx` 需与文件路径分离解析。

### 可选后端加固（防御性，非必须）

- `preview.go` 在 path 为空时，仅当 URL 路径为 `/preview-pdf` 才报「缺少 path 参数」；其余 fallback 请求返回 404 或更友好提示，避免任何顶层导航都得到误导性 JSON。属治本之末，前端拦截才是治本。

## 决策与实施记录（2026-07-06）

### 决策（用户已拍板）

- **点击行为**：方案 A 预览面板内切换（不改 selectedNode，文件树选中态保持不变）。
- **应用内打开范围**：所有可预览文件（md/text/code/image/pdf/office）；不可预览或解析失败由 PreviewFile 自身报错回退。
- **文档锚点**：滚动定位到对应标题（按 slug 或原文匹配）。
- **外部 http(s) 链接**：系统默认浏览器打开（`window.runtime.BrowserOpenURL`）。
- **后端**：不动 `preview.go`（前端拦截已治本，遵循精准修改原则）。

### 实施改动

| 文件 | 改动 |
|---|---|
| `frontend/src/components/FilePreviewRenderer.vue` | 新增 `filePath` prop、`openLink` emit；`.markdown-body` 加 `@click="onMarkdownClick"`；实现 `isExternalHref` / `resolveAbsolutePath` / `slugifyHeading` / `scrollToAnchor` / `onMarkdownClick`；import `BrowserOpenURL`。 |
| `frontend/src/components/ContentPanel.vue` | `previewFile` 加 `overridePath/overrideName` 形参，内部统一用本地 `targetPath`；`defineExpose` 暴露 `previewFile`；模板加 `:file-path` 与 `@open-link="onPreviewLink"`；新增 `onPreviewLink`；预览按钮改 `@click="previewFile()"`（避免 MouseEvent 被当作 overridePath 传入）。 |
| `__tests__/FilePreviewRenderer.spec.js` | 新增「markdown 链接点击分发」5 用例（`./` / `../` / 反斜杠 / 外部 / 锚点）+ runtime mock。 |
| `__tests__/ContentPanel.spec.js` | 新增 `previewFile(overridePath)` 用例。 |

### 验证

- 前端单测：`npx vitest run` → **170 passed (13 files)**，含本次新增 6 用例。
- 待人工验证（`wails dev`）：在 md 预览点击 `./other.md`、`../README.md`、图片链接、http 链接、`#锚点`，确认不再出现 `{"error":"缺少 path 参数"}`、不再崩溃、行为符合预期。
