# Research: PPT 单页图片本地离线 OCR 引擎选型对比

- **查询**：Windows 桌面应用（Go 1.26 + Wails）为 PPT 幻灯片单页图片（约 1600px PNG，简体中文为主少量英文混排）做本地离线 OCR 的引擎选型，四维对比（离线 / 中文准确率 / 性能 / 依赖纯度）并给出推荐排序
- **范围**：混合（外部信源核验 + 项目内代码实证）
- **日期**：2026-10-19
- **信度标注体系**：`[官方文档]`（微软/项目官方文档，本次抓取核验）｜`[官方仓库]`（GitHub Release/API 数据，本次核验）｜`[口碑]`（社区共识，本次未能核验原始出处）｜`[推断]`（由已知数据推算，需 PoC 验证）｜`[需实测]`（本次无法获得可靠数据）

> **检索环境限制声明**：本次研究环境中 raw.githubusercontent.com 与搜索引擎（Bing/DuckDuckGo）均被网络策略阻断，量化中文准确率基准（学术/第三方横评）未能取得；以下中文准确率结论均为定性口碑级，**落地前必须以 PoC 实测为准**（见 §9）。GitHub API 数据与微软官方文档均为本次实际抓取核验（2026-10-19）。

---

## 摘要

**结论前置**：推荐排序为 **① PaddleOCR-json（首选）→ ② RapidOcrOnnx Windows bin（体积敏感备选）→ ③ Tesseract 5 外部 exe（第三）**；**Windows.Media.Ocr 不推荐作主引擎**（官方文档实证无置信度 API，违反硬约束 5，且 Go 无现成 WinRT 绑定、桥接维护风险高）。三者候选均为「外部 exe 进程隔离 + stdout JSON/TSV 解析」形态，零 CGO、不破坏现行纯 Go 交叉编译链路，与项目既有 exec+ctx 取消先例（util/git.go）和 COM 先例（assets_thumbs）架构同构。

---

## 目录

1. 背景与硬约束
2. 项目现状锚点（代码实证）
3. 候选引擎总览
4. 候选详析
5. 四维对比表与万级页可行性估算
6. 置信度口径专项
7. 分发体积与 CI 影响专项
8. 推荐排序与理由
9. PoC 验证清单（落地前必测）
10. 信源清单
11. Caveats / 未决事项

---

## 1. 背景与硬约束

来自任务指定与 `.trellis/tasks/10-19-ocr/prd.md`：

| # | 硬约束 | 对选型的含义 |
|---|---|---|
| 1 | 数据本地不上云（SRS §1.4） | 一票排除全部云 API（百度/腾讯/阿里/Azure Vision 等） |
| 2 | 纯 Go 倾向强；CGO 方案破坏交叉编译与 CI，除非压倒性优势 | 排除 gosseract、yalue/onnxruntime_go 等一切 CGO 绑定 |
| 3 | 场景：售前/市场 PPT 截图型幻灯片，简体中文为主少量英文混排，输入为已渲染幻灯片 PNG（约 1600px 宽） | 中文识别质量是第一权重；输入已规整（无拍照畸变），对引擎的图像预处理要求低 |
| 4 | 万级页面量级索引（十万级幻灯片为 SRS 上限目标） | 单页耗时数量级决定队列可行性 |
| 5 | 必须输出置信度（ocr_confidence 列） | 引擎无置信度 API = 硬伤，需给替代口径 |
| 6 | 桌面应用随包分发或用用户机器已有组件 | 安装包体积增量须评估 |
| 7 | CI：GitHub Actions windows runner，无管理员安装偏好，绿色解压优先 | runner 预装清单已核验：无 Tesseract、无任何 OCR 组件（§7） |

PRD 验收补充（prd.md:61）：若走 COM/WinRT 须继承 assets-thumbs COM 全套约束（STA/杀进程/jr.Result 恰一次/绝对路径），真机冒烟禁 fixture 掩蔽。

---

