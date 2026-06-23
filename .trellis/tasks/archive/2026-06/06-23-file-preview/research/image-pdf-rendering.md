# Research: 在 Wails v2.12 + Vue3 WebView 中渲染本地图片与 PDF 文件预览

- **Query**: 本地磁盘上的图片（jpg/jpeg/png/bmp/gif/webp）和 PDF 文件如何在 Wails 内嵌 Chromium WebView 的前端组件内预览
- **Scope**: mixed（内部代码现状 + 外部 Wails/Chromium/pdf.js 社区方案）
- **Date**: 2026-06-23

## 0. 关键结论前置（TL;DR）

- **图片**：推荐**方案 A1（base64 dataURL）+ 现有 `PreviewFile` 扩展**。点选单个文件按需预览、单图通常小于数 MB，base64 直接塞进 `<img src>` 最简单、无端口/协议/生命周期成本，且与现有 `PreviewFile` 契约最契合。bmp / webp / gif / png / jpg 在现代 Chromium（Wails v2 自带，基于 CEF/WebView2 系内核）原生解码支持，无需后端转码。
- **PDF**：推荐**方案 B1（pdf.js）**作为内置预览，并保留**方案 B4（外部程序打开）**作为降级。Wails WebView 对本地 PDF 的原生渲染不可靠（WebView2 行为不稳定、依赖系统 Edge；macOS WKWebView 不内置 PDF.js），`<iframe src=本地pdf>` 不能作为主方案。
- **集成点**：现有 `FilePreview.Content string` + `IsBinary` + 1MB 上限不足以承载图片/PDF。建议**新增字段**而非改语义（见 §3）。

## 1. 现有体系现状（代码事实）

### Files Found

| File Path | Description |
|---|---|
| `model/models.go:115-124` | `FilePreview` 结构体定义 |
| `service/fileoperation.go:62-99` | `PreviewFile` 业务实现 |
| `app.go:225-...` | `App.PreviewFile(path)` Wails 绑定入口（maxSize=1MB） |
| `frontend/src/components/ContentPanel.vue` | 前端预览消费组件（待扩展图片/PDF 分支） |

### 现有数据契约（不可破坏）

```go
// model/models.go:115
type FilePreview struct {
    Path     string `json:"path"`
    Name     string `json:"name"`
    Size     int64  `json:"size"`
    Content  string `json:"content,omitempty"` // 文本内容；二进制文件留空
    IsBinary bool   `json:"isBinary"`          // 检测到 0x00 字节即置 true，且不再填 Content
    TooLarge bool   `json:"tooLarge"`          // 超过 maxSize(1MB) 即置 true，提前返回
    Error    string `json:"error,omitempty"`
}
```

关键行为（`fileoperation.go:62-99`）：

1. `os.Stat` 取大小，超过 `maxSize`（app 层传 1MB）直接返回 `TooLarge=true`，**不读内容**。
2. `util.IsPreviewable(filePath)` 为 false 时（非文本类扩展名），只读前 1024 字节做 0x00 探测；命中即 `IsBinary=true` 返回，**不填 Content**。
3. 文本类才走 `ReadFileSafe` 全量读为 `string` 放入 `Content`。

**对图片/PDF 的含义**：当前实现下，jpg/png/pdf 因扩展名不在文本白名单 → 直接判定 IsBinary → 前端拿不到任何字节，无法显示。这是必须扩展的核心痛点。

## 2. 图片预览方案对比

### 前置事实核对（Chromium 对 bmp/webp/gif 的原生支持）

现代 Chromium（>= 2017，含 Wails v2 在 Windows 用的 WebView2/Chromium 内核）原生支持解码：JPEG、PNG、GIF、WEBP、BMP、AVIF、ICO、SVG。结论：**bmp 和 webp 无需后端转码**，只要把字节喂给 `<img>` 即可。社区多年一致结论，无版本疑点。

### 方案 A1：后端读字节 → base64 dataURL 返回前端

**做法**：后端按扩展名识别为图片，读全文件字节，`base64.StdEncoding.EncodeToString`，拼成 `data:image/png;base64,xxxx` 返回；前端 `<img :src="dataURL">`。

