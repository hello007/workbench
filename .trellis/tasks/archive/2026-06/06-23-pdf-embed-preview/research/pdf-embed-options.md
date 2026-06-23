# Research: Wails v2（WebView2）下 PDF 内嵌预览的可实现方案（规避前端 pdfjs 主线程库）

- **Query**: Wails v2（Windows，WebView2）桌面应用中 PDF 内嵌预览的可实现方案，必须规避已确认的前端 `pdfjs-dist` 双实例（`PagesMapper`/`#pagesNumber` brand-check）问题
- **Scope**: mixed（方案可行性 + 许可证 + Windows 部署 + 与 Go 后端契合度）
- **Date**: 2026-06-23
- **约束前提**：
  - 前端 `pdfjs-dist`（v4/v6 均在排除范围）：任务明确要求**绕开"前端主线程跑 pdfjs 库"这条路**，故降级 v4 方案不在本研究推荐范围内（见 §0 澄清）。
  - 金融背景：规避 GPL/AGPL 传染与商业付费。
  - 项目现状：Wails v2.12 + Go 1.24 + Vue3；当前**零 CGO、零 PDF 依赖**；`main.go` 已用 `go:embed all:frontend/dist`；`build/bin/` 已用于随程序分发二进制（含 `data` 子目录）。

---

## 0. 一行结论（推荐方案排序）

| 排序 | 方案 | 一句话 | 许可证 | 推荐场景 |
|---|---|---|---|---|
| **推荐 1** | **B：pdfjs 官方完整 viewer + iframe 嵌入** | 把 pdfjs 官方 `web/viewer.html` 整套作为静态资源用 iframe 加载，iframe 是独立 browsing context，与主页面模块实例完全隔离，从架构上规避双实例 | Apache-2.0（宽松，无传染） | **首选**。开发量最小、保真度最高（官方渲染器）、纯前端静态资源、零外部 exe、零 CGO |
| **推荐 2** | **C：WebView2 原生 PDF 嵌入（iframe/object 直链本地 PDF）** | WebView2 内核是 Edge Chromium，原生支持渲染 PDF；Wails 下用 iframe/object 直接指向本地 PDF 文件路径 | 无（浏览器内置） | **次选/兜底**。零依赖，但 Wails `file://`/`wails.localhost` origin 下加载本地 PDF 需实测，工具栏/可控性差 |
| 备选 | **A-1：后端 Go 调 `pdftoppm`（poppler）转图片** | 后端把 PDF 逐页转 PNG，前端只显示 `<img>`，彻底规避前端 PDF 库 | poppler：GPL-2.0/3.0（**传染，金融不建议**）；改用 `poppler-windows` 分支 MIT 需核实 | 仅当 A/B/C 均不可行且可接受 GPL 风险时 |
| 不推荐 | A-2 mupdf | AGPL-3.0（强传染），CGO 构建复杂 | 金融背景直接排除 |
| 不推荐 | A-3 ghostscript | AGPL-3.0 | 金融背景直接排除 |
| 不推荐 | A-4 pdfcpu（Go 原生）转图片 | **pdfcpu 只能拆/合/提取，不能渲染成位图**（无光栅化引擎），无法满足"转图片预览" |
| 不推荐 | E 纯 Go PDF 光栅化 | 无成熟纯 Go 光栅化库（都要 CGO/外部引擎） |
| 不推荐 | D 非 pdfjs 前端库 | 纯前端能渲染 PDF 的库底层就是 pdfjs 或需 WASM/worker，仍有同类风险 |

**最终建议**：**优先落地方案 B（pdfjs 官方完整 viewer + iframe）**，它把"双实例"问题的根源——同一页面内主 bundle 与 worker chunk 的模块身份冲突——通过浏览器的独立 browsing context 从架构上消除，且 Apache-2.0 无传染风险、零 CGO、零外部 exe、保真度与官方 PDF.js 一致。B 失败时退方案 C。

---

## 1. 背景纠偏：为什么 iframe 嵌官方 viewer 能规避双实例

### 1.1 已确认的前端 pdfjs 双实例根因（不要再走）

