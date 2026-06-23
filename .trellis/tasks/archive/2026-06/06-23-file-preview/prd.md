# 文件类型预览功能（图片 / Office / PDF / 文本）

## Goal

在 WorkBench 右侧操作面板（ContentPanel）中，点击文件树文件后，根据文件类型在面板内直接预览内容，覆盖图片（jpg/png/bmp）、Word、PPT、Excel、PDF、文本类（txt/json/sql/md 等）。让用户无需用外部程序打开即可快速查看文件内容。

## Decision (ADR-lite)

**Context**：现有 `PreviewFile` 体系仅支持文本类（可编辑 textarea），图片/PDF/Office 一律判为二进制拒绝。

**决策历程**：
1. 原计划采用 Flyfish Viewer 一站式组件。
2. **POC 实测发现 Flyfish 不可行**（详见下文「POC 结论」）：依赖树极其庞大（40+ 直接依赖，含 three.js/typst/cad/libarchive/sql.js 等多个 WASM），`npm install` 在 resolve 阶段停滞 10+ 分钟未落地任何包；即使装上，打包体积/时间会被全格式 WASM 显著拉高。对"只要图片/PDF/Office/文本"的本项目属过度依赖。

**Decision**：**回退自研拼装方案**。按类型引入成熟轻量库，依赖少、安装快、体积可控、完全可控。

**Consequences**：
- 优势：依赖可控（每个库单独核查许可证）、打包体积可控、构建快、符合金融背景对第三方依赖审慎的要求。
- 代价：需自行集成多渲染器、PPT 保真较弱、不支持旧格式 .doc/.ppt/.xls（降级外部打开）。
- 后端 `ReadFileBytes`/`Kind` 基础已就绪（POC 产出，可复用）。

## PDF 内嵌预览降级决策（2026-06-23）

**背景**：阶段 1 MVP 原计划用 `pdfjs-dist` 渲染 PDF（翻页/缩放）。但实测 pdfjs + Vite ESM + WebView2 存在系统性「双实例」问题，4 种 worker 配置（v6 / v4 × `?url` / `workerPort`）全部失败。

**根因**（已用 systematic-debugging 确认，详见 `research/pdfjs-v6-pagesnumber-error.md`）：即便 `workerPort` 设为真 Worker（console 已确认 `[pdf-preview] workerPort set: true Worker`），`WorkerTransport.getPage` 访问私有字段 `#pagePromises` 仍 brand-check 失败。Vite 把 `pdf.mjs`（主库）与 `pdf.worker.mjs`（worker）各打包一份，导致 `PDFDocumentProxy` 类被定义两份，主库的 `getPage()` 对 worker 创建的 proxy 做私有字段访问时失败。

**决策**：**PDF 暂不支持内嵌预览，降级为「打开」按钮手动外部打开**。
- 点击 PDF 文件不渲染、不自动打开；
- 需要查看时由用户手动点击现有「用默认程序打开」按钮，触发系统默认阅读器。
- 已移除 `pdfjs-dist` 依赖（`npm uninstall pdfjs-dist`），`FilePreviewRenderer.vue` 删除全部 PDF 渲染逻辑，`ContentPanel.vue` 中 `kind === 'pdf'` 不再调用 `ReadFileBytes`。
- 简体中文注释已补充在 `FilePreviewRenderer.vue` 的 `fallbackMessage` 与 `ContentPanel.vue` 的 `previewFile`。

**待后续出现确定性方案再评估**：例如后端转图片（缩略图/逐页栅格化）、或 pdfjs/WebView2 修复。

## POC 结论（Flyfish，2026-06-23）

- `npm view` 确认包存在于 npmmirror（2.0.11）。
- `npm install` resolve 阶段拉取 40+ 直接依赖（three/typst.ts/cad-viewer/libarchive.js/sql.js/excalidraw/pdfjs-dist/styled-exceljs/ag-psd/hls.js 等），即使只需图片/PDF/Office/文本，全格式 WASM 仍全量进入 `node_modules`。
- 实测：安装 10+ 分钟仍在 resolve，`node_modules` 未落地任何 Flyfish 依赖，`package.json`/`package-lock.json` 未更新（安装从未成功完成）。
- 判定：**Flyfish 依赖树过大，不适合本项目，放弃。** 详见 `research/flyfish-file-viewer.md` §6 实测记录。

## Technical Approach（自研拼装）

**后端（已就绪）**：
- `model.FilePreview` 新增 `Kind` 字段 + 类型常量（text/image/pdf/office/unsupported）+ `FileBytes` 结构（base64）
- `service.ReadFileBytes(path)` 返回 base64 字节；`detectPreviewKind` 按扩展名识别类型；`PreviewFile` 同步填 `Kind`
- `app.go` 绑定 `ReadFileBytes`（上限 50MB）
- ✅ `go build ./model/... ./service/... ./util/...` 通过

**前端按类型渲染栈**（基于 research）：

| 类型 | 渲染方案 | 依赖 |
|---|---|---|
| 图片 jpg/png/bmp/webp/gif | 后端 base64 → `<img :src="dataUrl">`（Chromium 原生，含 bmp） | 无 |
| PDF | **暂不支持内嵌预览**（pdfjs + WebView2 双实例问题），降级「用默认程序打开」 | 已移除 `pdfjs-dist` |
| Markdown | markdown-it 渲染（`html:false` 防 XSS） | `markdown-it` |
| 代码高亮（含 md 代码块） | highlight.js 按需语言 | `highlight.js` |
| 代码/txt/sql/json 只读 | CodeMirror 6 只读（行号/折叠/虚拟滚动） | `codemirror` + `@codemirror/{view,state,language,commands}` + `@codemirror/lang-*` |
| Word .docx | docx-preview | `docx-preview` |
| Excel .xlsx/.xls | SheetJS(xlsx) 读 + 表格渲染 | `xlsx` |
| PPT .pptx | pptxtojson（保真低）或降级外部打开 | `pptxtojson`（可选） |
| 旧格式 .doc/.ppt / 不支持 | 降级「用默认程序打开」 | 复用 `OpenWithDefaultApp` |

