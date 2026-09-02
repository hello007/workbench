# 右侧操作面板文件预览支持 HTML 渲染

## 目标

右侧操作面板「文件预览」当前对 `.html/.htm` 文件仅以源码形式展示（CodeMirror 6 只读视图）。本任务为其增加**渲染预览**（浏览器所见即所得，含 JavaScript 执行与相对资源加载），默认进渲染视图、可一键切回源码，并保留现有编辑保存能力。

## 需求

1. `.html/.htm` 文件点「预览」后默认进入**渲染视图**（iframe 所见即所得）。
2. **完整渲染含 JS**：页面内脚本可执行；必须经 iframe sandbox 隔离（`allow-scripts` 且**不带** `allow-same-origin`），脚本无法访问 parent、无法调用 Wails 绑定的 Go 方法。
3. **相对资源加载**：HTML 引用的同目录树相对资源（css/js/图片/字体）随渲染正常加载（后端静态资源 handler + `<base>` 注入）。
4. **源码切换**：渲染视图工具栏「源码」按钮切回 CodeMirror 源码视图；源码态可再切回渲染。现有「编辑」按钮与编辑保存链路（textarea 编辑、Ctrl+S、gbk 编码回写）在源码态完整保留。
5. **内嵌链接导航**：渲染视图内点击相对链接（`./other.html`）在 iframe 内跳转；外链 `http(s)` 点击走系统默认浏览器打开（不脱离应用窗口）。
6. **刷新按钮**：渲染视图工具栏「刷新」按钮重载当前文件（外部编辑器修改后无需重新选文件）。
7. 降级路径维持现状：超 1MB（tooLarge）、二进制、编码异常时按现有 text 逻辑提示，不强行渲染。

## 验收标准

* [ ] 选中 `.html/.htm` 点「预览」默认显示渲染效果，样式/图片/JS 均生效。
* [ ] 引用同目录相对资源（如 `./style.css`、`./app.js`、图片）的 HTML 页面渲染完整。
* [ ] sandbox 隔离生效：iframe 内脚本 `window.parent` 访问受限，无法触达 Wails 绑定方法。
* [ ] 「源码」按钮切换到 CodeMirror 源码视图；「编辑」可用，保存后切回渲染视图内容已更新。
* [ ] 渲染视图内点相对链接可跳转到同目录其他 HTML；点 `http(s)` 外链由系统默认浏览器打开。
* [ ] 「刷新」按钮可重载最新文件内容。
* [ ] 中文路径/文件名正常。
* [ ] 后端：`/preview-raw` handler 有路径穿越、扩展名白名单、目录子树约束的单元测试。
* [ ] 前端：renderer 的 html 分支分发、源码/渲染切换、sandbox 属性有 Vitest 测试。
* [ ] 既有测试全部通过（`go test ./...`、`cd frontend && npm test`）。

## 完成定义（团队质量线）

* 后端 `go test ./...` 全绿；前端 `npm test` 全绿。
* 安全设计（sandbox 策略、白名单、目录约束）在代码注释中说明。
* 行为变化更新 `docs/功能说明.md`，并确认是否更新 `README.md`（项目规则要求）。

## 技术方案

### 渲染架构（核心）

* **iframe `srcdoc` + `sandbox="allow-scripts"`（无 `allow-same-origin`）**：
  * srcdoc 内容 = 原文件内容 + 注入的 `<base>` 标签 + 外链拦截脚本。
  * opaque origin：脚本可执行但与 parent 隔离，从架构上杜绝触达 Wails 绑定与主页面。
* **`<base href="http://wails.localhost/preview-raw/<urlencoded 目录>/">`**：相对资源基于 base 解析到后端 handler。注意 base 必须用**路径式**路由（`/preview-raw/<dir>/file.css`），不能用 query 式（`?path=`），因为相对 URL 解析会丢弃 base 的 query 串。
* **外链拦截**：注入脚本拦截 `click` 事件，`http(s)` 链接 `postMessage` 通知 parent，由主页面调 `BrowserOpenURL`（复用 markdown 预览的外链模式）走系统浏览器；相对链接交由 iframe 原生导航（目标也走 `/preview-raw`）。

### 后端改动

* `server/preview.go` 新增 `/preview-raw/` 路径式 handler（与现有 `/preview-pdf` 并列）：
  * 扩展名白名单：`.html .htm .css .js .mjs .json .png .jpg .jpeg .gif .svg .webp .bmp .ico .woff .woff2 .ttf .otf .map` 等。
  * 目录子树约束：资源路径必须位于「HTML 入口文件所在目录」之下（防任意路径读取）。实现方式在编码阶段定（如首段编码目录 + 校验其余部分不再上跳）。
  * `http.ServeFile` 提供，Content-Type 自动；HTML 入口与本路由命中行为一致。