## 2. 项目现状锚点（代码实证）

| 事实 | 出处 | 对选型的意义 |
|---|---|---|
| Go 1.26.0 / toolchain go1.26.6，`go-ole v1.3.0` 与 `modernc.org/sqlite v1.60.1` 是仅有的两个「非纯 Go 倾向」破例 | `go.mod:3-19` | 破例先例均为「进程隔离/纯 Go 实现」形态，COM 桥可接受，但**手写 WinRT syscall 桥无先例** |
| OCR 输入源现成：hd 缩略图 PNG 1600×900 默认档 | `service/assets_thumbs.go:47`、`:63` | 输入假设（1600px 已渲染 PNG）成立，OCR 队列可复用 thumbs backlog 形态 |
| COM STA worker 全套约束已成文 | `service/assets_thumbs.go:19,107-112`、`docs/spec/assets-thumbs-com.md` | 若走 WinRT/COM 桥，约束可继承，但成本仍在 |
| 外部进程 + ctx 取消链路现成 | `util/git.go:92-95`（ExecuteWithContext）、`:183-185`（CloneWithContext） | 外部 exe 引擎的取消/超时治理有直接先例可仿 |
| PRD 已预判候选 | `.trellis/tasks/10-19-ocr/prd.md:83,85-88` | 本文即 prd.md 所列 `research/ocr-engine-comparison.md` 的交付 |

---

## 3. 候选引擎总览

| 候选 | 形态 | 本次核验状态（2026-10-19） |
|---|---|---|
| PaddleOCR-json（hiroi-sora） | 独立 exe 常驻进程，stdin/base64/文件路径输入，JSON 输出 | v1.4.1（2024-08-28），MIT，1555★，Windows x64 包 88.4MB，附带 PP-OCRv3/v4 模型 `[官方仓库]` |
| RapidOcrOnnx（RapidAI）windows-bin | 独立 exe 命令行 + onnx 模型目录 | release 1.2.2（2023-02）windows-bin 9.0MB，模型 PP-OCRv3；仓库 commits 活跃（2026-10-04 仍有修复），Apache-2.0，376★ `[官方仓库]` |
| Tesseract 5 外部 exe（UB-Mannheim build） | tesseract.exe + traineddata，TSV/hOCR 输出 | UB-Mannheim 最新 5.4.0.20240606（2024-06-08，安装包 47.9MB）；官方最新 5.5.3（2026-07-24）仅源码，Windows 现成二进制滞后 `[官方仓库]` |
| Windows.Media.Ocr（WinRT 系统组件，Win10+ 自带） | 系统组件，需调用桥 | OcrEngine/OcrWord 官方 API 已逐项核验：**无任何置信度成员**；Go 生态无现成库（§4.4） `[官方文档]` |
| tesseract.js（WebView 内嵌 WASM） | 前端 JS | 排除，理由见 §4.5 `[口碑+推断]` |
| gosseract / otiai10 等 CGO 绑定 | CGO 链接 C 库 | 排除，理由见 §4.5 `[官方仓库]` |
| yalue/onnxruntime_go 自托管 PP-OCR 模型 | CGO + onnxruntime.dll | 排除，理由见 §4.5 `[官方文档]` |

---

## 4. 候选详析

### 4.1 PaddleOCR-json（首选候选）

**原理与集成**：C++ 编译的独立命令行程序（PaddleOCR 推理 + OpenCV），常驻进程循环读任务（本地图片路径 / Base64 / TCP 局域网调用三模式），stdout 返回 JSON。Go 侧零依赖：`exec.Command` 启动 + stdin 写任务 + stdout 逐行 JSON 解析，进程隔离与项目 exec 先例同构。支持 Windows 7+ x64（README 声明）`[官方仓库]`。