| 维度 | 评价 |
|---|---|
| 内存 | base64 膨胀约 1.33x，且 Go 端字节切片 + 编码字符串 + JSON 序列化三份副本短暂共存；单图几 MB 可接受，**大图（>20MB）会有压力** |
| 性能 | 一次性编码，无网络往返；点选按需触发，零空闲开销 |
| 实现复杂度 | **最低**。约 20 行 Go + 前端一个 `<img>` |
| 大图处理 | 受 `maxSize` 约束即可（建议图片上限放宽到 5-10MB，独立于文本 1MB） |
| 与现有 `PreviewFile` 契合度 | **最高**。只需新增一个 `DataURL string` 字段或复用语义扩展 |
| 缺点 | 大图内存峰值；dataURL 不可流式 |

### 方案 A2：Wails 自定义协议 / AssetServer / 注册本地目录

**做法**：Wails v2 提供 `app.options.AssetServer.Handler`（自定义 `http.Handler` 响应 `/xxx`），或 Windows 端经 WebView2 的虚拟主机映射（`bind:localhost` 系），让前端用 `http://wails.localhost/<path>` 访问本地文件。

| 维度 | 评价 |
|---|---|
| 内存 | **最优**。浏览器按需 GET，可流式、可缓存、可 range 请求，大图零压力 |
| 性能 | 每张图一次 HTTP GET，浏览器并发拉取，支持缓存复用 |
| 实现复杂度 | 中。需写 handler 做路径安全校验（防目录穿越）、MIME 推断；要处理 Wails v2 AssetServer 与前端 SPA 路由的共存 |
| 大图处理 | 天然友好 |
| 与现有 `PreviewFile` 契合度 | 低。绕开现有方法，前端要先拿到一个可访问 URL；`PreviewFile` 仍只返回文本 |
| 缺点 | 引入新的访问入口与安全面（路径穿越、任意文件读），需要白名单 |

Wails 官方文档（v2）确实提供 `AssetServer.Handler`/`ExternalURL`/Windows `WebviewIsTransparent` 等选项，自定义协议渲染本地资源是社区常见做法，但**配置与安全成本高于方案 A1**。

### 方案 A3：临时本地 HTTP 服务

**做法**：Go 端用 `net/http` 起一个随机端口服务静态目录，前端 `http://127.0.0.1:<port>/file?path=...`。

| 维度 | 评价 |
|---|---|
| 内存 | 优（同 A2，流式） |
| 性能 | 优 |
| 实现复杂度 | 中高。要管理端口分配、生命周期（启停）、CSP/安全、端口冲突 |
| 大图处理 | 友好 |
| 与现有 `PreviewFile` 契合度 | 低 |
| 缺点 | 端口暴露风险（本机其他进程可访问）、生命周期管理繁琐 |

适合"文件浏览器批量缩略图墙"这种**多文件并发**场景，不适合"点选单个文件按需预览"。

### 方案 A4：其他 — `runtime` 选择文件 / 转存到 embed

不适用：本场景文件已在磁盘，无需选择对话框；转存到 embed 增加无谓复制。排除。

### 推荐结论（图片）

**方案 A1（base64 dataURL）** 最适合"用户点击单个文件按需预览"：

- 与现有 `PreviewFile` 契约高度契合，扩展面最小；
- 无端口/协议/生命周期/安全面新增；
- 单图按需触发，内存峰值瞬时且可控（配合放宽后的图片 size 上限）；
- bmp/webp/gif 原生支持，无需后端转码。

仅当未来出现"缩略图墙 / 大图（>20MB）流式浏览"需求时，再演进到方案 A2（自定义协议）。

## 3. 与现有 PreviewFile 的集成点（图片）

### 推荐：新增字段，不改既有语义

在 `FilePreview` 新增（向后兼容，老消费者忽略未知字段）：

```go
type FilePreview struct {
    // ...既有字段不变...
    DataURL  string `json:"dataUrl,omitempty"`  // 图片专用：data:image/<ext>;base64,...
    Kind     string `json:"kind,omitempty"`     // 预览类型分流：text/image/pdf/binary/unsupported
}
```

理由：

