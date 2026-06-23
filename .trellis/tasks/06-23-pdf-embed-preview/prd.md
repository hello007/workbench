# PDF 内嵌预览（方案 B：pdfjs viewer + iframe）

## Goal

实现 PDF 内嵌预览，替换当前的「用默认程序打开」降级。**规避已确认的前端 pdfjs 双实例问题。**

## Decision (ADR-lite)

**方案 B：pdfjs 官方完整 viewer 作为静态资源，用 iframe 加载。**

- iframe 是独立 browsing context（独立 window/模块图/Worker 池），pdfjs 类只在 iframe 内定义一份，与主页面完全隔离 → **架构上根治双实例**。
- 与之前 4 次"在主页面跑 pdfjs 库"的失败**本质不同**：主页面根本不 import pdfjs。
- Apache-2.0（无传染）、零 CGO、零外部 exe、保真最高（官方渲染器）、自带完整工具栏（翻页/缩放/搜索/缩略图/打印）。
- 详见 `research/pdf-embed-options.md`。

**Consequences**：viewer 静态资源几 MB 进 exe；viewer 工具栏 UI 风格与 WorkBench 不完全一致（可接受）；iframe 与主页面通信用 postMessage（如需）。

## 共同前提（go/no-go，必须先验证）

Wails AssetServer 把本地 PDF 文件路径映射成同源 URL（`http://wails.localhost/...`），iframe 能加载。dev（vite dev server origin）与 build（wails.localhost）两态行为一致。这是 research §6.1 标的关键不确定点。

## 分步实施（吸取 PDF 4 次失败教训，先验证基础）

- **POC-1（基础验证）**：Wails AssetServer handler 服务本地 PDF（同源 URL）+ 前端 iframe 直链（WebView2 原生渲染 PDF）。验证 AssetServer URL 映射 + iframe 加载基础可行。**通过才进 POC-2。**
- **POC-2（方案 B 完整）**：pdfjs 官方 viewer 资源放 `frontend/public/pdfjs-viewer/`；前端 pdf 分支 iframe 加载 `viewer.html?file=<pdfUrl>`（完整工具栏/保真）。

## Acceptance Criteria

* [x] POC-1：iframe 能加载并显示本地 PDF（AssetServer 同源 URL + WebView2 原生渲染）
* [x] POC-2：pdfjs viewer 渲染 PDF（翻页/缩放/搜索可用），dev + build 两态一致
* [x] 主页面不 import pdfjs（双实例无从发生）
* [x] exe 体积可控（最终 33.17MB，含 viewer）
* [x] `wails build` 成功

## 实施收尾记录

- 最终 exe 体积：**33.17 MB**（`build/bin/workbench.exe`，含 pdfjs viewer 静态资源）。
- pdfjs viewer 版本：**v4.8.69**（Apache-2.0）。
- locale 已精简为中文+英文：`web/locale/` 仅保留 `en-US`、`zh-CN`、`zh-TW` 三个目录。
- 路径安全：`server/preview.go` 仅放行 `.pdf` 扩展名，`filepath.Clean` + `Abs` 规范化，`os.Stat` 校验为普通文件；`http.ServeFile` 原生支持 Range/Last-Modified（大 PDF 按需读取）。
- 错误处理：handler 返回 400/404/405 + JSON；前端 iframe `src` 缺失时走降级分支（用默认程序打开）。
- 验证：`go build ./...`、`go test ./...`、`cd frontend && npm run build`、`wails build` 均通过。

## 约束

- Wails v2.12 + WebView2（Windows 桌面）
- 主页面不 import pdfjs 库（靠 iframe 隔离）
- 金融背景：许可证审慎（pdfjs Apache-2.0 已合规）
- 路径安全（AssetServer handler 防目录穿越）

## Out of Scope

- viewer 工具栏深度定制（先官方默认）
- iframe 与主页面的双向通信（除非必要）

## Research References

* `research/pdf-embed-options.md` — PDF 内嵌方案对比（B 推荐、C 兜底、A 排除）
* （归档）`06-23-file-preview/research/pdfjs-v6-pagesnumber-error.md` — 前端 pdfjs 双实例根因档案
