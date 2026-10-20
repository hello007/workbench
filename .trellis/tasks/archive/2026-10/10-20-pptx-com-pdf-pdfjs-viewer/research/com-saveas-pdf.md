# Research: pptx 经 Office COM 转 PDF（SaveAs / ExportAsFixedFormat / WPS 兼容性）

- **Query**: PowerPoint COM SaveAs PDF 正确调用方式、WPS 兼容性、链接媒体已知问题、目标文件覆盖行为、转换后 Close/Save 行为
- **Scope**: mixed（内部代码/规范沉淀 + 外部官方文档与社区资料）
- **Date**: 2026-10-20

## 结论先行（推荐调用方式）

**首选 `Presentation.SaveAs(pdf绝对路径, 32)`（ppSaveAsPDF=32），链路为：防弹窗三板斧（DisplayAlerts=1 + AutomationSecurity=3）→ `Presentations.Open(absPath, msoTrue(-1), msoFalse(0), msoFalse(0))`（只读、无窗口）→ `SaveAs(pdfAbsPath, 32)` → `Close`；目标路径必须绝对路径（COM 服务器进程按自身 cwd 解析）。该调用不写回源 pptx、可静默覆盖已存在的目标 PDF，且 WPS 兼容层对本常量体系已真机实证（24 值成功，32 待冒烟一并验证）；若真机验证发现 WPS 对 32 异常，回退方案为 `ExportAsFixedFormat(pdfAbsPath, 2)`（ppFixedFormatTypePDF=2）。**

---

## 1. PowerPoint COM（PowerPoint.Application）

### 1.1 ppSaveAsPDF 常量确认

官方 PpSaveAsFileType 枚举确认 **`ppSaveAsPDF = 32`**（同表相关值：ppSaveAsOpenXMLPresentation=24、ppSaveAsXPS=33、ppSaveAsDefault=11）。

来源：[PpSaveAsFileType enumeration (PowerPoint) | Microsoft Learn](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.ppsaveasfiletype)

### 1.2 SaveAs 与 ExportAsFixedFormat 的差异

| 维度 | `SaveAs(FileName, FileFormat, EmbedFonts)` | `ExportAsFixedFormat(Path, FixedFormatType, Intent, ..., UseISO19005_1)` |
|---|---|---|
| 官方语义 | 保存从未保存过的演示文稿，或以不同名称/格式另存 | **官方明文：等价于 UI 的「Save As PDF or XPS」发布命令**，产出静态视图副本 |
| 参数控制面 | 仅文件名 + 格式 + 字体嵌入 | Intent（Screen 默认/Print）、PrintHiddenSlides（**默认 msoFalse 不导出隐藏页**）、页范围（PrintRange 必传可传 Nothing）、BitmapMissingFonts（默认 True）、UseISO19005_1（PDF/A，默认 False）等 15 参 |
| FixedFormatType | FileFormat=32（PDF）/ 33（XPS） | ppFixedFormatTypePDF=**2** / ppFixedFormatTypeXPS=1 |
| 状态副作用 | 保存语义：Presentation 的文件关联（FullName）重定向到新文件 | 发布语义：不动 Presentation 的文件关联（社区共识，官方未明文） |
| 渲染保真 | 同一渲染管线，两法产出的 PDF 视觉一致（社区共识，无官方明文区分） | 同左；Intent=Print 会按打印意图优化 |

来源：[Presentation.SaveAs](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.presentation.saveas)、[Presentation.ExportAsFixedFormat](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.presentation.exportasfixedformat)

### 1.3 只读无窗口打开后能否转 PDF

**能。** `ReadOnly:=msoTrue` 只阻止「写回原文件」，不阻止导出到其他文件；`WithWindow:=msoFalse` 下 Slide.Export 级无窗口导出已在本仓真机验证（`service/assets_thumbs_windows.go:853` 同参数），SaveAs/ExportAsFixedFormat 同为 Presentation 级方法、不依赖窗口视图。社区大量转换器（PowerShell/Python win32com/Perl Win32::OLE）均采用「ReadOnly 打开 → 导出 PDF」模式（参见 SO 7607914、63817805）。

