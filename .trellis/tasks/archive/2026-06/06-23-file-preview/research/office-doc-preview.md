# Research: Go + Wails 桌面应用内预览 Office 文档（Word/PowerPoint/Excel）

- **Query**: 在 Go 1.24 + Wails v2.12 + Vue3 桌面应用中，应用内预览 .docx/.doc、.pptx/.ppt、.xlsx/.xls 的方案选型
- **Scope**: 外部技术选型（mixed：少量核对项目现状）
- **Date**: 2026-06-23

## 项目现状（核对结论）

- 后端 `go.mod`：`go 1.24.0`，已依赖 Wails v2.12.0，**无任何 Office 相关 Go 库**（excelize / unioffice 均未引入）。
- 前端 `package.json`：Vue 3.5 + Element Plus，**无任何 Office 预览 JS 库**（mammoth / docx-preview / xlsx / handsontable 均未引入）。
- 结论：本任务为从零选型，无既有迁移成本。

---

## 方案矩阵（按维度对比）

### 维度说明
保真度 = 渲染后与 Office 原版排版的一致程度；运行时依赖 = 用户机器是否需额外装东西；许可证 = 能否商用。

| 方案 | 路径 | 保真度 | 运行时依赖 | 许可证 | 实现复杂度 | 旧格式 .doc/.ppt/.xls | Go+Wails 契合度 |
|---|---|---|---|---|---|---|---|
| **A 纯前端 JS** | 浏览器/Wails webview 内直接渲染 | 低-中（按类型差异巨大） | 无 | 全部宽松（见下） | 中 | **基本不支持**（OOXML 库读不了二进制 OLE 格式） | 高（最契合，前端 npm 即可） |
| **B 后端 Go 库读取** | Go 解析后传 JSON/HTML 给前端 | 低（仅数据，丢排版） | 无 | excelize 宽松；**unioffice 商业付费** | 中 | **不支持**（二进制旧格式） | 高（原生 Go） |
| **C LibreOffice 转换流水线** | soffice --convert-to pdf/html → pdf.js 或 iframe 渲染 | **高（最接近原版）** | **需装 LibreOffice（约 350MB）** | MPL-2.0（可商用） | 中-高 | **支持**（LO 兼容旧格式） | 中（需跨进程调 soffice，处理隐藏窗口/进程） |
| **D Windows Office COM** | 调 Word/Excel/PPT COM 自动化导出 | **最高（就是 Office 本身）** | **需装正版 MS Office** | 受限于用户已购 Office | 高（仅 Windows、COM/PowerShell、版本绑定） | 支持 | 低（与"跨平台 Go"理念冲突，强绑 Windows+Office） |
| **E 降级外部打开** | 调系统默认程序打开 | 原生（非内嵌） | 依赖用户机器已装查看器 | 无 | 极低 | 取决于用户查看器 | 高（一行命令） |

---

## 各方案关键事实核实（含来源）

### 方案 A：前端纯 JS 库

#### Word（.docx）
| 库 | 版本 | 许可证 | 保真度 | 来源 |
|---|---|---|---|---|
| **mammoth** | 1.12.0 | **BSD-2-Clause** | 低（docx→语义化 HTML/Markdown，丢弃样式、字体、复杂排版） | npm registry `mammoth` latest |
| **docx-preview** | 0.3.7 | **Apache-2.0** | 中（尽量还原 Word 原始样式，比 mammoth 更接近原版） | npm registry `docx-preview` latest |

- 两者**都只读 OOXML (.docx)**，对旧二进制 .doc **完全不支持**。
- 来源：npm registry（`https://registry.npmjs.org/mammoth/latest`、`/docx-preview/latest`）。

#### Excel（.xlsx）
| 库 | 版本 | 许可证 | 能力 | 来源 |
|---|---|---|---|---|
| **SheetJS (xlsx)** | 0.18.5（npm 上的社区版） | **Apache-2.0** | 解析 xlsx/xls/csv，读出 sheet 数据；npm 社区版**不含写入/导出**的高级能力，写入需 Pro 或自行处理 | npm registry `xlsx`；SheetJS 官方 docs.sheetjs.com |
| **Luckysheet / x-spreadsheet** | luckysheet 2.1.13 | luckysheet（MIT，社区维护偏弱） | 类 Excel 网格 UI 渲染，可做"可交互电子表格"预览 | npm registry |
| **Handsontable** | 17.1.0 | **⚠️ 双重许可：非商用免费，商用需付费购买商业授权** | 成熟数据网格 | GitHub `handsontable/handsontable` LICENSE.txt（明文：商业用途需商业授权） |
| **vue3-table-lite** | — | MIT | 轻量表格组件，适合"只读表格预览"，非类 Excel 交互 | npm |

