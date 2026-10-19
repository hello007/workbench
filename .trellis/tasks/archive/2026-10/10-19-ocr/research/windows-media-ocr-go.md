# 研究：Go 1.26 直接调用 Windows.Media.Ocr 可行性评估

- **Query**：评估 Go 1.26（Windows 桌面，无 CGO 倾向）直接调用 Windows.Media.Ocr（WinRT）的技术可行性；盘点现有库、PowerShell 桥、C# COM 服务器等备选路线并给出推荐排序
- **Scope**：mixed（外部检索 + 本机 Win11 zh-CN 实测 + 本仓库依赖核查）
- **Date**：2026-10-19
- **实测环境**：Windows 11 Pro 10.0.26200（zh-CN），Go 1.26.6，PowerShell 5.1（本机未安装 pwsh/PS7）

## 摘要

**结论：可行，且已有现成纯 Go 库覆盖目标 API，无需放弃转 Tesseract。** `deploymenttheory/go-bindings-winrt`（MIT，v0.6.1）生成了含 `Windows.Media.Ocr` 在内的 282 个 WinRT 包，本次研究已在其上完成 Go 1.26.6 端到端识别验证（纯 syscall 零 CGO）：1600×400 图像识别 31-43ms，二进制增量 3.68MB。需修正任务书一处前提：**OcrWord 没有置信度字段**（微软文档与本机 Get-Member 实证一致，仅 BoundingRect + Text）。语言包是最大的产品化风险点：OCR 组件属按需功能，缺失时 `TryCreateFromUserProfileLanguages` 静默返回 null（本机实测 zh-Hans-CN 可用、仅 1 个可用语言），产品必须显式检测并引导安装。推荐路线排序：① go-bindings-winrt 直连（首选）→ ② 手写 syscall 桥（备选）→ ③ PowerShell 常驻批量桥（兜底）→ ④ C# COM 服务器（不推荐）→ ⑤ C# CLI 外挂（不推荐）。转 Tesseract 无必要。

## 目录