### 1.4 其他注意点

- **FileName 必须显式带 `.pdf` 扩展名**（省略时 PowerPoint 按字面名称落盘，产无扩展名文件；社区共识，中置信度）。
- `ExportAsFixedFormat` 的 `PrintRange` 参数官方标注 Required 但可传 Nothing（全范围）；late binding（go-ole IDispatch）下建议走 SaveAs 规避 15 参可选参数封送复杂度（SO 7607914 即因 late binding 传参形态报 `0x80020011`）。

## 2. WPS 演示（wpp.exe / KWPP.Application）

### 2.1 已实证的兼容事实（本仓真机）

| 事实 | 证据位置 |
|---|---|
| WPS 接管 `PowerPoint.Application` ProgID（标准 MS CLSID 存在但 LocalServer32 缺失，实际启动 wpp.exe）——**进程集合必须 {powerpnt.exe, wpp.exe}** | `docs/spec/assets-thumbs-com.md` §进程 diff |
| **WPS 兼容层接受 PowerPoint PpSaveAsFileType 常量体系：真机冒烟 `SaveAs(path, 24)` 成功** | `service/assets_thumbs_smoke_integration_windows_test.go:397`（`ppSaveAsOpenXMLPresentation=24`） |
| WPS 同名文件允许双开；Presentations.Count 不可靠 | `docs/spec/assets-thumbs-com.md` 双引擎兼容表 |
| COM 激活的 WPS 实例应用级默认不可见，须显式 `Visible=msoTrue`（SaveAs/Export 级无窗口路径不需要） | 同 spec §9.1 |
| 手写 OOXML zip fixture 被 WPS 拒开（Open E_FAIL 0x80020009）——**转换冒烟 fixture 一律 COM 自产** | `assets_thumbs_smoke_integration_windows_test.go:312-315` |
| 交给 COM 服务器的路径参数一律绝对路径（Export 相对路径 = 确定性「发生意外」，SaveAs/Open 同属此类调用） | `docs/spec/assets-thumbs-com.md` |
| CreateInstance 须 0x8001010A 忙态重试 5×1s；Export 瞬态 0x80020009 判定可复用 | `docs/spec/assets-thumbs-com.md` |

### 2.2 SaveAs PDF 兼容性（待验证项）

- **是否支持 `ppSaveAsPDF=32`：无权威文档佐证**。WPS 开放平台文档（open.wps.cn）为 JS 渲染 SPA 无法抓取；本环境网络限制下（DuckDuckGo/grep.app 不可达）未检索到 KWPP 对象模型 SaveAs PDF 的权威条目或社区 issue 原文。社区普遍认为 WPS 支持「输出为 PDF」且兼容 PpSaveAsPDF 常量，置信度低，**须真机验证**。
- 支持面判断依据：WPS 兼容层以 PowerPoint 对象模型为兼容目标（24 值已实证），「输出为 PDF」是 WPS 全家桶标配功能（免费个人版含 PDF 能力），32 值大概率可用，但**不可只凭推断上实现**。
- **实现建议**：转换方法内做「`SaveAs(pdfAbs, 32)` 失败 → `ExportAsFixedFormat(pdfAbs, 2)`」一次回退，真机冒烟（COM 自产 fixture）一次性验证两法；两者都被拒时返回降级错误（与 prd D4 瞬态重试链正交）。

## 3. 链接媒体 / 动画已知问题

