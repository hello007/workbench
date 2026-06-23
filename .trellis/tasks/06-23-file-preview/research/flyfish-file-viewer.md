# Research: Flyfish Viewer（@flyfish-group/file-viewer3）一站式预览组件

- **来源**：用户指定 https://gitee.com/flyfish-dev/file-viewer（用户提出"调研一下再出方案"）
- **调研方式**：inline 抓取 Gitee 仓库 README 全文（本会话内完成）
- **日期**：2026-06-23

## 0. 一句话结论

Flyfish Viewer 是一个**纯前端、Serverless、全格式（194 扩展名 / 23 预览链路）** 的文档预览组件，**Apache-2.0** 许可证，提供 Vue3 原生组件包。它能用一个 `<file-viewer>` 组件**替代我们原本要手搓的全部渲染分支**（图片 / PDF / Office / 旧格式 / Markdown / 代码高亮），且**无需安装 LibreOffice、无需后端转码**。这是一个比"自研拼装"和"LibreOffice 转 PDF"都更优的一站式方案，前提是解决两个集成点（见 §4）。

## 1. 关键事实（来自官方 README，已核实）

| 项 | 内容 |
|---|---|
| 定位 | 纯前端文档预览，**不依赖后端转码服务、不依赖 LibreOffice 守护进程、不依赖额外转码链路** |
| 许可证 | **Apache-2.0**（可商用、可二开，需保留版权与来源说明） |
| Vue3 包 | `@file-viewer/vue3`（标准）/ `@flyfish-group/file-viewer3`（兼容历史名），当前 `2.0.0` |
| 集成方式 | `app.use(FileViewer)` + `<file-viewer :url="url" />` 或 `:file="file"` |
| 输入 | `url?: string`（同源 URL，PDF 支持 Range 渐进）/ `file?: File`（Blob/ArrayBuffer 包装） |
| 渲染策略 | 重型解析器（PDF/Office/Typst/压缩包等）**按需异步加载**，不进首屏 |
| 容器要求 | 填满父容器，需父容器提供稳定高度 |

## 2. 格式覆盖（核对用户需求是否全覆盖）

| 用户需求 | Flyfish 支持 | 渲染器 | 结论 |
|---|---|---|---|
| 图片 jpg/png/bmp | gif/jpg/jpeg/bmp/tiff/png/svg/webp/avif/ico/heic/heif | 原生 + heic2any | ✅ 覆盖，且更广 |
| Word | docx/docm/dotx/dotm + **doc/dot**（msdoc-viewer）+ rtf/odt | docx-preview + 可选 Worker | ✅ 含旧 .doc |
| PPT | pptx/pptm/potx/potm/ppsx/ppsm/odp | @aiden0z/pptx-renderer | ✅ |
| Excel | xlsx/xltx + **xlsm/xlsb/xls/xlt/csv/ods/numbers** | styled-exceljs + 虚拟滚动 | ✅ 含旧 .xls |
| PDF | pdf | pdfjs-dist（Range、旋转、目录） | ✅ |
| 文本 txt/json/sql/md 等 | json/json5/yaml/toml/ini/proto/js/ts/vue/go/rs/py/sql/diff/log 等 + md/markdown | highlight.js + Markdown 阅读样式 | ✅ |

**用户列出的全部类型，Flyfish 均原生覆盖，且额外支持旧格式（.doc/.xls）——这是自研拼装方案做不到、只有 LibreOffice 才能做到的。**

## 3. 与本项目的契合度

- **技术栈零冲突**：本项目 Vue 3.5 + Vite 8 + Element Plus，Flyfish Vue3 包即装即用。
- **取代自研拼装**：若用 Flyfish，则 markdown-it / highlight.js / CodeMirror / docx-preview / SheetJS / pdfjs-dist **全部无需单独引入**，前端代码从"多渲染分支"简化为"一个组件 + 喂数据"。
- **取代 LibreOffice**：纯前端渲染旧 .doc/.xls，无需引导用户安装 350MB 的 LibreOffice。
- **桌面应用友好**：README 明确适合"需要离线能力的业务场景"，契合 Wails 单机工具。

## 4. 关键集成点（必须在落地前解决 / 验证）

### 4.1 数据喂入方式（核心架构决策）
Flyfish 输入是 `url` 或 `file`，但本项目文件是**本地磁盘路径**。两条路径：

| 方式 | 做法 | 优劣 |
|---|---|---|
| **Blob 方式** | 后端读字节返回前端，前端 `new File([bytes], name)` → `:file` | 简单；但大文件/PDF 全量 base64 有内存峰值，且失去 pdf.js Range 渐进加载 |
| **本地 HTTP 服务（推荐）** | Wails 用 `AssetServer.Handler` 或起一个 `net/http` 随机端口，把本地文件暴露为 `http://127.0.0.1:port/...` → `:url` | 支持 Range / 按需加载 / worker；大文件友好；但引入本地服务与路径安全校验 |