详见 `.trellis/tasks/archive/2026-06/06-23-file-preview/research/pdfjs-v6-pagesnumber-error.md`。核心：在**同一个主页面 bundle**里，Vite 把 `pdf.mjs`（主库）与 `pdf.worker.mjs`（worker）各打包一份，`PagesMapper`/`WorkerTransport` 类被定义两份，私有字段 `#pagesNumber` 的 brand-check 失败。4 种 worker 配置全部失败，错误一字不差。

> 关键判据：双实例问题的病灶在"**主页面与主页面加载的 worker 处于同一 JS realm / 模块图，类构造器被复制成两份**"。

### 1.2 iframe 是独立 browsing context，从架构上根治

`<iframe src="viewer.html">` 加载的是一个**全新的 browsing context（独立 window / 独立模块图 / 独立 Worker 池）**。官方 `web/viewer.html` 是一个**自包含的单页应用**：它内部自己 `import` pdfjs、自己创建 Worker、自己配置 `workerSrc`，所有 pdfjs 类（含 `PagesMapper`）只在 iframe 的 realm 内定义一份。主页面（WorkBench）的 Vue 应用与 iframe 内的 viewer **不共享模块实例、不共享 JS realm**，因此：

- 不存在"主 bundle 一份、worker chunk 一份"的复制；
- `PagesMapper` 在 iframe 内只有一份构造器，brand-check 不会失败；
- 主页面根本不 import pdfjs，前端 `node_modules` 里甚至可以不装 `pdfjs-dist`。

这是与之前所有失败方案的本质区别：之前是在**主页面**里跑 pdfjs 库；方案 B 是把 pdfjs 关进一个隔离的 iframe 沙盒里跑。

---

## 2. 方案 B：pdfjs 官方完整 viewer + iframe 嵌入（推荐 1）

### 2.1 可行性结论

**可行，且是开发量最小、保真度最高的方案。** pdfjs 官方仓库 `mozilla/pdf.js` 的 `web/` 目录就是完整的 PDF 预览应用（带工具栏：翻页、缩放、搜索、侧边缩略图、下载、打印），构建产物可直接作为静态资源用 iframe 加载。

### 2.2 实现路径

1. **获取 viewer 静态资源**：从 `pdfjs-dist` 的 npm 包或官方 release 取预构建的 `web/` 目录（`viewer.html` + `viewer.{js,css,mjs}` + `viewer.worker.*` + `locale/`）。注意 viewer 版本必须与 worker 版本严格一致（官方构建已保证）。建议直接用官方 release 的 `pdfjs-x.y.z-dist.zip`（解压即得 `build/` + `web/`）。
2. **放入前端静态目录**：放到 `frontend/public/pdfjs-viewer/`（Vite 的 `public/` 原样拷贝到 `dist/`，不经打包，避免 Vite 对 viewer 内部 ESM/worker 做 chunk 拆分——这是关键，确保 viewer 内部模块图不被 Vite 介入）。
   > 也可放进 Go 侧 `embed` 目录随二进制分发，但放 `public/` 最简单，`main.go` 已 `go:embed all:frontend/dist` 会一并打包。
3. **iframe 加载**：
   ```html
   <iframe
     :src="`/pdfjs-viewer/web/viewer.html?file=${encodeURIComponent(pdfFileUrl)}`"
     class="pdf-viewer-frame"
   />
   ```
   官方 viewer 通过 URL query `?file=<path>` 指定要打开的 PDF。
4. **PDF 文件传递（三种方式，按可靠性排序）**：
   - **（首选）本地文件 URL（`http://wails.localhost/...`）**：Wails v2 开发态前端跑在 vite dev server（`localhost:34115`），打包后跑在 `file://` 或 `wails.localhost`。把 PDF 文件路径通过 Wails 的 AssetServer 暴露为可访问 URL（Wails v2 的 `assetsHandler` / 自定义 handler 可把任意本地路径映射成 HTTP 路径），iframe 用该 URL 加载。**避免 base64**（大文件 viewer 也能吃，但内存/性能差）。
   - **base64**：viewer 的 `?file=` 也支持，但官方更推荐传 URL。若 origin/路径受限，可后端把 PDF base64 喂给一个临时 blob URL。
   - **postMessage**：复杂，不推荐。