- **离线能力**：完全离线 ✔ `[官方仓库]`
- **中文准确率**：附带 PP-OCRv3/v4 识别库，官方描述对「手写、艺术字、小字号、复杂背景」也有不错识别率 `[官方仓库]`；PP-OCR 系是中文开源 OCR 事实标准——最强背书：中文离线 OCR 最流行的桌面工具 Umi-OCR（48030★，MIT，2025-11 仍活跃）将其列为核心引擎 `[官方仓库]`；上游 PaddleOCR 官方仓库持续演进（2025 年 PP-OCRv5，2026 年主页已出现「较 PP-OCRv5 再提升 5.1% 识别精度、CPU 端到端 5.2× 加速」的新版本描述）`[官方仓库]`。**注意**：PaddleOCR-json v1.4.1 附带模型停留在 v3/v4，未跟上上游 v5，模型可替换但兼容性 `[需实测]`
- **置信度**：输出 JSON 每文本块含 `score` 字段（0~1 浮点，越接近 1 越可信）——README 原文已核验 `[官方文档]` ✔
- **性能**：官方称效率高于 Python 版 PPOCR 及 Python 任务流 OCR 引擎 `[官方仓库]`；单页数量级 `[推断]`：1600px 页 0.3~1.0s（CPU、mobile 模型），万级页单进程约 0.8~2.8 小时，3-4 进程并行约 15~45 分钟
- **分发体积**：压缩包 88.4MB `[官方仓库]`；解压后体积 `[需实测]`（7z 高压缩，解压后预计更大）。附条件：Win7 下报缺 VCOMP140.DLL 须装 VC 运行库 `[官方文档]`——Win10/11 一般已具备，企业 LTSC 裁剪系统需运行时探测（仿 assets 缩略图 COM 机器差异探测思路）
- **许可**：仓库 MIT，PaddleOCR 模型 Apache-2.0 `[官方仓库]`，随包分发无障碍
- **CI**：下载 release 7z 解压即用，无管理员 ✔ `[推断]`（runner 具备网络下载能力已核验 §7）

### 4.2 RapidOcrOnnx windows-bin（体积敏感备选）

**原理与集成**：RapidAI 的 C++/ONNX Runtime 实现，windows-bin.7z 仅 9.0MB（exe+动态库，模型另配），示例模型为 PP-OCRv3 det/rec/cls onnx（约 15~20MB `[推断]`，README 给出模型文件清单）。命令行输入图片路径，stdout 输出文本块结果（含 score `[口碑]`，输出格式细节 `[需实测]`）。

- **离线能力**：完全离线 ✔ `[官方仓库]`
- **中文准确率**：同 PP-OCR 系（v3 模型）；仓库属 RapidAI 主仓（RapidOCR 8179★，v3.10.0 2026-10-08 发版，Apache-2.0）生态，C++ CLI 子项目 release 停在 2023-02 但 commits 活跃（2026-10-04）`[官方仓库]`
- **置信度**：结果含 score `[口碑]` ✔（需 PoC 核对输出协议）
- **性能**：与 PaddleOCR-json 同量级（同为 onnx CPU 推理）`[推断]`；仓库自带 run-benchmark 工具可自测 `[官方仓库]`
- **分发体积**：bin 9MB + 模型 15~20MB ≈ **25~30MB，全候选最小** `[官方仓库+推断]`
- **模型可替换性**：onnx 模型目录约定替换，社区有换用 v4/v5 rec 模型实践 `[口碑]`，与 C++ 前处理的兼容性 `[需实测]`
- **许可**：Apache-2.0 `[官方仓库]`
- **风险**：release 滞后近三年、维护热度低于 PaddleOCR-json 生态（Umi-OCR 背书缺失）；CLI 为单次执行模式（每页起进程）或需改造为常驻（性能减损）`[需实测]`

### 4.3 Tesseract 5 外部 exe

**原理与集成**：tesseract.exe + chi_sim.traineddata，`exec.Command` 进程隔离，`-c tessedit_create_tsv=1` 或 hOCR 输出取逐词 conf（0~100）`[官方文档]` ✔。