| 项 | 行为 | 置信度 |
|---|---|---|
| 嵌入视频 | PDF 为静态格式，视频渲染为**海报帧/首帧占位**（官方无明文，社区共识） | 中 |
| 外链媒体（未嵌入、链接到本地/网络文件） | 渲染时由 COM 进程解析链接；**链接失效/不可达 → 空白或占位图**；WPS 与 PowerPoint 对链接解析行为可能不同（未验证） | 中 |
| 动画 | 渲染为**动画结束后的终态**静态画面 | 中（社区共识） |
| 隐藏页 | **默认不导出**（官方默认 PrintHiddenSlides=msoFalse）；SaveAs(32) 无参数控制此行为——本预览场景「隐藏页不出现在 PDF」需产品确认可接受性；要导出隐藏页只能走 ExportAsFixedFormat 传 msoTrue | 高（官方文档明文默认值） |
| 超链接 | 两法默认保留为 PDF 内可点击链接 | 中 |

## 4. SaveAs 目标文件已存在时的行为

- **静默覆盖**。COM 自动化无 UI 确认环节，目标 PDF 已存在时直接覆盖写（社区共识，中高置信度）；SaveAs 与 ExportAsFixedFormat 行为一致。
- 例外：目标文件被其他进程锁定/只读属性时报错（常规文件 IO 语义）。
- 对本项目无阻碍：缓存目录产物由本服务独占写，若想规避「覆盖半旧文件期间的读者读到撕裂 PDF」，可用「先写临时名 → 成功后 rename」模式（与 `util.SaveJSON` 原子写同思路，PDF 为二进制须自行实现 temp+rename）。

## 5. 转换后 Presentation 的 Close / Save 行为

- **两法均不写回源 pptx 文件**（导出 PDF 是生成新文件，源文件 mtime/内容不变）。
- `SaveAs(32)` 的保存语义副作用：Presentation 的 FullName 重定向为 PDF 路径、Saved 置 true——随后的 Close 无保存提示；对本仓「短会话打开→转换→Close」链路无实际影响。
- `ReadOnly:=msoTrue` 打开时**源文件零风险由 ReadOnly 本身保证**：无论后续是否误调 Save，都不会写回源文件（ReadOnly 下 Save 报错/拒绝）。
- **不应调用 Save、无需调用 Save**；Close 即可。收口沿用现有约定：自启实例 Quit、复用用户实例禁 Close/Quit（`docs/spec/assets-thumbs-com.md`）。

## 来源链接

- [PpSaveAsFileType enumeration | Microsoft Learn](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.ppsaveasfiletype) — ppSaveAsPDF=32 权威确认
- [Presentation.SaveAs method | Microsoft Learn](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.presentation.saveas) — SaveAs 签名与 remarks
- [Presentation.ExportAsFixedFormat method | Microsoft Learn](https://learn.microsoft.com/en-us/office/vba/api/powerpoint.presentation.exportasfixedformat) — 15 参签名、各默认值、「等价于 UI Save As PDF/XPS」明文
- [SO 7607914: perl Win32::OLE ExportAsFixedFormat syntax for powerpoint](https://stackoverflow.com/questions/7607914) — late binding 下 ExportAsFixedFormat 封送困难实例证
- [SO 63817805: Exporting PPT presentation to PDF from Excel VBA](https://stackoverflow.com/questions/63817805) — 跨应用 COM 调 ExportAsFixedFormat 用例
- 本仓：`docs/spec/assets-thumbs-com.md`（COM 全部已实证坑）、`service/assets_thumbs_windows.go`（Open 参数/防弹窗三板斧/进程集合）、`service/assets_thumbs_smoke_integration_windows_test.go:397`（WPS SaveAs 24 实证）

## Caveats / 未确认项

1. **WPS 对 `SaveAs(path, 32)` 与 `ExportAsFixedFormat(path, 2)` 的支持情况未获权威来源确认**（WPS 开放平台文档 SPA 不可抓取、KWPP 关键词检索零命中、DuckDuckGo/grep.app 网络不可达）——列为真机冒烟验证项，实现按双法回退设计。
2. 「目标文件静默覆盖」「视频渲染为海报帧」「超链接保留」来自社区共识、无官方明文，已标注置信度；如需硬证据须真机实测。
3. `SaveAs` 重定向 FullName / Saved 置 true 的副作用描述基于保存语义推断与社区实践，未做官方逐字引用。