- [1. 结论速览](#1-结论速览)
- [2. Windows.Media.Ocr API 本体事实](#2-windowsmediaocr-api-本体事实)
- [3. 路线 A：go-bindings-winrt 现有库直连（首选）](#3-路线-ago-bindings-winrt-现有库直连首选)
- [4. 路线 B：手写 syscall 桥（备选）](#4-路线-b手写-syscall-桥备选)
- [5. 路线 C：PowerShell 投影桥（兜底）](#5-路线-cpowershell-投影桥兜底)
- [6. 路线 D / E：C# COM 服务器与 C# CLI 外挂（不推荐）](#6-路线-d--ec-com-服务器与-c-cli-外挂不推荐)
- [7. 路线 F：放弃 WinRT 转 Tesseract？（无必要）](#7-路线-f放弃-winrt-转-tesseract无必要)
- [8. 工作量与风险总表](#8-工作量与风险总表)
- [9. 仓库内先例与相关规范](#9-仓库内先例与相关规范)
- [10. 信源清单](#10-信源清单)

## 1. 结论速览

| 排序 | 路线 | 可行性 | 工作量（产品化含测试） | 核心风险 |
|---|---|---|---|---|
| 1 | go-bindings-winrt 直连 | **已验证**（本机端到端跑通） | 2-3 人天（POC 已完成） | 库成熟度低（1 star / preview）；锁版本 + fork 兜底 |
| 2 | 手写 syscall 桥 | 可行（有多个活跃先例） | 3-5 人天 | vtable/GUID 手工维护、ABI 细节 bug 难排查 |
| 3 | PowerShell 常驻批量桥 | 可用但体验差 | 2-3 人天 | 冷启 ~0.9s、编码坑（已实证）、PS7 不兼容风险 |
| 4 | C# COM 服务器中间层 | 可行但负收益 | 5-8 人天 | 双工具链、COM 注册部署、CsWinRT 依赖 |
| 5 | C# CLI 外挂 exe | 可行 | 2-3 人天 | 被路线 1 完全覆盖，进程开销与分发负担 |
| - | 放弃转 Tesseract | **无必要** | 5+ 人天 | 精度/分发均劣于系统组件 |

## 2. Windows.Media.Ocr API 本体事实

### 2.1 引擎创建与语言包（最大产品化风险）

- 引擎创建三入口：`OcrEngine.TryCreateFromLanguage(Language)` / `TryCreateFromUserProfileLanguages()`（静态）/ `AvailableRecognizerLanguages`（静态属性）。微软文档原文：*「If any language from GlobalizationPreferences.Languages can be resolved to any of the available OCR languages returns new instance of OcrEngine class, otherwise returns null」* —— **语言不可用时静默返回 null，不抛错、无错误码**。
- 语言包本质：OCR 识别组件是随语言包分发的按需功能。`AvailableRecognizerLanguages` 文档原文：*「language pack must be installed on the device to be used. A user can install new OCR language packs through Windows Settings.」*
- 本机实测（Win11 26200 zh-CN 专业版）：`AvailableRecognizerLanguages.Count = 1`（仅 zh-Hans-CN），`TryCreateFromUserProfileLanguages()` 成功。PowerToys 官方 Text Extractor 功能同样基于 Windows.Media.Ocr，其文档仅提示「OCR language packs are available for installation」，印证语言包非保证预装。
- **安装率无权威数据（未找到微软口径统计）**。定性判断：中文消费版系统一般预装 zh-CN OCR 组件；LTSC、N 版、精简镜像、定制系统可能缺失。产品化必须：创建引擎返回 null 时给出引导（设置 → 时间和语言 → 语言 → 语言选项 → 勾选「光学字符识别」组件），且该检测须与「当前待识别语言」无关地前置。

### 2.2 MaxImageDimension

- 签名 `public static uint MaxImageDimension`，文档仅述「Gets the maximum image pixel dimensions supported by the OCR engine」，**无固定数值承诺**。
- 本机双通道实测均为 **10000**（像素，宽高同限）：PS 5.1 探针与 Go 探针一致。超限图像识别会失败，产品侧应按 Go 侧纯 Go 缩放（`golang.org/x/image/draw`）后喂入。

### 2.3 识别结果结构（修正任务书假设：无置信度）

| 类型 | 字段/属性 | 置信度 |
|---|---|---|
| `OcrResult` | `Text`（全文，`\r\n` 分行）、`TextAngle`、`Lines` | 无 |
| `OcrLine` | `Text`、`Words` | 无 |
| `OcrWord` | `Text`、`BoundingRect`（像素矩形） | **无任何置信度字段** |

- 微软 Learn 的 OcrWord 页仅列出 BoundingRect 与 Text 两个属性；本机 PS 探针 `Get-Member -MemberType Property` 实证一致：`BoundingRect,Text`。任务书中「Word 的 conf 值域」假设**不成立**，涉及置信度的需求（如低置信过滤）无法基于该 API 实现。

### 2.4 性能量级（本机实测，1600×400 合成图）

| 测量项 | PS 5.1 桥 | Go 直连（go-bindings-winrt） |
|---|---|---|
| powershell.exe -NoProfile 冷启动+引擎探针 | 0.859s | 不适用 |
| 进程内 解码+引擎准备（首次） | 86ms | 45.3ms |
| 进程内 识别（RecognizeAsync） | 52ms | 42.7ms（第二次进程 31.7ms） |
| 首次全链路（含渲染与一次性初始化） | 3.118s | —（首用一次性成本，进程内摊薄） |

- 口径说明：合成图为白底 Arial 48pt 简单文本，真实截图（复杂版面/小字）耗时更高，但量级为「每页数十至一二百毫秒」，非秒级。首用一次性初始化（DirectX/WIC 链路）约 2s，常驻进程可摊薄。
- Go 侧二进制体积：探针程序 `-trimpath` 构建 **3,684,864 字节（3.68MB）**（含 winrt 运行时 + storage/imaging/ocr 三域绑定，未做 `-s -w` 裁剪）。

### 2.5 中文相关行为

- CJK 文本按**单字或小片段拆分为多个 Word**（实测「你好世界」产生多个独立 word 条目），逐词 BoundingRect 粒度对中文更碎；按行聚合（OcrLine.Text）使用可规避该问题。
- 识别精度 caveat：合成 Arial 图中「Hello」被识别为「HeIIo」（l/I 混淆）；C# CLI 项目（zh-h/Windows.Media.Ocr.Cli）用户反馈低对比度图像可能整页识别失败。截图类输入建议前置二值化/放大处理。

## 3. 路线 A：go-bindings-winrt 现有库直连（首选）

### 3.1 库概况

| 项 | 值 |
|---|---|
| 仓库 | https://github.com/deploymenttheory/go-bindings-winrt |
| 版本/许可 | v0.6.1，MIT |
| 活跃度 | 2026-07-14 创建，push 2026-10-14，1 star，2 open issues（自述 Status: preview） |
| 依赖 | go-bindings-win32 v0.5.0 + go-winmd v1.0.0（均为同组织纯 Go） |
| Go 版本要求 | go 1.25.0（与本仓库 Go 1.26.6 兼容，已实证编译） |
| 覆盖面 | 「The full surface ships」：282 个包，`Windows.ApplicationModel` 至 `Windows.Web.UI.Interop` 全量生成，含 `bindings/winrt/media/ocr` |
| 构建约束 | `//go:build windows && (amd64 \|\| arm64)`，纯 Go 零 CGO |

- ocr 包内容已本地核实（v0.6.1）：`IOcrEngine`（RecognizeAsync 在 vtable slot 6）、`IOcrEngineStatics`（MaxImageDimension slot 6 / AvailableRecognizerLanguages slot 7 / TryCreateFromLanguage slot 9 / TryCreateFromUserProfileLanguages slot 10，均含 IID 常量）、`IOcrResult/IOcrLine/IOcrWord` 访问器、`IVectorViewOfOcrLine/OcrWord` 迭代。
- 图像输入链完整：`storage.StorageFileStatics().GetFileFromPathAsync` → `IStorageFile.OpenAsync` → `imaging.BitmapDecoderStatics().CreateAsync` → `GetFrameAsync(0)` → `IBitmapFrameWithSoftwareBitmap.GetSoftwareBitmapAsync`（GetSoftwareBitmapAsync/GetPixelDataAsync/GetFileFromPathAsync 三个关键方法均已核实存在）。
- 运行时层（`bindings/runtime/winrt`）：`init.go` 调 `RoInitialize(RO_INIT_MULTITHREADED)`（MTA，处理 RPC_E_CHANGED_MODE）；`activation.go` 封装 `GetActivationFactory`；异步为**事件驱动 Await()**（put_Completed 回调 + channel，非轮询），失败以 `winrt.AsyncError` 经 `errors.As` 携带真实 HRESULT。

### 3.2 端到端验证记录（本次研究实测）

本机 Go 1.26.6 探针（识别中文+英文合成图，结果见 §2.4）调用序列已验证可行：

```go
ocs, _ := ocr.OcrEngineStatics()          // GetActivationFactory("Windows.Media.Ocr.OcrEngine")
maxDim, _ := ocs.MaxImageDimension()       // 10000
engine, _ := ocs.TryCreateFromUserProfileLanguages()  // 需判 nil
fop, _ := sfs.GetFileFromPathAsync(png)    // 文件→StorageFile
sb, _  := fwsb.GetSoftwareBitmapAsync().Await()
rop, _ := engine.RecognizeAsync(sb)
result, _ := rop.Await()                   // 事件驱动阻塞
text, _ := result.Text()                   // 全文
```

import 路径注意：命名空间挂在 `bindings/winrt/` 二级目录下（如 `.../bindings/winrt/media/ocr`、`.../bindings/winrt/graphics/imaging`），运行时层在 `.../bindings/runtime/winrt`——漏写 `winrt` 段会得到误导性的「module does not contain package」错误（实测踩过）。

### 3.3 与仓库内 go-ole COM 队列的关系

- 现有素材缩略图链路（`service/assets_thumbs_windows.go`：LockOSThread + CoInitializeEx STA + 常驻 worker goroutine）是**经典 COM + IDispatch** 模式；WinRT 激活走 MTA（RoInitialize MULTITHREADED），**不需要 STA/LockOSThread**，不可照搬 STA 队列模型。若复用「常驻 worker + 任务通道」形态仅作调用编排（MTA 下并发调用亦可行，按串行简化更稳）。
- Windows.Media.Ocr 属 WinRT 投影接口（IInspectable 派生），**go-ole 的 IDispatch 路线确实不适用**（任务书猜测成立）；go-ole 仅在 winrt-go/go-bindings-winrt 的底层充当 IUnknown/vtable 载体（go-mswin/winrt README 明确此组合关系）。

### 3.4 风险与对策

| 风险 | 事实 | 对策 |
|---|---|---|
| 库成熟度 | 1 star、preview、2026-07 新库、单维护者 | 锁定 v0.6.1；接口薄封装隔离（service 层只暴露识别结果 struct）；必要时 fork 内嵌（代码为生成物，可自持） |
| 生成代码缺陷 | 同类生成器（winrt-go-gen）自述父接口方法不生成等问题 | 本包所需方法已逐一核实生成；新增 API 面时重新核验 |
| 离线/内网构建 | 依赖 3 个外部模块 | go.sum 锁定；必要时 vendor（纯 Go，体积可控） |
| 语言包缺失 | TryCreate 返回 null 静默 | 前置检测 + 前端引导文案（E_OCR_* 错误码按 `model/app_error.go` 惯例新增） |

## 4. 路线 B：手写 syscall 桥（备选）

- **激活机制**：`combase.dll` 导出 `RoInitialize`/`RoGetActivationFactory`/`WindowsCreateString`（HSTRING）等；IInspectable vtable 前 6 槽固定（QI/AddRef/Release/GetIids/GetRuntimeClassName/GetTrustLevel），接口方法随后。`golang.org/x/sys/windows`（本地核查 v0.48.0/v0.49.0）**有 `CoInitializeEx`（ole32）但无任何 Ro* 封装**——手写桥需 `syscall.NewLazySystemDLL("combase.dll").NewProc("RoGetActivationFactory")` 自声明，Go 侧完全可行（go-webview2/Wails 生态手写 COM vtable 是成熟先例，本仓库 go.mod 含 go-webview2 v1.0.23）。
- **异步处理**：两条路——实现 `IAsyncOperationCompletedHandler` 回调对象（syscall.NewCallback 构造 COM 对象）或轮询 `IAsyncInfo.get_Status`；go-bindings-winrt 选择了回调+channel，手写时回调方案更优。
- **图像输入**是隐藏工作量主体：文件路径走 StorageFile→BitmapDecoder 异步链需再声明 4-6 个接口；纯像素输入需构造 `Windows.Storage.Streams.Buffer`/IBuffer 写入，更繁琐——推荐临时 PNG 文件 + BitmapDecoder 链。
- **社区实践**（手写桥完全可行且可精简的实证）：
  - `mariyaholic/nowplaying`：**纯标准库**（go list -m all 仅自身）手写 WinRT/COM 引擎（激活工厂、vtable 调用、HSTRING、参数化 IID）驱动 SMTC，含激活工厂缓存等优化实践；要求 Go 1.27.1+（本仓库 1.26.6 不可直接 import，仅供源码参考）。
  - `mtunjic/winrt-toast`：combase syscall 直调 `Windows.UI.Notifications`，仅依赖 x/sys，零 CGO，附 go-toast/beeep 对比文档。
  - `go-mswin/winrt`：基于 winrt-go 生成器生成单一命名空间（Windows Hello）投影的先例，证明「按需生成 OcrEngine 绑定」路线可行。
  - 未找到任何 Go 项目直接使用 Windows.Media.Ocr（GitHub 仓库检索 + windows.media.ocr 关键词检索均无 Go 语言结果）——手写 OCR 桥无可抄作业，须自行写全链路。
- 工作量构成：激活/HSTRING/异步（1-1.5 人天）、图像解码链（1-1.5 人天）、OcrEngine 全接口与结果遍历（1 人天）、调试 ABI 细节（1 人天）。仅当禁用第三方依赖或 fork 维护不可接受时选择。

## 5. 路线 C：PowerShell 投影桥（兜底）

- **机制**：PS 5.1 经 `[Windows.Media.Ocr.OcrEngine,Windows.Foundation,ContentType=WindowsRuntime]` 投影语法直调 WinRT，异步经 `System.Runtime.WindowsRuntime` 的 `AsTask` 反射封装 await。先例充分：go-toast/toast（~487 星，写临时 .ps1 经 powershell.exe 执行）、gen2brain/beeep（~1.8k 星，Windows 后端同为 PowerShell）——即任务书所述「系统组件被 Go 项目经 PowerShell 桥调用」的成熟形态。
- **本机实测数据**：
  - `powershell.exe -NoProfile` 冷启+引擎创建+探针 **0.859s**；完整渲染+解码+识别首跑 **3.1s**（含 ~2s 一次性 WinRT/OCR 初始化）。
  - 进程内稳态：解码 86ms + 识别 52ms。
  - **编码坑实证**：脚本首行设 `[Console]::OutputEncoding=[System.Text.Encoding]::UTF8` 后，重定向 stdout 输出**仍为 GBK**（git-bash 捕获到 mojibake）。结论：stdout 文本回传中文不可靠，**必须 base64 包裹 JSON 或写临时文件回传**。
  - `AsTask` 反射过滤须用 `-like 'IAsyncOperation*'`（参数类型名为 `IAsyncOperation\`1`，`-eq 'IAsyncOperation'` 恒为 null，实测踩过）。
- **每页一次不可接受**（0.9-3s/页），必须常驻 PowerShell 进程按 stdin 行协议批量识别（摊薄冷启与一次性初始化），由此引入进程生命周期管理（超时杀进程、崩溃重启、并发单实例）——复杂度向 Go 直连看齐，而可靠性上限更低。
- **PS7 风险**：本机未装 pwsh 无法实测。社区通识 PS7（.NET 5+）已移除内置 WinRT 投影（需 C#/WinRT 包），投影语法在 pwsh 下大概率失效——**桥脚本必须锁定 powershell.exe 5.1**（Win10/11 必装，风险可控但须显式声明）。佐证摩擦：StackOverflow 上有人在 PowerShell 中被迫退化为 inline C# 调 Windows.Media.Ocr（问题 score 2，零采纳回答）。
- 结论：作为路线 A 的快速验证工具或极端兜底可用；作为产品主链路，性能、编码可靠性、进程管理三方面均劣于路线 A。

## 6. 路线 D / E：C# COM 服务器与 C# CLI 外挂（不推荐）

- **D：go-ole 调自注册 .NET COM 服务器包装 OCR**。需要：C# 项目（.NET 5+ 须再挂 C#/WinRT 才能调 WinRT）→ `EnableComHosting` 产出 .comhost.dll → regsvr32 注册（机器级需管理员；逐用户注册为 HKCU 兼容性陷阱）→ go-ole 按经典 COM 调用。双工具链构建、注册状态随更新/卸载漂移、浏览器通道无窗环境下 COM 激活行为待验证。工作量 5-8 人天，相对路线 A 无任何收益，不推荐。.NET 侧 COM 暴露官方文档：https://learn.microsoft.com/en-us/dotnet/core/native-interop/expose-components-to-com
- **E：随应用分发 C# CLI（zh-h/Windows.Media.Ocr.Cli 模式，225 星）**。实现最简单（Go exec 调 exe 收 stdout），但仍需 .NET SDK 构建 + 分发 exe + 进程/编码管理，能力被路线 A 完全覆盖，仅作应急参考。该项目 2023-04 后停更。

## 7. 路线 F：放弃 WinRT 转 Tesseract？（无必要）

- Windows.Media.Ocr 为 Win10+ 系统组件，**零模型分发成本**，对印刷体中文与截图场景质量可接受（PowerToys Text Extractor 生产背书）；Tesseract 需引入 cgo（违背无 CGO 约束）或独立 exe + chi_sim 训练数据分发（数十 MB），中文精度通常不如 Windows OCR，工程量 5+ 人天。
- 仅当需求扩展到「无语言包环境保证可用 + 手写体/复杂版面」时才需重评；当前场景（PPT 页面图文提取）语言包缺失可引导解决，不构成转 Tesseract 的理由。

## 8. 工作量与风险总表

| 路线 | POC | 产品化（含测试/门禁） | 主要风险 | 适用前提 |
|---|---|---|---|---|
| A go-bindings-winrt | **0（已完成）** | 2-3 人天：service 封装 + E_OCR_* 错误码 + 语言包缺失引导 + 临时文件生命周期 + 超时/取消 + 单测（≥76% service 门禁） | 库 preview 期成熟度；锁 v0.6.1 + 必要时 fork | 默认选择 |
| B 手写 syscall | 2 人天 | 3-5 人天 | vtable/GUID 维护、ABI bug 难查、无社区作业可抄 | 禁用三方依赖时 |
| C PowerShell 常驻批量 | 0.5 人天 | 2-3 人天 | GBK 编码（须 base64 回传，已实证）、PS7 不兼容（锁 5.1）、进程管理 | 仅作兜底/原型 |
| D C# COM 服务器 | 1-2 人天 | 5-8 人天 | 注册部署、双工具链、负收益 | 不推荐 |
| E C# CLI 外挂 | 0.5 人天 | 2-3 人天 | 分发/进程开销，被 A 覆盖 | 不推荐 |
| F Tesseract | 1 人天 | 5+ 人天 | 精度、分发重量、cgo 违约 | 无必要 |

## 9. 仓库内先例与相关规范

| 文件 | 关联点 |
|---|---|
| `service/assets_thumbs_windows.go`（workerLoop 约 159-163 行：LockOSThread + CoInitializeEx STA + S_FALSE 放行） | go-ole 经典 COM 队列先例；WinRT OCR 走 MTA，勿照搬 STA 模型 |
| `docs/spec/assets-thumbs-com.md` | COM 进程外服务器（PowerPoint）队列/超时/杀进程形态；WinRT OCR 为进程内系统组件，无进程快照 diff/COM 忙态重试等问题 |
| `.trellis/tasks/archive/2026-10/10-14-ppt-com/research/com-thumbnail-export.md` | COM 技术选型研究文档先例 |
| `go.mod`（go-ole v1.3.0 / x/sys v0.48.0 / go-webview2 v1.0.23） | 现有 COM 相关依赖基线 |
| `.trellis/spec` 已废弃，规范沉淀位于 `docs/spec/`（按项目关键规则） | 新增 OCR 契约文档落点应为 `docs/spec/` |

## 10. 信源清单

**微软官方文档（Learn）**

- OcrEngine 类（RecognizeAsync → OcrResult 结构与 remarks）：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine
- OcrEngine.MaxImageDimension（uint，无固定值）：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine.maximagedimension
- TryCreateFromUserProfileLanguages（可解析否则返回 null）：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine.trycreatefromuserprofilelanguages
- TryCreateFromLanguage：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine.trycreatefromlanguage
- AvailableRecognizerLanguages（语言包必须已安装原文）：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine.availablerecognizerlanguages
- OcrWord（仅 BoundingRect/Text，无置信度）：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrword
- OcrLine：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrline
- OcrResult：https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrresult
- PowerToys Text Extractor（同 API 生产背书 + 语言包提示）：https://learn.microsoft.com/en-us/windows/powertoys/text-extractor
- .NET 组件暴露为 COM（EnableComHosting）：https://learn.microsoft.com/en-us/dotnet/core/native-interop/expose-components-to-com

**Go 库与先例（GitHub）**

- deploymenttheory/go-bindings-winrt（首选库，282 包全量，含 media/ocr）：https://github.com/deploymenttheory/go-bindings-winrt
- saltosystems/winrt-go（51 星，仅蓝牙域生成器先例）：https://github.com/saltosystems/winrt-go
- go-mswin/winrt（winrt-go-gen 单命名空间生成先例）：https://github.com/go-mswin/winrt
- mariyaholic/nowplaying（stdlib-only 手写 WinRT 引擎）：https://github.com/mariyaholic/nowplaying
- mtunjic/winrt-toast（syscall 直调 + PowerShell 桥对比文档）：https://github.com/mtunjic/winrt-toast 及 https://github.com/mtunjic/winrt-toast/blob/main/docs/comparison.md
- go-toast/toast（PowerShell 桥先例，~487 星）：https://github.com/go-toast/toast
- gen2brain/beeep（PowerShell 桥先例，~1.8k 星）：https://github.com/gen2brain/beeep
- deploymenttheory/go-winmd（纯 Go WinMD 读取器）：https://github.com/deploymenttheory/go-winmd

**C# CLI 参考**

- zh-h/Windows.Media.Ocr.Cli（225 星，2023 停更）：https://github.com/zh-h/Windows.Media.Ocr.Cli

**StackOverflow**

- 非 UWP 桌面可用性：https://stackoverflow.com/questions/52537944/
- PowerShell 调 OCR 退化为 inline C#（PS 桥摩擦佐证）：https://stackoverflow.com/questions/72648947/

## Caveats / Not Found

- **OCR 语言包安装率无权威统计**（微软无公开口径），仅有行为文档与单机实证；建议实现时以前置检测兜底而非依赖安装率假设。
- **PS7（pwsh）行为未实测**（本机未安装），「PS7 无内置 WinRT 投影」为社区通识，若最终选择路线 C 须真机验证。
- **真实截图（复杂版面）识别耗时未测**（合成图仅能量级参考）；实施阶段应补真实 PPT 页面样本基准。
- MaxImageDimension=10000 为本机单机实测值，微软文档未承诺固定值；不同机器/版本可能不同，代码须动态读取。
- GitHub API 检索在限流前未覆盖全部「Go + SMTC PowerShell 桥」项目清单；已获先例（go-toast/beeep）足以支撑结论，未再补充检索。