* `service/fileoperation.go` 的 `detectPreviewKind` 不变（HTML 仍为 kind=text，走现有 content/encoding 读取链路），渲染分支由前端按扩展名判定（与 `isMarkdown` 同模式 `isHtmlRender`）。

### 前端改动

* `FilePreviewRenderer.vue`：新增 `isHtmlRender`（kind=text 且 ext=html/htm）渲染分支（iframe srcdoc）；源码/渲染切换状态、刷新动作由 `ContentPanel.vue` 控制（按钮模式参考 markdown 的「目录」按钮）。
* `ContentPanel.vue`：HTML 预览时显示「源码/渲染」切换 +「刷新」按钮；编辑入口仅源码态展示（现状即 text 态，保持）。
* srcdoc 组装工具函数（base 注入 + 外链拦截脚本注入）独立封装，便于单测。

## Decision (ADR-lite)

**Context**：HTML 渲染需在「保真度（含 JS/资源）」与「安全（Wails 绑定不可被任意脚本触达）」间取舍。
**Decision**：iframe sandbox=`allow-scripts`（不带 allow-same-origin）+ srcdoc 注入 base 与外链拦截脚本 + 后端白名单静态资源 handler（路径式路由 + 目录子树约束）。
**Consequences**：动态页面可跑、资源可加载、parent 隔离；代价是后端新增一条需谨慎设计的安全面（白名单+子树约束+测试覆盖），且 srcdoc 受现有 1MB tooLarge 限制（超限 HTML 降级提示，可接受）。

**已确认决策记录**：
* 2026-09-02 渲染深度：完整渲染含 JS（用户选定）。
* 2026-09-02 视图模式：默认渲染 + 可切源码（用户选定）。
* 2026-09-02 MVP 范围：核心渲染 + 内嵌链接导航 + 刷新按钮；`.xhtml/.mht` 不做（用户选定）。

## 实现偏差记录（2026-09-02 实施时确认）

* **目录子树约束未做，改为白名单 + 绝对路径 + 普通文件校验**：iframe 内相对引用经浏览器解析 `../` 后服务端无法还原入口目录；安全模型与 `/preview-pdf` 对齐（PreviewFile 本就允许预览任意本地路径，白名单扩展不扩大能力面；sandbox opaque origin 内脚本也无法 fetch 读取）。理由已写入 `server/preview.go` 注释。
* **`/preview-raw` 用 `http.ServeContent` 而非 `ServeFile`**：Go `ServeFile` 对 basename 为 `index.html` 的请求会 301 重定向到目录，iframe 内跳 index.html 会落到目录请求被 400；`ServeContent` 直出且保留 Range/Last-Modified。

## Out of Scope

* `.xhtml` / `.mht` / 其他变体格式。
* 可视化 HTML 编辑（所见即所得编辑器）。
* 整站远程浏览（仅外链跳系统浏览器）。
* HTML 超 1MB 的渲染放宽（维持 tooLarge 降级）。

## 实施计划（小步提交）

* PR1（后端）：`/preview-raw` handler + 白名单 + 子树约束 + Go 单测。
* PR2（前端）：renderer html 渲染分支 + srcdoc/base/外链拦截封装 + 源码/渲染切换 + 刷新按钮 + Vitest 测试。
* PR3（收尾）：边界打磨 + `docs/功能说明.md` / `README.md` 更新。

## 技术备注

### 现状勘察（2026-09-02）

* 预览链路：`ContentPanel.vue` 调 `PreviewFile`（`service/fileoperation.go:78`）→ `detectPreviewKind` 返回 kind → `FilePreviewRenderer.vue` 按 kind 分发。
* HTML 现状：kind=text → CodeMirror 6 只读源码（`cmHtml()`），text 类支持「编辑」（textarea + 保存 + gbk 回写）。
* 已有渲染先例：markdown（markdown-it `html:false` 防 XSS + mermaid + TOC + 外链 `BrowserOpenURL` + 相对链接 openLink 应用内跳转）、PDF（iframe 加 `/preview-pdf` handler，`server/preview.go` 白名单仅 .pdf）。
* 安全背景：Wails v2 WebView2 主页面源 `http://wails.localhost`，绑定 Go 方法可被同 WebView 内 JS 调用 —— sandbox 不带 allow-same-origin 是硬要求。
* srcdoc 相对 URL 解析基于 parent URL；query 式 base 会被相对引用丢弃 query，故后端路由必须路径式。

### 关键文件

* `frontend/src/components/FilePreviewRenderer.vue` — 渲染器分发、srcdoc 分支
* `frontend/src/components/ContentPanel.vue` — 预览入口、模式切换/刷新按钮
* `server/preview.go` — 新增 `/preview-raw` handler
* `service/fileoperation.go` — kind 判定（不改逻辑，仅确认）
* `frontend/src/components/__tests__/FilePreviewRenderer.spec.js`、`ContentPanel.spec.js` — 前端测试
* `server/` 下新增 handler 测试 — 后端测试