- **关键风险**：Handsontable 商用必须付费，本项目虽是单机工具但若对外分发可能构成商用，**应避免**或确认许可后再用。
- SheetJS 注意：npm 上 `xlsx@0.18.5` 是社区版（Apache-2.0），但官方已将**最新版迁到自有 CDN**，npm 不再更新；社区版读取能力足够预览，写入受限（来源：SheetJS 官方站 sheetjs.com）。
- SheetJS **社区版能读 .xls（旧二进制 BIFF）**，这是它相比 Word/PPT 库的一个优势。

#### PowerPoint（.pptx）—— 成熟度核实（重要）
| 库 | 状态 | 许可证 | 说明 | 来源 |
|---|---|---|---|---|
| **pptx2html** | ⚠️ **已停滞**（npm 最后修改 2022-05-13，3+ 年未更新） | MIT | 不建议新项目采用 | npm registry `pptx2html` time 字段 |
| **pptxtojson** | ✅ **仍在维护**（npm 最后修改 2026-05-31，最新 2.0.4） | MIT | pptx→JSON，可配合自定义渲染；是 pptx2html 的后继/替代 | npm registry `pptxtojson` time 字段 |
| **pptxjs（gottox 等）** | 成熟度普遍偏低 | — | 纯前端渲染 pptx 整体生态薄弱，无工业级成熟方案 | npm/社区现状 |

- **结论**：PPT 是三类里前端方案最弱的。即便用 pptxtojson，渲染保真度仍远低于 Word（docx-preview）和 Excel（SheetJS）。PPT 想高保真**必须走方案 C（LibreOffice）**。

### 方案 B：后端 Go 库

| 库 | Stars | 最后更新 | 许可证 | 能力 | 来源 |
|---|---|---|---|---|---|
| **xuri/excelize** | ~20680 | 2026-06-22（活跃） | **BSD-3-Clause**（商用友好） | 读/写 xlsx/xlsm/xlam；**不转 HTML/PDF**，只产出数据/单元格值/样式，需前端自行渲染 | GitHub API `qax-os/excelize` |
| **unidoc/unioffice** | ~4881 | 2026-05-29 | ⚠️ **商业产品（UniDoc EULA），需 license key**；有 Free Tier（metered，需注册 API key），完整商用需付费（白金版示例发票 $2,400） | 创建/处理 docx/xlsx/pptx；**不直接输出 HTML/PDF** | GitHub `unidoc/unioffice` LICENSE.md + README（明文商业产品）；unidoc.io 官网发票示例 |

- **结论**：Go 库整体只能"读数据"，不能"渲染成可见文档"。即使 excelize 读出单元格，前端仍要自己画表格。
- **强烈建议排除 unioffice**（商业付费，单机工具不值得）；优先用 **excelize**（BSD-3，免费商用）。
- 所有 Go 库**都不支持旧二进制 .doc/.ppt/.xls**（只认 OOXML）。

### 方案 C：LibreOffice 转换流水线（保真最高）

- **调用方式**：`soffice --headless --convert-to pdf --outdir <dir> <input.docx>`
  - 也可 `--convert-to html` 直接转 HTML（但 HTML 保真度低于 PDF）。
- **Windows 隐藏窗口**：用 Go 的 `os/exec` 调 `soffice.exe`（带 `--headless` 即不弹 UI）；为避免 console 闪烁，可用 `syscall.SysProcAttr{HideWindow: true}`（Windows 专属，与项目已用的 `console_windows.go` 同源思路）。
- **许可证**：MPL-2.0，**可商用、可随软件分发**（来源：DocumentFoundation wiki Development/Headless）。
- **旧格式支持**：LibreOffice **能读 .doc/.ppt/.xls 旧二进制格式**并转 PDF，这是它相比 A/B 方案的核心优势。
- **体积成本**：LibreOffice 安装包约 300-400MB，需**明确告知用户**作为前置依赖（或安装时检测/引导安装）。
- **风险**：跨进程调用有启动开销（首次转换约 1-3 秒，因需启动 soffice 进程）；可考虑常驻一个 soffice 实例（`--accept` socket）做转换服务以降低延迟。
- **二次渲染**：转出的 PDF 用 **pdf.js / pdfjs-dist**（Apache-2.0）在 Wails webview 内渲染，实现"应用内预览"。

