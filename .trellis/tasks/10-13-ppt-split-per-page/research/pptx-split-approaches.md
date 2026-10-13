# Research: PPT 按页拆分技术方案选型（Go 桌面应用 / Wails v2）

- **Query**: 在 Go 桌面应用（Wails v2，Windows 为主、Linux 次之，无 Office 安装保证）中实现「按页拆分 .pptx，每页生成一个独立新 pptx」的技术方案选型
- **Scope**: mixed（外部生态调研 + 本仓库约束映射）
- **Date**: 2026-10-13
- **外部证据核实方式**: GitHub API（unioffice 源码 / 许可证 / 仓库搜索）、StackExchange API（python-pptx 删页社区答案）、go.mod 本地实测；raw.githubusercontent.com 被本机沙箱阻断，pkg.go.dev 文档页渲染不完整，均改用 API 通道核实

---

## 结论速览

**推荐方案 B：纯 Go 直接操作 pptx（OOXML zip 结构），仅用标准库 `archive/zip` + `encoding/xml` 实现。**
核心理由：① 唯一同时满足「无 Office 安装保证」与「单测不依赖本机 Office（PRD DoD 硬约束）」的方案；② 未被删除的 part（母版/版式/主题/媒体/字体）逐字节原样拷贝，保真度是四种方案中最高的——不存在任何格式转换环节；③ 零第三方依赖，与 go.mod 现有 8 个直接依赖的精简风格一致；④ Windows/Linux 天然一致，无平台分叉。

---

## 方案 A：PowerPoint COM 自动化

### 实现路径

两条子路径：

1. **go-ole 直驱**：Go 内 `oleutil.CreateObject("PowerPoint.Application")` → IDispatch 调 `Presentations.Open(path, ReadOnly, Untitled, WithWindow:=msoFalse)` → 逐页 `Slide.Delete()` 删其余页 → `SaveAs` 输出。需 `runtime.LockOSThread` + `ole.CoInitialize`（PowerPoint COM 是 STA，goroutine 线程漂移会崩）。
2. **PowerShell 脚本中转**：Go `os/exec` 调 `powershell -NoProfile -File split.ps1`，脚本内 `New-Object -ComObject PowerPoint.Application` 完成同样流程。Go 侧零 COM 代码，复杂度转移到脚本。

### 拆分保真度

**最高之一**。由 PowerPoint 自己重存文件，母版/版式/媒体/节（sections）/自定义放映（custom shows）由 PowerPoint 内部引擎维护一致性，删页时自动更新节与自定义放映引用，无修复弹窗风险。

### 依赖要求

- 必须本机安装 PowerPoint（任意含 PowerPoint 的 Office 版本）；
- 须可交互桌面会话（COM 自动化不可在无 UI 会话运行，微软 KB257757 明确不支持服务场景；桌面应用场景合法）；
- go-ole v1.3.0 **已是本仓库间接依赖**（go.mod:31，Wails 依赖链带入），升为直接依赖零新增模块。

### 平台兼容

仅 Windows。Linux 无 Office，方案天然不可用——与本仓库「Linux 支持已存在」的现状冲突（Linux 平台该工具只能禁用或降级报错）。

### 许可证与成本

无额外授权费用（复用用户已购 Office），但功能可用性**取决于终端用户是否装 Office**——PRD 明确「无 Office 安装保证」，此为硬冲突。

### 实现复杂度

中等。单引擎逻辑不复杂，但工程面较大：STA 线程绑定、异常时 POWERPNT.EXE 孤儿进程清理（须 `Quit` 兜底 + 进程级强杀）、`DisplayAlerts = ppAlertsNone` 防弹窗卡死、N 页 = N 次 Open/SaveAs（每文件约 0.5~2 秒，100 页约 2 分钟级）。

### 已知坑

- PowerPoint `Application.Visible = false` 历史上直接抛错，隐藏窗口须在 `Open` 的 `WithWindow:=msoFalse` 实现；
- Office 首启向导 / 激活弹窗 / 杀软拦截均可能让自动化挂起，桌面环境不可控；
- CI（无 Office）完全无法跑真实链路，单测只能测 fake，真实路径零验证覆盖——与 DoD「单测覆盖」精神冲突面最大。

---

## 方案 B：纯 Go 直接操作 OOXML zip 结构（推荐）

### 原理

.pptx 是标准 OOXML zip 包。拆分 = 对每个目标页复制整个 zip，然后做一次「最小编辑集」：