- **离线能力**：完全离线 ✔
- **中文准确率**：**多源口碑一致认为 chi_sim 明显弱于 Paddle 系**（空格/标点/多栏/图文混排场景尤其），2024-2026 量化基准本次未取得 `[口碑+需实测]`
- **性能**：1600px 页 1~3s（chi_sim LSTM，CPU 单线程为主）`[口碑+推断]`；万级页单进程 2.8~8.3 小时，8 进程并行 20~60 分钟——**全候选最慢** `[推断]`
- **模型体积**：tessdata_fast chi_sim 仅 **2.4MB**、eng 3.9MB、osd 10.1MB（osd 可省）；tessdata_best chi_sim 12.5MB、eng 14.7MB（质量更高、更慢）`[官方仓库]` ✔ 体积数据本次已核验
- **Windows 二进制**：UB-Mannheim 最新 **5.4.0.20240606**（2024-06-08，安装包 47.9MB，InnoSetup 可绿色解压）；官方已到 5.5.3（2026-07-24）但仅源码——**Windows 发行滞后约两年** `[官方仓库]`
- **许可**：Apache-2.0（引擎与 traineddata 均）`[官方仓库]`
- **最小集成组合体积**：exe+依赖 dll 约 30MB + chi_sim 2.4MB + eng 3.9MB ≈ **35~40MB** `[推断]`

### 4.4 Windows.Media.Ocr（WinRT 系统组件）——不推荐作主引擎

**API 事实（本次逐项核验微软官方文档）** `[官方文档]`：

| 核验项 | 结论 |
|---|---|
| OcrEngine 成员 | `AvailableRecognizerLanguages` / `MaxImageDimension` / `RecognizerLanguage` / `IsLanguageSupported` / `RecognizeAsync(SoftwareBitmap)` / `TryCreateFromLanguage` / `TryCreateFromUserProfileLanguages`——**不存在任务中提到的「MaxRetrievalCount」类 API，图像上限相关常量是 MaxImageDimension** |
| OcrWord 成员 | 仅有 BoundingRect / Text（Remarks 描述结果只有行/词/位置）；**整页无任何 "Confidence" 字样——Windows.Media.Ocr 不暴露置信度，硬约束 5 直接不满足** |
| 语言依赖 | 可用识别语言来自设备已装语言包（AvailableRecognizerLanguages 返回设备可用集合）；简体中文系统通常含 zh-Hans OCR 包，企业 LTSC 裁剪镜像可能缺失，须运行时探测降级 |

**Go 调用桥成本（本次核验）** `[官方仓库]`：

- GitHub 搜索 `windows media ocr language:Go` 结果为 **0**——无现成 Go 库
- saltosystems/winrt-go（MIT，51★，2026-10-16 仍活跃）：仓库树仅 181 项，`windows/media` 下只有 `.control`（SMTC），**不覆盖 Windows.Media.Ocr**——用它须自行跑其 winmd 代码生成器并长期维护生成物
- go-ole 只解决 COM/IInspectable 基础，WinRT 激活（RoGetActivationFactory）、HSTRING、IAsyncOperation 异步泵、SoftwareBitmap 编码均须手写——**手写桥无项目先例，维护风险显著高于既有 go-ole COM 先例**
- 可行替代：**PowerShell 5.1 子进程桥**（系统自带 powershell.exe 对 WinRT 有现成 projection，社区广泛使用 `[口碑]`）——进程隔离符合先例、零新依赖，但须常驻单进程批处理多图以摊薄 200~500ms 启动成本 `[推断]`；注意不能用 pwsh 7（WinRT 支持需额外包）`[口碑]`

**其他维度**：