### 方案 D：Windows Office COM 自动化

- **可行性**：仅 Windows；需用户机器**装有正版 MS Office**；通过 Go 调 COM（如 `go-ole`）或起 PowerShell 脚本驱动 Word/Excel/PowerPoint Application 对象导出 PDF/HTML。
- **许可证**：依赖用户已购 Office 授权，**不可随软件捆绑 Office**；不同 Office 版本（2016/2019/365）COM 接口略有差异，需做版本兼容。
- **保真度最高**（直接用 Office 引擎），但**强绑 Windows + Office**，与"跨平台 Go 桌面工具"理念冲突，且用户未必装 Office。
- **结论**：除非目标用户群 100% 是装了 Office 的 Windows 用户，否则不推荐作为主方案。

### 方案 E：降级外部打开（兜底）

- 调用系统默认程序打开文件（Windows：`os.StartProcess` / `rundll32` / `start`；Go 可用 `exec.Command("cmd", "/c", "start", "", path)`）。
- 非内嵌预览，但**零实现成本、零依赖、对所有格式通用**。
- 建议作为**所有方案的兜底**：当应用内无法预览（加密/宏/超大/格式不支持）时，提供"用默认程序打开"按钮。

---

## 各类型最佳渲染路径（核心结论）

| 文档类型 | 推荐路径 | 理由 |
|---|---|---|
| **.docx（Word）** | 优先 **docx-preview**（前端，中保真）；要高保真则走 **C（LibreOffice→PDF）** | docx-preview 在 OOXML 里保真最好且零依赖；mammoth 仅适合"只要文字内容"场景 |
| **.doc（旧版 Word）** | **只能走 C（LibreOffice→PDF）或 E（外部打开）** | 前端/Go 库均不支持二进制 .doc |
| **.xlsx（Excel）** | **SheetJS（xlsx）读数据 + vue3-table-lite/Luckysheet 渲染表格**；或 **excelize 后端读 + 前端表格**；高保真走 **C** | SheetJS 社区版读取足够，渲染靠前端表格组件 |
| **.xls（旧版 Excel）** | **SheetJS 社区版能读 .xls**（少数能读旧格式的库）；或走 **C/E** | SheetJS 支持 BIFF 旧格式，是 A 方案里唯一能吃旧格式的 |
| **.pptx（PowerPoint）** | **走 C（LibreOffice→PDF）最务实**；前端仅 pptxtojson（保真低）可作轻量备选 | 前端 PPT 渲染生态最弱，无工业级方案 |
| **.ppt（旧版 PPT）** | **只能走 C（LibreOffice→PDF）或 E（外部打开）** | 前端/Go 库均不支持二进制 .ppt |

---

## 整体推荐组合（2-3 套）

### 组合 1：轻量 MVP（前端库 + 降级打开）— 推荐作为首版
- **构成**：docx-preview（docx）+ SheetJS+xlsx + vue3-table-lite（xlsx/xls）+ pptxtojson（pptx，保真低）+ E（外部打开作兜底）
- **保真度**：Word 中、Excel 中、PPT 低；旧格式全部走外部打开
- **运行时依赖**：**无**（纯前端 npm）
- **许可证**：全部 Apache-2.0 / MIT / BSD，**可商用无风险**（注意**别用 Handsontable**）
- **工作量**：低（约 2-4 天前端集成）
- **适用**：快速上线、用户主要是 .docx/.xlsx 且可接受 PPT/旧格式用外部程序打开

### 组合 2：完整高保真版（LibreOffice 流水线）— 推荐作为目标版
- **构成**：Go 后端调 **soffice --headless --convert-to pdf**（所有 Office 格式统一转 PDF）+ 前端 **pdf.js** 渲染 + E 兜底
- **保真度**：**高（接近原版）**，且**统一支持 .doc/.ppt/.xls 旧格式**
- **运行时依赖**：**需用户安装 LibreOffice（约 350MB）**——必须在安装/首次使用时检测并引导
- **许可证**：MPL-2.0（LO）+ Apache-2.0（pdf.js），**可商用**
- **工作量**：中-高（跨进程调用、隐藏窗口、soffice 常驻优化、PDF 渲染集成，约 1-2 周）
- **适用**：用户对保真度要求高、文档格式杂（含旧格式）、可接受装 LibreOffice