5. **origin 兼容性（关键实测点）**：Wails v2 默认用 `wails.localhost`（HTTPS-like）作为前端 origin，iframe 内 viewer 加载本地 PDF 时需同源或通过自定义 AssetServer handler 提供。这正是 Wails 的强项——它内置 AssetServer，把任意本地文件映射成 URL 是其设计能力。见 §6 Caveats 1。

### 2.3 维度评估

| 维度 | 评估 |
|---|---|
| 许可证 | **Apache-2.0**（pdf.js 官方）。宽松，无 copyleft 传染，金融可用。 |
| 外部依赖 | 无 exe、无 CGO、无系统库。仅前端静态资源（约几 MB：viewer + worker + locale）。 |
| Windows 部署 | 零额外部署。资源随 `dist/` 被 `go:embed` 进 exe，单文件分发不变。 |
| 保真度 | **最高**。就是官方 PDF.js 渲染器，与 Chrome 内置 PDF 查看器同源质量。 |
| 工作量 | **小**。取官方构建、放 `public/`、写一个 iframe 标签 + 文件 URL 映射 handler。 |
| 与 Wails+Go 契合度 | **高**。Wails AssetServer 天然支持把本地资源映射成 URL；前端只是个 iframe。 |
| 工具栏 | 自带完整工具栏（翻页/缩放/搜索/缩略图/打印/下载），无需自研。 |

### 2.4 风险点

- viewer 自带工具栏可能不符合 WorkBench UI 风格（可改 CSS/参数隐藏部分，但维护成本：升级 pdfjs 版本时改动要重做）。
- iframe 与主页面通信需用 `postMessage`（如需把"当前页/缩放"回传主应用）。
- 本地 PDF URL 映射需实测 Wails `assetsHandler` 在 dev/build 两态的行为一致性。

---

## 3. 方案 C：WebView2 原生 PDF 嵌入（推荐 2 / 兜底）

### 3.1 可行性结论

**基本可行，但可靠性依赖实测。** WebView2 内核是 Edge Chromium，Chromium **内置了 PDF 渲染能力**（同样是 PDF.js 移植）。理论上 `<iframe src="本地.pdf">` 或 `<object data="本地.pdf">` 直接让 WebView2 用内置 PDF 查看器渲染。

### 3.2 实现路径

```html
<iframe :src="pdfFileUrl" class="pdf-native-frame" />
<!-- 或 -->
<object :data="pdfFileUrl" type="application/pdf" class="pdf-native-obj" />
```
其中 `pdfFileUrl` 是 PDF 文件的本地 URL（同方案 B 的 URL 映射方式）。

### 3.3 维度评估

| 维度 | 评估 |
|---|---|
| 许可证 | 无（浏览器内置能力，不引入第三方代码）。 |
| 外部依赖 | 无。 |
| Windows 部署 | 零。依赖 WebView2 Runtime（Wails v2 已强依赖，用户机器必有）。 |
| 保真度 | 高（Chromium 内置 PDF.js）。 |
| 工作量 | 极小（一个标签）。 |
| 契合度 | 高。 |

### 3.4 风险点（为何排第 2）

1. **Wails origin 限制**：Wails 打包后前端在 `file://` 或 `wails.localhost`，iframe 加载 `file:///D:/...pdf` 跨 origin 可能被拦截或显示空白。需用 AssetServer 把 PDF 映射成同源 URL。
2. **工具栏不可控**：Chromium 内置 PDF 查看器的工具栏样式固定，无法定制、无法隐藏部分按钮（受 WebView2 版本行为影响）。
3. **行为一致性**：不同 WebView2 Runtime 版本对 PDF 查看器支持细节可能不同；部分精简版 WebView2 可能禁用 PDF 查看（`put_IsBuiltInErrorPageEnabled` / 相关策略）。**这是把它列为"需实测的兜底"而非首选的原因。**

---

## 4. 方案 A：后端（Go）把 PDF 转图片，前端 `<img>` 显示

### 4.1 总体可行性

**技术上可行，但每个子方案都有硬伤（许可证或能力缺失）。** 思路：Go 后端把 PDF 逐页光栅化为 PNG/JPEG，前端只显示 `<img>`，翻页=请求下一页图片，彻底规避前端任何 PDF 库。适合超大 PDF 按需转页。但金融背景 + 单文件分发约束下，各渲染引擎均有障碍。

### 4.2 A-1：poppler（`pdftoppm` / `pdftocairo`）