| 必改项 | 位置 | 操作 |
|---|---|---|
| 页序与页 ID 列表 | `ppt/presentation.xml` 的 `<p:sldIdLst>` | 删除其他页的 `<p:sldId>` 条目（页序以此处为准，**与 slide 文件名无关**） |
| 关系表 | `ppt/_rels/presentation.xml.rels` | 删除其他页 rId → part 的 Relationship 条目 |
| 内容类型声明 | `[Content_Types].xml` | 删除其他页 slide part 与其 notesSlide part 的 `<Override>` 条目（Override 指向不存在的 part 会触发修复弹窗） |
| 页 part | `ppt/slides/slideN.xml` + `ppt/slides/_rels/slideN.xml.rels` | 从 zip 中删除 |
| 备注页 | `ppt/notesSlides/notesSlideN.xml`（+ 各自 `.rels`） | 随所属页一并删除（notesSlide 关系双向指回 slide，留孤儿有修复风险） |
| 节（sections） | `ppt/presentation.xml` `<p:extLst>` 内 `p14:sectionLst` | 引用的是数字型 sldId，删页后须剥离或重建，否则悬空引用 |
| 自定义放映 | `ppt/presentation.xml` `<p:custShowLst>` | 引用的是 slide rId，须剥离 |

**明确保留不动**：slideLayouts / slideMasters / theme / media / embeddedFontLst（字体内嵌）/ presProps / viewProps / tableStyles。这些是包级共享资源，保留即保真。可选进阶：GC 掉仅被已删页引用的 media（不做也不影响正确性，仅文件偏大）。

### 拆分保真度

**四种方案中最高**。除上述 7 项列表编辑外，所有 part 逐字节原样拷贝，无任何渲染/转换/重编码环节。已删页的媒体若不 GC 会残留在包内（正确性无损）。

### 依赖要求

**零第三方依赖**，纯标准库 `archive/zip` + `encoding/xml`（XML 列表编辑用轻量 DOM 走 `encoding/xml` 的 Token 流或 `xml.Marshal`/`Unmarshal` 均可）。

### 平台兼容

Windows / Linux 完全一致，无平台分叉文件、无 tag 分叉。

### 许可证与成本

无（自有代码）。

### 实现复杂度

中等偏低。核心是 4 个 XML 的列表编辑 + zip 逐项复制，约 300~500 行。佐证：

- **unioffice 源码证实编辑集正确性**（2026-10-13 经 GitHub API 核实 master 分支 `presentation/presentation.go`）：其 `Presentation.RemoveSlide(s Slide) error` 的实现正是「从 SldIdLst 删条目 + 按 rId 从 rels 删关系 + `removeNotesAt` 联动删备注页」三件事，与上表最小编辑集吻合；
- **python-pptx 社区配方**（StackOverflow 54945022 高票答案，2026-10-13 核实）：`prs.part.drop_rel(rId)` + `del prs.slides._sldIdLst[i]` 两步即稳定删页，佐证「sldIdLst + rels」两处编辑是充分最小集（该配方连 notesSlide 都未删，PowerPoint 仍正常打开，说明 PowerPoint 对孤儿 part 容忍度高，本方案按更严格口径处理）；
- GitHub 搜索 `pptx split language:go` 结果 **total_count = 0**（2026-10-13），无现成轮子，但编辑集本身已被上述两个实现充分踩坑验证。

### 已知坑

1. **页序陷阱**：`slide1.xml` 文件名与展示页序**无关**，必须经 `sldIdLst → rId → presentation.xml.rels → part 名` 三跳映射，直接按文件名序拆是典型错误；
2. **保留页中的跨页超链接/动作按钮**：保留页的 rels 里可能存在指向已删页的 jump-to-slide 关系（rel type 以 `/slide` 结尾），悬空时可能触发修复，实现时应扫描保留页 rels 并剔除该类条目（低成本）；
3. **rels 中 rId 非连续**：不能假设 rId2/rId3… 连续排列，必须解析实际映射；
4. **sections / custShows**：是社区经验性结论的悬空引用修复弹窗触发点（未逐项实测，建议实现时直接剥离并在验收用例中覆盖含节/含自定义放映的 fixture）；
5. **docProps/app.xml** 的 `Slides` 计数与 `TitlesOfParts` 会变陈旧——纯元数据，PowerPoint 下次保存自动修正，无害；
6. **输出原子写**：二进制 zip 不适用 `util.SaveJSON`（JSON 专用），但应复刻其原子写惯例（`os.CreateTemp` 同目录临时文件 → 写入 → `os.Rename` 替换，见 `util/json.go:34` 的实现注释，Windows 为 MoveFileEx REPLACE_EXISTING）。