- **中文准确率**：对清晰印刷体幻灯片可用 `[口碑]`；**无权威中文基准**，且中文离线 OCR 最流行的 Umi-OCR 支持引擎列表（本次核验 README）仅列 PaddleOCR-json/RapidOCR-json、不含系统 OCR——间接口碑信号 `[官方仓库+口碑]`
- **性能**：数量级最快（约 0.1~0.3s/页 `[口碑+推断]`），GPU/DirectX 可用时加速、CPU 自动兜底，无部署依赖 `[口碑]`
- **置信度替代口径**（若坚持采用）：`ocr_confidence` 恒 NULL + 引擎标识列区分，前端不显示低置信度徽标——语义上可行但削弱 PRD §2.5 的 OCR 检索加权设计（无置信度筛选面）。**结论：该替代口径不足以扭转排序，Windows OCR 只宜作未来「快速模式」可选档，不作主引擎**
- **CI**：GitHub windows runner 为 en-US Server，无 zh-Hans 语言包（预装清单已核验无 OCR 组件 `[官方仓库]`），中文链路无法在 CI 验证；DISM 补语言特性需管理员+网络，违约束 7

### 4.5 排除项清单与理由

| 候选 | 排除理由 | 信度 |
|---|---|---|
| gosseract / otiai10（3138★，活跃） | 链接 libtesseract C 库，需 CGO——破坏纯 Go 交叉编译与 CI，违硬约束 2；中文质量相对 Paddle 系亦无压倒性优势，不满足「压倒性优势豁免」条件 | `[官方仓库]` |
| yalue/onnxruntime_go（731★） | README 原文「you'll need a version of Go with cgo support」——CGO 明确；「自研纯 syscall 加载 onnxruntime.dll」理论上可行但等同手写桥，无现成库，风险同 WinRT 桥 | `[官方文档]` |
| onnx-go 等纯 Go 推理框架 | 算子覆盖与生态不足以跑 PP-OCR 检测+识别全链路，无可用实践 | `[口碑]` |
| tesseract.js（WebView 内嵌） | WASM 推理 5~15s/页且内存峰值高，万级页不可行；浏览器侧后台队列与 Wails 前端生命周期冲突（面板切换/刷新丢任务） | `[口碑+推断]` |
| RapidOCR Python 版（v3.10.0，2026-10 活跃） | 需随包分发 Python 运行时（数百 MB），违体积方向；其 onnx 模型可被 4.1/4.2 复用 | `[官方仓库+推断]` |
| 云 API（百度/腾讯/阿里/Azure 等） | 违硬约束 1，一票排除 | 约束 |

---

## 5. 四维对比表与万级页可行性估算

### 5.1 四维对比主表

| 维度 | PaddleOCR-json | RapidOcrOnnx bin | Tesseract 5 exe | Windows.Media.Ocr |
|---|---|---|---|---|
| **离线能力** | ✔ 完全离线 | ✔ 完全离线 | ✔ 完全离线 | ✔ 系统组件（语言包依赖） |
| **中文准确率** | PP-OCRv3/v4，中文开源事实标准，Umi-OCR 4.8 万星背书 `[官方仓库+口碑]` | 同 PP-OCR 系（v3） `[官方仓库]` | 多源口碑弱于 Paddle 系 `[口碑]` | 清晰印刷体可用，无权威基准 `[口碑]` |
| **性能（1600px 单页）** | 0.3~1.0s `[推断]` | 同左 `[推断]` | 1~3s `[口碑+推断]` | 0.1~0.3s `[口碑+推断]` |
| **置信度** | score 0~1 ✔ `[官方文档]` | score ✔ `[口碑，需实测]` | conf 0~100 ✔ `[官方文档]` | **无** `[官方文档]` |
| **分发体积增量** | 压缩包 88.4MB `[官方仓库]` | 25~30MB `[官方仓库+推断]` | 35~40MB（fast 档） `[官方仓库+推断]` | 0（语言包风险另计） |
| **CI 影响** | 下载解压 ✔ | 下载解压 ✔ | 下载解压 ✔ | 中文链路 CI 不可验 |
| **Go 集成** | exec+JSON 行协议，常驻进程 | exec+stdout 文本解析 | exec+TSV 解析 | 手写 WinRT 桥（高风险）或 PowerShell 桥 |
| **许可** | MIT + 模型 Apache-2.0 | Apache-2.0 | Apache-2.0 | 系统组件 |