| 维度 | 评估 |
|---|---|
| 许可证 | **GPL-2.0-or-later**（poppler 上游）。**金融背景有 GPL 传染风险**：若把 poppler 静态/动态链接进商业闭源产品，理论上要求开源。**有争议点**：调用独立 `pdftoppm.exe`（子进程、非链接）通常被认为不构成"链接"，GPL 边界存疑——但合规审查往往一刀切回避 GPL。 |
| Windows 获取/分发 | 无官方 Windows 二进制；常用第三方 `poppler-windows`（如 oschwartz10612/poppler-windows，分发为 zip 含多个 exe + DLL，约 30-40MB）。需把 `poppler/bin/*.exe + *.dll` 放进 `build/bin/` 随程序分发。**体积代价明显**（+30MB+）。 |
| 渲染保真 | 高（基于 xpdf/Cairo，业界常用）。CJK 支持需带字体。 |
| CGO vs exe | **调 exe**（`exec.Command("pdftoppm", "-png", "-r", "150", "-f", page, file, out)`），零 CGO，与项目零 CGO 现状契合。 |
| 工作量 | 中。Go 包 `pdftoppm` 子进程 + 前端图片预览（项目已有 `kind==='image'` 的 `<img>` 分支可复用）。 |
| 结论 | **备选，仅当 B/C 均不可行且法务认可 GPL 子进程用法时**。 |

### 4.3 A-2：mupdf（`mupdf-go` / `mutool draw`）

| 维度 | 评估 |
|---|---|
| 许可证 | **AGPL-3.0**（mupdf 上游 Artifex）。**强传染**：AGPL 即便网络服务也触发开源义务，闭源商业分发几乎不可能。商业授权另购（Artifex 商业 license 费用高）。 |
| Windows 构建 | `mupdf-go` 是 CGO 绑定，需 C 工具链交叉编译 mupdf，**Windows 下 CGO 构建复杂度高**，破坏项目零 CGO 现状。 |
| 结论 | **金融背景直接排除**（AGPL + CGO 双重不利）。 |

### 4.4 A-3：Ghostscript（`gs`）

| 维度 | 评估 |
|---|---|
| 许可证 | **AGPL-3.0**（自 v9.07 起上游 Artifex 改 AGPL）。同 mupdf，强传染。 |
| 结论 | **金融背景直接排除**。 |

### 4.5 A-4：pdfcpu（Go 原生）

| 维度 | 评估 |
|---|---|
| 许可证 | Apache-2.0（pdfcpu，宽松，无传染）。 |
| 能力 | **关键限制：pdfcpu 只能拆/合/提取/优化/水印（页面级处理），不能把页面光栅化为位图**——它没有 PDF→图片的渲染/光栅化引擎。 |
| 结论 | **无法满足"转图片预览"需求**。可作为 PDF 页面提取/处理的辅助库，但**不能用于渲染预览**。 |

### 4.6 方案 A 综合结论

在金融 + 零 CGO + 单文件分发约束下，A 方案无可干净落地的子方案：
- poppler GPL（争议）、mupdf/Ghostscript AGPL（排除）、pdfcpu 无渲染能力。
- 即使接受 poppler 子进程，也要 +30MB 外部二进制，且 GPL 合规风险。

---

## 5. 方案 D / E：不推荐

### 5.1 D：非 pdfjs 的前端 PDF 渲染库

- **`pdf-lib`**：只能**创建/编辑** PDF，**不能渲染/预览**，排除。
- **`react-pdf`/`vue-pdf`** 等上层封装：底层就是 pdfjs-dist，**同样会触发双实例**，排除。
- **`pdf.js` 的其他 fork**：本质同源。
- 结论：纯前端能渲染 PDF 的库，底层几乎全是 pdfjs 或需 WASM + worker，**仍有同类风险**，不推荐。

### 5.2 E：纯 Go PDF 光栅化库

- **现状**：Go 生态中**没有成熟的纯 Go PDF 光栅化库**。能渲染 PDF 的 Go 库（`mupdf-go`、`fitz`、`goldmark-pdf` 渲染端）全部依赖 C 引擎（mupdf/MuPDF/Poppler），即 CGO。
- `unidoc`（商业）：能渲染但**商业付费授权**（金融背景虽不忌讳付费，但引入商业闭源依赖与"避免付费"约束冲突），且渲染能力有限。
- 结论：**无可用的纯 Go 方案**，排除。