---

## 方案 C：外部依赖（LibreOffice headless / python-pptx 子进程 / 现成 CLI）

### C1. LibreOffice headless

- `soffice --headless` **无按页拆分原生能力**（convert-to 只做格式转换），拆分须另写 UNO/Basic 宏脚本驱动；
- LibreOffice 重写文件走 Impress 导入/导出链路，母版/版式/动画/过渡存在已知漂移风险，保真度三种里最低；
- 依赖用户机器装 LibreOffice（约数百 MB），「无安装保证」冲突与方案 A 同构；
- 许可证 MPL-2.0（本身无成本），但分发体积与用户环境不可控。
- **结论：不采纳。**

### C2. python-pptx 子进程

- 依赖用户机器装 Python + `pip install python-pptx`，安装保证为零，桌面工具不可接受；
- python-pptx 无官方删页 API（社区配方即上文 B 方案两行），本质是「用 Python 写 B 方案」，复杂度未降低反而多一层进程边界与 JSON/输出解析协议；
- **结论：不采纳。**

### C3. 现成 CLI 工具

- GitHub Go 生态搜索（2026-10-13）：`pptx split language:go` 零命中；`pptx golang` 命中的库均为文本抽取/替换用途（`moipa-cn/pptx` Apache-2.0 仅文本图片替换、`manuviswam/go-pptx` GPL-3.0 仅 2 star），**无任何可复用的拆分实现**；
- 无业界事实标准的 pptx 按页拆分 CLI。
- **结论：无可选项。**

---

## 方案 D：商业库

| 库 | Go 支持 | 许可证 | 核实结果 |
|---|---|---|---|
| **unioffice**（unidoc，4934 star） | 原生 Go，API 完整 | **纯商业 EULA**（仓库 LICENSE.md 原文："requires a license code to operate"，https://unidoc.io/eula/，价格需询官网） | `RemoveSlide` API 已从源码确认（含 notes 联动）；曾为 AGPL 双授权，现仓库已转纯商业 |
| **Aspose.Slides** | **无 Go 版本**（仅 Java/.NET/C++/Python） | 商业 | cgo 桥接 C++ 版为巨型依赖，不可行 |
| **Spire.Presentation**（E-iceblue） | **无 Go 版本** | 商业 | — |

**结论：unioffice 功能上一步到位，但（1）商业授权费用与全 OSS 依赖现状冲突；（2）AGPL 已不可选；（3）方案 B 的编辑集已被其源码反向验证，自研成本可控。不采纳，仅作为保底后手。**

---

## 对比总表

| 维度 | A. PowerPoint COM | B. 纯 Go OOXML（推荐） | C. LibreOffice/python-pptx | D. 商业库 unioffice |
|---|---|---|---|---|
| 拆分保真度 | 高（PowerPoint 重存，引擎自洽） | **最高（未触碰 part 逐字节原样拷贝）** | 低~中（Impress 导入导出漂移） | 高 |
| 母版/版式/媒体保留 | 随 PowerPoint 重存保留 | **随包原样保留（引用制）** | 导出重写，有漂移风险 | 保留 |
| 第三方依赖 | go-ole（已是间接依赖） | **零（纯标准库）** | LibreOffice 数百 MB / Python 运行时 | unioffice 单库 |
| 本机 Office/外部安装要求 | **必须装 Office** | **无任何要求** | 必须装对应环境 | 无 |
| Windows | 可用 | **可用** | 装 LO 则可用 | 可用 |
| Linux | **不可用**（无 Office） | **可用** | 装 LO 则可用 | 可用 |
| 许可证与成本 | 复用用户 Office | 自有代码，无成本 | MPL-2.0，无成本 | **商业 EULA，需购买** |
| 实现复杂度 | 中（STA/进程清理/防弹窗） | **中低（约 300~500 行，编辑集已被双实现验证）** | 中（多一层进程协议） | 低（调 API） |
| 单测可离 CI 验证 | **否（CI 无 Office，仅 fake）** | **是（fixture pptx 用 zip 构造，全链路可测）** | 否（CI 无 LO） | 是 |
| 与 PRD「无 Office 安装保证」冲突 | **冲突** | **不冲突** | 冲突 | 不冲突 |
| 已知坑 | 孤儿 POWERPNT.EXE、弹窗挂起、慢 | 页序三跳映射、sections/custShows 剥离、跨页超链接清理 | 保真度、分发体积 | 商业费用、锁库 |

---

## 映射到本仓库约束