### 5.2 万级页总耗时估算 `[推断]`（1600px/页，单进程基线）

| 引擎 | 单页 | 1 万页单进程 | 4~8 进程并行 | 队列可行性结论 |
|---|---|---|---|---|
| PaddleOCR-json / RapidOcrOnnx | 0.3~1.0s | 0.8~2.8 小时 | **15~45 分钟** | 可行（后台 backlog 形态，仿 thumbs 队列） |
| Tesseract 5（fast 档） | 1~3s | 2.8~8.3 小时 | 20~60 分钟 | 勉强可行，耗时与体验成本最高 |
| Windows.Media.Ocr | 0.1~0.3s | 17~50 分钟 | 5~15 分钟 | 最快，但被置信度硬约束否决主引擎资格 |
| tesseract.js | 5~15s | 14~42 小时 | — | 不可行 |

> 估算口径：并行度按消费端 CPU 物理核折算；引擎进程常驻（PaddleOCR-json 原生支持）时每页耗时含推理解码，不含进程启动。**全部数字为数量级推断，PoC 实测前不作为排期依据。**

---

## 6. 置信度口径专项（对应硬约束 5 与 PRD Open Question）

| 引擎 | 置信度获取 | 语义建议 |
|---|---|---|
| PaddleOCR-json | 文本块级 score 0~1 `[官方文档]` | 直接落 `ocr_confidence`（建议统一换算 0~100 存整数）；低置信度阈值线建议 PoC 后按 score 分布取 P10/P20 分位定标 `[需实测]` |
| RapidOcrOnnx | 文本块级 score `[口碑]` | 同上 |
| Tesseract | 逐词 conf 0~100（TSV）`[官方文档]` | 页级口径建议取词 conf 的面积加权均值或最小值，PoC 定标 |
| Windows.Media.Ocr | **不存在**（官方 API 无成员）`[官方文档]` | 替代口径 = `ocr_confidence` 恒 NULL + 引擎标识 + 前端不渲染低置信度徽标；**仅够兜底，不足以支撑主引擎** |

页级聚合口径（多文本块 → 单值）属实现期决策，本研究的结论是：**三条推荐候选均有原生块级置信度，无需启发式替代**。

---

## 7. 分发体积与 CI 影响专项

**GitHub Actions windows runner（Windows2025-Readme.md 本次核验）** `[官方仓库]`：

- 预装清单**不含 Tesseract，不含任何 OCR 组件**——三候选引擎在 CI 均须下载解压步骤
- 下载 release 压缩包 + 绿色解压均无需管理员，符合约束 7（PaddleOCR-json 7z / RapidOcrOnnx 7z / UB-Mannheim InnoSetup 可用 innoextract 免管理员抽取 `[推断]`）
- Windows runner 无 zh-Hans OCR 语言包：**Windows.Media.Ocr 中文链路在 CI 结构性不可验**；exe 类引擎无此问题（单测以 stub 注入驱动管线，引擎本体冒烟放手动 job 或真机）

**安装包体积增量汇总**：

| 方案 | 压缩包 | 估算解压/安装占用 | 备注 |
|---|---|---|---|
| PaddleOCR-json | 88.4MB `[官方仓库]` | `[需实测]`（预计 150MB+） | 含 v3/v4 模型与 OpenCV/Paddle 推理库 |
| RapidOcrOnnx bin+模型 | ~25~30MB `[官方仓库+推断]` | 最小 | 模型可换 v4/v5 rec 尝试提质 |
| Tesseract 最小组合 | ~35~40MB `[推断]` | 含 chi_sim(fast 2.4MB)+eng(3.9MB) | UB-Mannheim 完整安装包 47.9MB 含全语言，不建议整包随发 |
| Windows.Media.Ocr | 0 | 0 | 语言包缺失机器降级 |