**布局**：上下结构·增量。文件信息 → 操作按钮（文本类加「编辑」）→ 预览区（`flex:1` 稳定高度），不破坏现有 Git/文件夹/文件三态。

**编辑能力（双模式）**：默认只读高亮预览；文本类点「编辑」切回现有 textarea + `SaveFile`（零废弃）。

## Requirements

* 按类型预览：图片/PDF/Office(docx/xlsx/pptx)/文本(md/json/sql/code/txt) 均可在面板内预览
* 保留文本类「编辑」入口，复用 `SaveFile`
* 不支持/损坏/超大文件 → 提示 + 「用默认程序打开」降级
* 各类型大小上限独立（文本 1MB；图片/PDF/Office 放宽）
* 许可证合规：核查每个引入库的许可证（已知避坑：unioffice/Handsontable 商业付费——本方案不用）

## Acceptance Criteria

* [ ] 点击 jpg/png/bmp，面板显示图片
* [ ] ~~点击 pdf，面板可查看（翻页/缩放）~~ **【降级/暂缓】**：pdfjs + WebView2 系统性双实例问题（详见上方「PDF 内嵌预览降级决策」与 `research/pdfjs-v6-pagesnumber-error.md`），暂不支持内嵌预览，点击 PDF 显示降级提示，由用户手动点「用默认程序打开」走系统默认阅读器。
* [ ] 点击 docx/xlsx/pptx，面板显示内容（保真度按各库能力）
* [ ] 点击 txt/json/sql/md，面板显示格式化/高亮/渲染内容
* [ ] 文本类提供「编辑」入口，编辑后可保存（现有 SaveFile 链路不回退）
* [ ] 不支持/超大/损坏文件，显示清晰提示 + 「用默认程序打开」
* [ ] `go test ./...`、前端 `npm test` 通过；`wails build` 成功
* [ ] README.md 评估并更新预览功能说明

## Definition of Done

* 单元/集成测试覆盖（后端 ReadFileBytes/detectPreviewKind；前端渲染分支、编辑/预览切换）
* `wails build` 产物体积记录（前后对比）
* 大文件/损坏/不支持/加密/宏文档降级提示齐备
* README.md 更新

## Implementation Plan（分阶段，小步推进）

* **阶段 1（MVP）**：图片 + PDF + 文本（md/代码/json/sql）。依赖成熟轻量，快速验证自研路线。安装 pdfjs-dist/markdown-it/highlight.js/codemirror；前端预览渲染器组件（按 Kind 分发）；ContentPanel 文件预览态接入（上下结构 + flex:1）；文本双模式（只读 + 编辑）。
* **阶段 2（Office）**：docx-preview + xlsx 表格 + pptxtojson（弱）/降级。旧格式 .doc/.ppt 降级外部打开。
* **阶段 3（收尾）**：测试、README 更新、打包体积评估。

## Out of Scope（本任务不做）

* 旧格式 .doc/.ppt/.xls 的内嵌预览（降级外部打开）
* **PDF 内嵌预览**：因 pdfjs + WebView2 系统性双实例问题（4 种 worker 配置均失败，详见 `research/pdfjs-v6-pagesnumber-error.md`）暂不支持，降级为「打开」按钮手动外部打开；待后续确定性方案（后端转图 / pdfjs 修复）再评估。
* PDF/Office 的文本选择/搜索（Office 不强求）
* 文档比对、水印、AI 切片等高级特性
* Markdown 实时分屏预览

## Technical Notes

* 关键文件：`ContentPanel.vue`（前端预览态）、`service/fileoperation.go`（`ReadFileBytes`/`PreviewFile` 已改）、`model/models.go`（`Kind`/`FileBytes` 已加）、`app.go`（`ReadFileBytes` 已绑定）、`util/file.go`（`ReadFileSafe`）
* 技术栈：Go 1.26 + Wails v2.12 + Vue3 (Composition API) + Element Plus 2.13 + Vite 8
* PDF：已放弃 pdfjs 内嵌方案（详见上方「PDF 内嵌预览降级决策」），降级「用默认程序打开」
* CodeMirror 6：ESM + Vite 原生，无 worker，虚拟滚动支持大文件

## Research References

* `research/image-pdf-rendering.md` — 图片 base64 + pdf.js
* `research/office-doc-preview.md` — Office 前端库（避坑 unioffice/Handsontable）
* `research/text-preview.md` — markdown-it + highlight.js + CodeMirror 6
* `research/flyfish-file-viewer.md` — Flyfish 调研（POC 实测：依赖过大不可行，已放弃）

## 打包体积记录（2026-06-23 收尾）

* `wails build` 产物 `build/bin/workbench.exe` ≈ **17.6 MB**（18,475,008 字节），已移除 `pdfjs-dist`。
* 前端 production chunk（`vite build`）：`Home-*.js` 约 1.67 MB（gzip 539 KB，含 docx-preview/xlsx/CodeMirror/highlight.js/markdown-it），`index-*.js` 约 0.96 MB（gzip 318 KB）。Vite 提示单 chunk 超 500 KB，属预期（桌面应用不强制拆包），后续若需可按路由/渲染器做动态 import 拆分。
* 结论：相对仅文本预览时期体积增长可控，符合「依赖可控、体积可控」的自研拼装决策。