> 注：这是 Flyfish 方案落地的**最大工作量与风险点**，需在实现期实测 Wails 环境下的可行性。

### 4.2 Worker / WASM 静态资源部署
Flyfish 内部依赖多个静态资源：`vendor/docx/docx.worker.js`、`vendor/xlsx/sheet.worker.js`、`vendor/libarchive/worker-bundle.js`、pdf.worker、Typst/CAD WASM 等。需：
- 用 `file-viewer-copy-assets` 把资源复制到前端 `public/`
- 在 Wails 打包后确认这些 worker/wasm 路径可正确加载（Vite + Wails 静态资源路径需实测）
- 必要时通过 options 显式传 `workerUrl` / `wasmUrl`

### 4.3 Vue3 组件与 Element Plus 共存
需确认 `<file-viewer>` 与现有 Element Plus / Vue Router 无样式与全局冲突；组件"填满父容器"特性要求预览区父容器有稳定高度（呼应"预览区提升为主区域"）。

### 4.4 许可证合规
Apache-2.0 要求在项目 README / About 中保留 Flyfish Viewer 版权与来源声明。

## 5. 三方案对比（更新版）

| 维度 | A 自研拼装 | B LibreOffice 转 PDF | **D Flyfish 一站式** |
|---|---|---|---|
| 用户需求格式覆盖 | 拼，PPT 弱、不支持旧格式 | 全（含旧格式） | **全（含旧格式），且 194 扩展名** |
| 运行时依赖 | 无 | **需装 LibreOffice ~350MB** | **无** |
| 保真度 | 中/低 | 高 | 中-高（docx-preview / exceljs 级别） |
| 实现工作量 | **高**（多库 + 多渲染分支） | 中-高 | **低**（一个组件 + 喂数据） |
| 后续维护 | 高（N 个库各自升级） | 中 | 低（跟随 Flyfish 升级） |
| 许可证风险 | 需逐一核查（避 unioffice/Handsontable） | MPL-2.0 + Apache | Apache-2.0（需保留来源声明） |
| 主要风险 | 自己踩各库的坑 | LO 安装/进程/体积 | 第三方组件成熟度、Wails 下 worker/wasm 集成、数据喂入 |

## 6. 风险与 Caveats（诚实标注）

- **依赖树体积（POC 实测，2026-06-23）**：`npm install` resolve 阶段即拉取 **40+ 直接依赖**，含 `three`、`@myriaddreamin/typst.ts`、`@flyfish-dev/cad-viewer`、`libarchive.js`、`sql.js`、`@excalidraw/excalidraw`、`pdfjs-dist`、`styled-exceljs`、`ag-psd`、`hls.js` 等——**即使只要图片/PDF/Office/文本，CAD/3D/Typst/压缩包/邮件等全格式重型依赖（含多个 WASM 二进制）仍全量进入 `node_modules`**。安装耗时显著（resolve + 下载量达数 GB），`wails build` 打包体积与构建时间预计明显上升。这是选型时须重点评估的隐性成本，POC 将给出 `node_modules` 与打包 exe 的实测体积。
- **第三方组件成熟度**：Flyfish Viewer 在 Gitee Star 仅 6、Fork 3，仓库较新（1 天前仍在提交）。其**底层渲染器是成熟库**（docx-preview / pdfjs-dist / exceljs 封装），但**外层封装的稳定性、issue 响应、长期维护需谨慎评估**。建议落地前用真实业务文件（含复杂 Word/PPT）做回归验收。
- **Wails 环境未实测**：worker/wasm 资源在 Wails 打包后的加载、本地 HTTP 服务的数据喂入，均需实现期验证（README 主要面向普通 Web 部署）。
- **包体积**：core + 按需加载，仍是较重依赖；`wails build` 产物体积需评估。
- **PPT 复杂特效**：README 自述"复杂 Office 特效仍建议用真实业务文件做回归"，PPT 保真可能不及 LibreOffice。

## 7. 推荐

**采用 Flyfish 一站式方案作为主方案**，理由：
1. 用一个成熟组件覆盖用户全部需求（含旧格式），省去手搓多渲染分支的高昂工作量；
2. 纯前端、无后端依赖，彻底避开 LibreOffice 350MB 体积与安装引导；
3. Apache-2.0，许可证清晰可商用。

落地时优先解决 §4 的两个集成点（数据喂入 + worker/wasm 部署），并用真实文件做回归验收。

> 若用户对"第三方组件依赖"有顾虑（金融/银行环境常对引入新依赖审慎），可回退到 A 自研拼装（可控但工作量大）或 B LibreOffice（高保真但重）。