体积敏感性结论：若 88.4MB 增量不可接受，RapidOcrOnnx 组合以约 1/3 体积提供同源（PP-OCR 系）质量，是有效折中。

---

## 8. 推荐排序与理由

### 推荐 1：PaddleOCR-json（主引擎首选）

理由：① 中文质量为候选中最强背书（PP-OCR 系 + Umi-OCR 4.8 万星生态验证）`[官方仓库]`；② score 置信度官方文档实证 ✔；③ 常驻进程 + JSON 协议 + Base64 输入对 Go 后台队列最友好，进程隔离与项目 exec/COM 双先例同构；④ MIT + Apache-2.0 许可随包分发无障碍。代价：88.4MB 分发增量与 VC 运行库探测（LTSC 裸系统）。

### 推荐 2：RapidOcrOnnx windows-bin + PP-OCRv3 onnx（体积敏感备选）

理由：同源模型质量、Apache-2.0、体积仅约 1/3；模型目录可替换，存在换 v4/v5 rec 提质的社区实践 `[口碑]`。风险：release 停滞于 2023、生态背书弱于 PaddleOCR-json、输出协议与常驻化 `[需实测]`。**若 PoC 中 PaddleOCR-json 体积被否决，此项顺位上移。**

### 推荐 3：Tesseract 5 外部 exe（第三顺位）

理由：置信度与许可齐备、集成最简单（单次 exec + TSV）、tessdata_fast 中文模型仅 2.4MB；但多源口碑中文准确率明显弱于 Paddle 系、单页耗时最高、Windows 二进制发行滞后两年 `[官方仓库]`。仅当 Paddle 系两候选在 PoC 中出现致命问题时启用。

### 不推荐（主引擎）：Windows.Media.Ocr

理由：① **官方文档实证无置信度 API，硬约束 5 直接不满足**，替代口径（NULL+降权）削弱 PRD 检索加权设计；② Go 桥成本高风险大——GitHub 无现成库（搜索 0 结果）、winrt-go 不覆盖该命名空间、手写 RoGetActivationFactory/IAsyncOperation 桥无项目先例，维护风险高于既有 go-ole COM 先例；③ 中文质量依赖系统语言包且无权威基准、Umi-OCR 未将其列为支持引擎的间接口碑信号；④ CI 中文链路结构性不可验。保留价值：未来若加「快速预索引模式」（不落置信度、允许降质），可经 PowerShell 5.1 常驻进程桥低成本接入 `[推断]`。

---

## 9. PoC 验证清单（落地前必测）

1. **中文准确率横评** `[需实测]`：采 30~50 张真实售前 PPT hd PNG（1600px），PaddleOCR-json / RapidOcrOnnx / Tesseract(chi_sim fast 与 best 各一) 四组跑识别，人工抽检字符错误率 + 图文混排/多栏/艺术字分场景统计
2. **性能实测** `[需实测]`：同数据集单页耗时分布（P50/P95）、score/conf 分布直方图、低置信度页占比；验证 4 进程并行吞吐线性度
3. **PaddleOCR-json 常驻进程稳定性**：连续数千页内存/句柄曲线、stdin 任务背压行为、进程崩溃自动重启策略 `[需实测]`
4. **体积与兼容**：解压后实测体积；裸系统（无 VC 运行库 LTSC 形态）启动探测 `[需实测]`
5. **模型升级路径**：RapidOcrOnnx 挂 PP-OCRv4/v5 rec onnx 的兼容性（若走推荐 2）`[需实测]`
6. **CI 管线**：windows runner 下载解压 + stub 驱动全管线单测；引擎本体冒烟单列手动 job `[推断→落地验证]`

---

## 10. 信源清单