---

## 6. Caveats / 实现阶段需验证的点

1. **方案 B/C 的本地 PDF URL 映射（最关键实测点）**：Wails v2 的 AssetServer（`assetsHandler` 自定义 handler）能否稳定地把任意本地文件路径（如 `D:\...\xxx.pdf`）映射成 `http://wails.localhost/pdf/...` 供 iframe/object 加载，且在 `wails dev`（vite dev server origin）与 `wails build`（`wails.localhost`/`file://`）两态行为一致。**实现前先做最小验证**：写一个 handler 返回 PDF 二进制 + `Content-Type: application/pdf`，iframe 能否加载渲染。
2. **iframe 同源与 viewer worker**：方案 B 下，viewer 内部创建 Worker 的路径相对 viewer.html 所在目录；放在 `public/pdfjs-viewer/web/` 时，dev 态 vite 会原样服务 `public/`，worker 相对路径解析需确认。若 dev 态 worker 起不来，viewer 会回退（但因为是隔离 iframe + 官方完整 worker 配置，回退不影响主页面，最坏只是该 iframe 内性能下降，**不会像之前那样崩主应用**）。
3. **大 PDF 性能**：方案 B/C 下，大 PDF 的初始加载由浏览器内置/官方 viewer 处理，本身支持按需加载页（range request / 流式），但本地文件 URL 映射若不支持 range request，会一次性读全文件。Handler 实现需支持 HTTP Range（Wails AssetServer 默认是否支持 Range 需确认）。
4. **viewer 版本锁定**：方案 B 用的官方 viewer 是某一版本，CVE/兼容性随版本变化；建议锁定一个稳定版本（如 pdfjs-dist v4 LTS 线的某 tag 对应的官方 release zip），并在 README 记录版本。
5. **未实机验证**：本研究基于对 pdfjs 官方 viewer 架构、Wails v2 AssetServer、WebView2 PDF 能力的既有确定认知，**未在本机跑通 iframe+viewer 最小用例**；方案 B 的 §6.1/§6.2 是落地前必须先验证的两个最小实验。
6. **方案 B 资源体积**：viewer + worker + locale 约数 MB，进入 `dist/` 被 embed 进 exe，单文件体积增加几 MB，可接受。

---

## 7. 相关文件

| 文件 | 说明 |
|---|---|
| `frontend/src/components/FilePreviewRenderer.vue` | 现有预览组件，已有 image/docx/text/markdown 分支，PDF 分支需新增（方案 B 加一个 iframe 分支即可复用现有工具栏外壳或不复用） |
| `main.go` | 已 `//go:embed all:frontend/dist`，方案 B 的 viewer 静态资源放入 `frontend/public/` 后会被自动打包 |
| `build/bin/` | 已用于随程序分发二进制（方案 A 若选 poppler 会放这里，但不推荐 A） |
| `app.go` | 前后端桥接，方案 B/C 的 PDF 文件 URL 映射 handler（AssetServer `assetsHandler`）在此或单独文件配置 |
| `.trellis/tasks/archive/2026-06/06-23-file-preview/research/pdfjs-v6-pagesnumber-error.md` | 已确认的前端 pdfjs 双实例根因档案（本研究的前提） |

---

## 8. 外部参考（实现阶段核对）

- **mozilla/pdf.js 官方仓库**：`web/viewer.html` 完整 viewer 应用结构、`?file=` query 参数约定、官方 release zip（`pdfjs-x.y.z-dist.zip`）。
- **pdf.js 许可证**：Apache-2.0（`LICENSE`）。
- **Wails v2 AssetServer / `assetsHandler`**：自定义 HTTP handler、本地资源 URL 映射、Range request 支持。
- **WebView2 PDF viewing**：Chromium 内置 PDF 渲染、iframe/object 行为、不同 Runtime 版本差异。
- **poppler / mupdf / ghostscript / pdfcpu 许可证**：GPL-2/3、AGPL-3、Apache-2.0 的传染性边界（金融合规口径）。

> 说明：本研究方案结论基于对上述开源生态的确定认知；落地前 §6 列出的最小验证用例需实机跑通后再进入实现阶段。