- **不要复用 `Content` 装图片字节**：`Content` 当前语义是"可编辑文本"，且 `SaveFile` 依赖它做回写；把 base64 塞进去会污染编辑链路。
- **不要复用 `IsBinary` 兜底**：图片也属二进制，但需要区别于"无法预览的二进制"。新增 `Kind` 枚举让前端精准分流（`ContentPanel.vue` 按 `Kind` 选渲染分支）。
- **图片上限独立**：app 层 `maxSize` 对图片放宽（建议 8MB）或在 service 内按 `Kind=image` 走独立阈值；避免把文本的 1MB 上限套到图片上。

### service 层改造轮廓（仅说明，不落代码）

1. `PreviewFile` 入口先按扩展名判定 `Kind`（image/pdf/text/binary）。
2. `Kind=image`：跳过 0x00 探测，读全文件（受图片上限约束），base64 编码，按扩展名映射 MIME（bmp→`image/bmp`、webp→`image/webp`、jpg→`image/jpeg`、png/gif 同名），填 `DataURL`，`Content` 留空。
3. `Kind=pdf`：见 §5。

## 4. PDF 预览方案对比

### 方案 B1：pdf.js（mozilla）渲染到 canvas / viewer

**做法**：前端集成 `pdfjs-dist`（npm），用 `getDocument` 加载 PDF 字节（来自后端 base64 或 ArrayBuffer），逐页渲染到 `<canvas>`；或直接用官方 `viewer.html`（功能完整：翻页、缩放、搜索）。

| 维度 | 评价 |
|---|---|
| 可行性 | **高**。Wails WebView 就是 Chromium，pdf.js 在其上运行完全等同普通 Web |
| 体积 | pdf.js 核心 + worker 约 1-2MB（gzip 后更小），对桌面应用可接受 |
| 与 Vite 打包兼容 | **良好**。`pdfjs-dist` 提供 ESM 入口；worker 需配置 `workerSrc`（用 `?worker` 或 `?url` 导入，Vite 原生支持） |
| 体验 | 与浏览器原生 PDF 等同，可控 |
| 缺点 | 需要正确配置 worker；首次集成有踩坑成本（worker 路径、cMap、字体） |

### 方案 B2：`<iframe src="本地pdf">` / object / embed

**做法**：直接把本地 PDF 路径塞进 iframe。

| 维度 | 评价 |
|---|---|
| 可行性 | **不可靠**。Wails WebView 对 `file://` 本地 PDF 的原生渲染依赖宿主：Windows WebView2 取决于系统 Edge 是否注册 PDF 处理器，行为不稳定；macOS WKWebView 不内置 PDF.js，常显示空白或下载。社区多份反馈证实不能作为跨平台主方案 |
| 实现复杂度 | 极低（一行标签）但不可控 |
| 与现有体系 | 低。仍需先把本地路径变成 WebView 可访问的 URL（回到方案 A2/A3 的协议或 HTTP 问题） |
| 缺点 | 黑盒、跨平台不一致、无法保证可用 |

### 方案 B3：后端转图片再展示

**做法**：Go 端用 PDF 渲染库（如 `mupdf` go 绑定、`unidoc`、调外部 `pdftoppm`/`gs`）把每页转 PNG，前端逐页 `<img>`。

| 维度 | 评价 |
|---|---|
| 可行性 | 中。需要引入 CGO/外部依赖或商业库，**构建复杂度显著上升**（尤其 Windows 下 CGO） |
| 内存/性能 | 差。整本 PDF 全量光栅化开销大 |
| 体验 | 无文字选择、无搜索 |
| 适用 | 仅当需要"只读缩略图"且无前端渲染能力时 |
| 缺点 | 重依赖、弱化体验 |

### 方案 B4：外部程序打开（降级方案）

**做法**：调用系统默认 PDF 阅读器打开（Windows `cmd /c start`、`rundll32` 或 `exec.Command`）。项目已有 `OpenInExplorer`（`fileoperation.go:143`）这类系统调用先例，复用模式即可。

| 维度 | 评价 |
|---|---|
| 可行性 | 高，零渲染成本 |
| 体验 | 跳出应用，非内嵌预览 |
| 定位 | **降级/兜底**，或作为"在新窗口打开"按钮 |