### 组合 3：混合分层（推荐生产采用）
- **构成**：前端轻量库优先（组合 1）→ 检测到本地已装 LibreOffice 则自动升级为 PDF 预览（组合 2）→ 都失败时 E 外部打开
- **保真度**：自适应（有 LO 则高，无则中/低）
- **运行时依赖**：LO 可选（不强制）
- **工作量**：中（在组合 1 基础上加 LO 检测与转换分支，约 1 周）
- **适用**：兼顾"开箱即用"与"高保真"，体验最佳；**推荐作为最终交付方案**

---

## 风险清单

| 风险 | 影响范围 | 应对 |
|---|---|---|
| **旧二进制格式 .doc/.ppt/.xls** | 方案 A、B 全部不支持 | 走 C（LO）或 E（外部打开）；SheetJS 可救 .xls |
| **宏文档（.docm/.xlsm/.pptm）及 VBA** | 宏代码无法渲染，且有安全风险 | 仅静态预览内容，**不执行宏**；LO 转换默认不执行宏 |
| **加密/密码保护文档** | 所有方案均无法直接读 | 检测到加密则提示用户，降级为 E 外部打开（由 Office/LO 弹密码框） |
| **大文件 / 复杂排版** | docx-preview/pptxtojson 可能卡顿或丢版；LO 转换耗时长 | 加文件大小阈值（如 >20MB），超限直接提示并降级 E |
| **Handsontable 商用许可** | 若商用需付费 | **避免使用**，改 vue3-table-lite 或 Luckysheet |
| **unioffice 商业付费** | 引入即需 license key | **排除**，用 excelize（BSD-3）替代 |
| **SheetJS 社区版写入受限** | 仅预览场景无影响（只读） | 预览只读无风险；若未来需导出 xlsx 再评估 Pro |
| **LibreOffice 体积大** | 用户体验/分发成本 | 作为可选依赖，安装时检测+引导，不强制捆绑 |
| **跨进程 soffice 启动延迟** | 首次转换慢 | 常驻 soffice `--accept` socket 服务复用进程 |

---

## Caveats / Not Found

- **pptxjs（gottox/pptxjs 等具体某仓库）** 未逐一核实每个 fork 的活跃度；结论"纯前端 PPT 渲染整体薄弱"基于 npm 生态现状，未对每个小众 fork 做逐一尽调。若要选用某个 pptx 库，建议单独再核其 GitHub last-commit / issues。
- **Excel COM / Office COM 的具体 Go 绑定库**（如 `go-ole/go-ole`）未深入核实版本兼容矩阵；方案 D 仅做可行性判断，落地前需单独调研 COM 调用与 Office 版本兼容。
- **pdf.js 在 Wails webview 内的具体性能**（超大 PDF 渲染）未实测；建议组合 2/3 落地时做性能基线测试。
- 本研究的 npm/GitHub 元数据抓取时间为 2026-06-23，版本与"最后更新"为该时点快照。

---

## 来源汇总

- npm registry：`https://registry.npmjs.org/{mammoth,docx-preview,xlsx,pptx2html,pptxtojson,handsontable,luckysheet,vue3-table-lite}/latest`
- GitHub API：`https://api.github.com/repos/qax-os/excelize`、`/unidoc/unioffice`
- GitHub raw：`https://raw.githubusercontent.com/handsontable/handsontable/master/LICENSE.txt`（Handsontable 商用许可明文）
- GitHub raw：`https://raw.githubusercontent.com/unidoc/unioffice/master/LICENSE.md`（unioffice 商业产品声明）+ `README.md`（UniDoc EULA 徽章）
- unioffice 官网：`https://unidoc.io/`（白金版 $2,400 发票示例，证实付费商业）
- SheetJS 官方：`https://docs.sheetjs.com/`、`https://sheetjs.com/pro`
- LibreOffice：`https://wiki.documentfoundation.org/Development/Headless`（headless --convert-to 用法）