| 仓库约束 | 方案 B 落点 |
|---|---|
| Go 后端、单测禁依赖本机 Office（PRD DoD） | 生产实现纯标准库；测试 fixture 用 `archive/zip` 程序化构造含 N 页的最小 pptx（含 sections/custShows/notesSlide 边界形态），真实链路 CI 全覆盖 |
| Provider 抽象 + fake 注入先例（`service/process.go:38` `ProcessProvider`） | 抽 `PptxSplitProvider interface { Split(ctx context.Context, srcPath, outDir string, ...) error }` 或更窄的拆分函数注入点；COM/外部依赖若将来引入，天然落进同一接口 |
| 原子写惯例（`util/json.go:34` `SaveJSON`：CreateTemp 同目录 → Sync → Rename） | 输出 pptx 为二进制 zip，不适用 SaveJSON 本体，但复刻同一原子写序列；与「data/ 原子写」关键规则精神一致 |
| Windows 主平台 + Linux 已存在 | 纯 Go 无平台分叉，无需 `_windows/_other` 双文件 |
| 新增 service 两处同步（`app_services.go` 字段+构造）+ 如有事件同步 `web_serve.go` applySink | 拆分为长操作：按 CLAUDE.md 取消链路惯例接入 `context.WithCancel` + 取消注册表；进度事件走 EventSink（`emitEvent`），则 applySink 须同步加行 |
| 依赖精简风格（go.mod 现仅 8 个直接依赖） | 零新增依赖 |

### PRD Open Questions 的技术面输入

- **.ppt 老格式**：四种方案中仅 COM 可处理 .ppt（二进制 OLE 格式）；纯 Go 方案天然仅支持 .pptx（OOXML），与 PRD「仅支持 .pptx」假设吻合，建议 Out of Scope 显式排除 .ppt；
- **命名规则**：技术方案无约束，`name-01.pptx` 风格对排序与重名更友好（纯 Go 生成文件名零开销）。

---

## 推荐倾向与理由

**推荐方案 B，且按「最小编辑集 + 显式剥离 sections/custShows」口径实现。**

1. **约束满足是排他性的**：PRD 同时要求「无 Office 安装保证」与「单测不依赖本机 Office」，方案 A/C 在第一条或第二条上必撞其一，唯 B 全满足；
2. **保真度不是折衷而是最优**：引用制包结构决定「保留即保真」，母版/版式/媒体不经任何转换环节；COM 与商业库反而都要经一次引擎重存；
3. **实现风险已被外部实现背书**：unioffice `RemoveSlide` 源码与 python-pptx 社区配方双重验证了编辑集的最小充分性；已知坑（页序三跳、sections/custShows、跨页链接）均有明确对策；
4. **工程面完全落在仓库既有模式内**：Provider 抽象、原子写、取消链路、覆盖率门禁均有先例可循，无新模式引入；
5. **后手充足**：若未来出现 B 无法处理的畸异文件，可按 Provider 接口在 Windows 侧加 COM 引擎作可选增强（go-ole 已在依赖树中），架构上无需推倒。

---

## 引用来源（2026-10-13 核实）

- unioffice `presentation/presentation.go`（master 分支，经 GitHub contents API）：`RemoveSlide` + `removeNotesAt` 实现片段；`LICENSE.md`：商业 EULA 声明
- StackExchange API，SO 问题 54945022 高票答案：`drop_rel(rId)` + `del _sldIdLst[i]` 删页配方；次高票：仅 `_sldIdLst.remove` 的宽松配方
- GitHub Search API：`pptx split language:go` total_count=0；`pptx golang` 生态库清单（均无拆分能力）
- 本仓库：`go.mod`（go-ole v1.3.0 间接依赖，8 直接依赖）、`service/process.go:33-88`（Provider 模式）、`util/json.go:34`（原子写惯例）

## Caveats / Not Found

- 「sections/custShows 悬空引用触发 PowerPoint 修复弹窗」为社区经验性结论（OOXML 规范定义了这两处对 slide 的引用，悬空风险合理推断），未在本机逐项实测；建议实现阶段以含节/含自定义放映的 fixture 做验收用例闭环验证；
- PowerPoint COM 子路径的细节（`WithWindow:=msoFalse`、`ppAlertsNone`、孤儿进程）基于通用 Office 自动化经验，本机未实测（不影响推荐结论，B 方案不依赖该路径）；
- unioffice 商业授权具体价格官网未公开固定值，需询价（已确认其性质为纯商业 EULA）；
- 「跨页超链接悬空触发修复」的影响等级未实测，实现时按低成本防御性清理处理即可。