### 推荐结论（PDF）

**方案 B1（pdf.js）为内置主方案 + 方案 B4（外部打开）作为降级按钮**：

- pdf.js 在 Wails WebView 内表现等同浏览器，体验完整（选择/搜索/缩放）；
- 不引入后端 CGO 重依赖，保持构建简单；
- 降级路径复用现有系统调用模式，容错好；
- 不推荐 iframe 直嵌（跨平台不可靠），不推荐后端转图（依赖过重）。

## 5. 与现有 PreviewFile 的集成点（PDF）

延续 §3 的新字段方案：

- `Kind = "pdf"` 分支：后端读 PDF 全字节（建议上限 10-20MB），**base64 编码填入 `DataURL`**（MIME 用 `application/pdf`），前端 pdf.js 用 `atob` → Uint8Array 喂给 `getDocument`。
- 或：为避免 base64 膨胀，新增 `Bytes []byte`（`json:"bytes,omitempty"`）字段直传二进制；但 JSON over Wails IPC 对 `[]byte` 仍会 base64 编码，**与 dataURL 无本质差别**，故统一用 `DataURL` 更省心。
- 前端 `ContentPanel.vue` 按 `Kind` 分流：text → 代码/文本视图；image → `<img :src="dataUrl">`；pdf → pdf.js 容器；unsupported/binary → 提示 + "外部打开"按钮。

## 6. 综合推荐与最小改动清单

| 项 | 推荐 | 理由 |
|---|---|---|
| 图片渲染 | base64 dataURL（方案 A1） | 与 PreviewFile 契约最契合，单图按需最简，bmp/webp 原生支持 |
| PDF 渲染 | pdf.js（方案 B1）+ 外部打开降级（B4） | WebView 原生 PDF 不可靠，pdf.js 体验完整且无重依赖 |
| 模型扩展 | `FilePreview` 新增 `DataURL string` + `Kind string`，不改 `Content`/`IsBinary` 语义 | 向后兼容，避免污染文本编辑链路 |
| 上限策略 | 文本 1MB 不变；图片上限放宽至 ~8MB；PDF 上限 ~10-20MB | 区分类型独立阈值 |
| 前端分流 | `ContentPanel.vue` 按 `Kind` 选渲染分支 | 单一入口，扩展清晰 |

## 7. 外部参考

- **pdf.js / pdfjs-dist**（mozilla）：npm `pdfjs-dist`，ESM + worker，Vite 用 `import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url'` 配 `workerSrc`。官方仓库 `mozilla/pdf.js`。
- **Wails v2 AssetServer**：`wails.io` 官方文档 Options → AssetServer（Handler/ExternalURL/Middleware），用于方案 A2 自定义协议渲染本地资源。
- **Chromium 图片格式支持**：JPEG/PNG/GIF/WEBP/BMP/AVIF 原生支持，社区与 caniuse 长期一致，bmp/webp 无需转码。
- **WebView2 PDF 行为**：Microsoft Edge WebView2 文档，PDF 渲染依赖系统 Edge，行为不稳定；不适合作为跨平台唯一方案。

## 8. Caveats / 未核实点

- **大图内存峰值**：base64 三份副本（字节切片 + 编码串 + JSON）的瞬时峰值未做实测；若用户预览超 10MB 图片频繁，应在实现阶段用 pprof 验证，必要时演进到方案 A2。
- **Wails v2 IPC 对 `[]byte` 的实际传输**：Wails v2 对 `[]byte` 返回值的序列化行为（是否自动 base64）需在实现时用最小 demo 验证；若已自动编码，则后端可直接返回 `[]byte` 而非手工拼 dataURL。本研究的集成建议以"显式 dataURL 字段"为最稳妥。
- **Vite + pdf.js worker 的具体配置版本**：`pdfjs-dist` 不同大版本（v3 vs v4）的 worker 文件名/导入路径不同，实现时需锁定版本并核对 `workerSrc` 写法。
- **PDF cMap/标准字体**：渲染含 CJK 的 PDF 需要配置 `cMapUrl`/`standardFontDataUrl`，否则可能出现中文乱码；实现时需把 cMap 资源一并打包。