| # | 信源 | URL | 信度 | 核验要点 |
|---|---|---|---|---|
| 1 | 微软 OcrEngine API 文档 | https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrengine | 官方文档（2026-10-19 抓取） | 全部成员表；无置信度成员；语言来自设备 |
| 2 | 微软 OcrWord API 文档 | https://learn.microsoft.com/en-us/uwp/api/windows.media.ocr.ocrword | 官方文档（同上） | 仅 BoundingRect/Text，全文无 Confidence |
| 3 | PaddleOCR-json 仓库 | https://github.com/hiroi-sora/PaddleOCR-json | 官方仓库 | MIT，1555★，v1.4.1（2024-08），win 包 88.4MB，PP-OCRv3/v4，score 字段，Umi-OCR 同源 |
| 4 | Umi-OCR 仓库 | https://github.com/hiroi-sora/Umi-OCR | 官方仓库 | MIT，48030★，支持引擎仅列 PaddleOCR-json/RapidOCR-json |
| 5 | RapidOcrOnnx 仓库 | https://github.com/RapidAI/RapidOcrOnnx | 官方仓库 | Apache-2.0，376★，v1.2.2（2023-02）win-bin 9MB，PP-OCRv3 模型清单，commits 2026-10 活跃 |
| 6 | RapidOCR 主仓库 | https://github.com/RapidAI/RapidOCR | 官方仓库 | Apache-2.0，8179★，v3.10.0（2026-10-08） |
| 7 | PaddleOCR 官方仓库 | https://github.com/PaddlePaddle/PaddleOCR | 官方仓库 | PP-OCRv5 及后续版本演进描述（主页原文） |
| 8 | UB-Mannheim Tesseract | https://github.com/UB-Mannheim/tesseract/wiki | 官方仓库 | 最新 5.4.0.20240606（2024-06），安装包 47.9MB |
| 9 | Tesseract 官方 | https://github.com/tesseract-ocr/tesseract/releases | 官方仓库 | 最新 5.5.3（2026-07-24） |
| 10 | tessdata_fast / tessdata_best | https://github.com/tesseract-ocr/tessdata_fast ｜ https://github.com/tesseract-ocr/tessdata_best | 官方仓库 | chi_sim 2.4MB/12.5MB，eng 3.9MB/14.7MB，osd 10.1MB |
| 11 | saltosystems/winrt-go | https://github.com/saltosystems/winrt-go | 官方仓库 | MIT，51★，2026-10-16 活跃；树内不覆盖 Windows.Media.Ocr |
| 12 | otiai10/gosseract | https://github.com/otiai10/gosseract | 官方仓库 | 3138★，基于 Tesseract C++ 库（CGO） |
| 13 | yalue/onnxruntime_go | https://github.com/yalue/onnxruntime_go | 官方仓库 | README 明示需 cgo 支持 |
| 14 | runner-images（Windows 预装清单） | https://github.com/actions/runner-images | 官方仓库 | Windows2025 镜像无 Tesseract/OCR |
| 15 | tesseract.js | https://github.com/tesseract-ocr/tesseract.js | 已知仓库（星标未核验） | WASM 形态 |
| 16 | PowerToys Text Extractor（Windows OCR 参考实现） | https://github.com/microsoft/PowerToys | 已知（本次未逐条核验） | 采用 Windows.Media.Ocr 的成熟桌面先例 |

---

## 11. Caveats / 未决事项

- **中文准确率无量化基准**：检索引擎在本环境不可达，§4 各「口碑」级结论未附第三方横评数据，PoC（§9.1）是唯一可靠定标手段
- **全部耗时数字为数量级推断**，非实测；排期估算须以 PoC 数据替换
- PaddleOCR-json **解压后体积、常驻进程内存基线**未实测
- RapidOcrOnnx 的 stdout 输出协议细节与常驻化可行性未核验
- Windows OCR 的 GPU/DirectX 加速行为为社区共识口径，官方文档未明示
- GitHub windows runner 是否可装 zh-Hans OCR 语言包未验证（DISM 路径违约束 7，未深究）
- sister 文档 `research/windows-media-ocr-go.md`（prd.md:88 预留）若立项，应承接 §4.4 的桥接成本细目与 PowerShell 桥原型数据
