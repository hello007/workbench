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
| PDF | pdf.js 渲染到 canvas（含翻页/缩放） | `pdfjs-dist` |
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
* [ ] 点击 pdf，面板可查看（翻页/缩放）
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
* PDF/Office 的文本选择/搜索（pdf.js 自带搜索可用，Office 不强求）
* 文档比对、水印、AI 切片等高级特性
* Markdown 实时分屏预览

## Technical Notes

* 关键文件：`ContentPanel.vue`（前端预览态）、`service/fileoperation.go`（`ReadFileBytes`/`PreviewFile` 已改）、`model/models.go`（`Kind`/`FileBytes` 已加）、`app.go`（`ReadFileBytes` 已绑定）、`util/file.go`（`ReadFileSafe`）
* 技术栈：Go 1.26 + Wails v2.12 + Vue3 (Composition API) + Element Plus 2.13 + Vite 8
* pdf.js worker：Vite 用 `?url` 导入 worker，配 `workerSrc`；CJK PDF 需 cMap
* CodeMirror 6：ESM + Vite 原生，无 worker，虚拟滚动支持大文件

## Research References

* `research/image-pdf-rendering.md` — 图片 base64 + pdf.js
* `research/office-doc-preview.md` — Office 前端库（避坑 unioffice/Handsontable）
* `research/text-preview.md` — markdown-it + highlight.js + CodeMirror 6
* `research/flyfish-file-viewer.md` — Flyfish 调研（POC 实测：依赖过大不可行，已放弃）
