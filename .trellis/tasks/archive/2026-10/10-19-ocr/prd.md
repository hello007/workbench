# 素材管理第十一批：单页 OCR 与检索集成

> SRS：docs/原始需求/202610-素材管理系统需求说明书-V1.0.md（§2.7.2 PPT 处理配置 OCR 开关、§2.2.4 幻灯片单页索引 OCR 文本+置信度、§2.5 检索字段加权 OCR 项）
> 状态：prd 定稿（2026-10-19），未进入实施

## Goal

图片型幻灯片（截图/图表为主的售前 PPT 主力形态）当前检索不到：解析链纯 Go OOXML 只提取文本框 XML 文本。本批引入本地离线 OCR 引擎 PaddleOCR-json，识别单页图片文字写入 ocr_text 并入 FTS 检索，实现「关键词命中图片型单页」；含 OCR 开关设置段、队列化补跑、置信度语义与检索呈现。

## Requirements（全部定案）

1. **引擎**：PaddleOCR-json v1.4.x（hiroi-sora，MIT + 模型 Apache-2.0）外部 exe 常驻进程，stdin JSON 任务 / stdout JSON 回包，Go 侧零新依赖（exec.Command 进程隔离，与 util/git.go exec+ctx、assets_thumbs COM 队列双先例同构）。
2. **分发**：引擎 exe + PP-OCRv3/v4 模型随应用包分发，固定相对路径（安装目录 ocr/ 下）；开发机/CI 经下载脚本获取（解压即用，免管理员）；引擎缺失 = OCR 功能不可用（设置面板显示「引擎未就绪」，不报错不阻塞扫描与缩略图链路）。
3. **开关**：全局设置 `AppSettings.assets.ocr` 段（enabled 默认 **true**）；关闭时不补跑、已有 ocr_text 保留照常检索。
4. **存储**：slides 表迁移加 `ocr_text TEXT NOT NULL DEFAULT ''` / `ocr_confidence REAL`（user_version 递增）。OCR 结果随 slides 行存续：unchanged 跳过天然保持；重解析删旧插新即作废，队列重新补跑。
5. **触发**：扫描不同步 OCR；扫描落库后异步队列补跑缺失页（仿 thumbs backlog 形态）；详情侧栏单页手动重试（仿 RetryAssetThumb）。
6. **置信度**：块级 score（0-1）页面级聚合取均值；识别成功零文本块 = ocr_text 空 + confidence NULL；低于阈值 0.6 前端「识别可信度低」标记。阈值定标以 PoC score 分布为准（PoC 清单见 research §9）。
7. **FTS 集成**：search_fts 重建加 ocr_text 列（FTS5 虚表无 ADD COLUMN：建新表→回填→DROP→RENAME；回填写入侧经 splitFtsText 同源切分）；bm25 权重 9 参数：slide_title 3.0 > slide_text 2.0 > **ocr_text 1.5** > file_note/slide_notes 1.0（SRS §2.5 序）。
8. **OCR 任务键**：(file_id, slide_num)，禁 slides.id；补跑写回前校验：行仍存在 + files.hash 与入队时一致（文件已变则丢弃，重解析链路会重新入队）。
9. **输入源**：hd 缩略图 PNG（data/assets/<lib>/thumbs/hd/<fileID>-<NNN>.png，绝对路径传引擎——COM 绝对路径契约同理）；PNG 缺失不入队待补（缩略图链路补齐后下次入队）。无 PowerPoint 机器自然降级。
10. **前端呈现**：OCR 命中页摘要带「OCR」来源标；低置信度标记；设置项 UI（开关 + 引擎就绪状态展示）。

## Technical Approach

### 引擎包装层（service/assets_ocr_engine*.go）
- 常驻子进程管理：启动（传模型目录 init 参数）→ stdin 逐行任务 → stdout 逐行 JSON 回包解析；请求-响应序号配对；引擎崩溃自动重启（一次任务内重试一次，再败标记该页失败不阻塞队列）。
- 超时：单页任务超时杀进程重建（杀进程集合仅 PaddleOCR-json.exe 自启实例，禁 taskkill /IM 全杀——COM 先例）；ctx 取消传播。
- 接口抽象 `ocrEngine`（Recognize(ctx, pngPath) (text, confidence, err)）：单测 stub 注入，引擎真机质量归 PoC/冒烟。
- 平台分叉：引擎仅 Windows 分发，非 Windows 构建走不可用桩（_windows/_other 双文件先例）。

### 队列编排层（service/assets_ocr.go）
- 仿 thumbs backlog：扫描完成后收集「ok 文件 + hd PNG 存在 + ocr_text 为空」页入队；串行消费（引擎 CPU 密集，PoC 后评估多进程）；开关关闭/引擎未就绪不入队。
- 取消链路：独立 ocrCancels/ocrLocks（复用素材域 opMu 形态，不跨域借锁）；CancelAssetOcr 库级取消；关停链 WaitOcrDone 插 CloseAll 前（thumbs 先例）。
- 事件：`assets-ocr:*` 独立前缀（EventSink 契约）；**web_serve.go applySink 须补 SetEventSink 行**（漏加 = 浏览器通道静默收不到）。
- 进度事件带 libraryId 前端按库过滤；error 事件只复位不弹提示（防与 Promise catch 双弹——assets-scan 先例）。

### 写回与 FTS（service/assets_db.go 扩展）
- 写回单事务：按 (file_id, slide_num) UPDATE slides.ocr_text/ocr_confidence + 同事务 UPDATE FTS 行（rowid=-slideID 事务内一致）ocr_text 列（splitFtsText 切分）。
- schema 迁移 v+1：slides 加列（ALTER TABLE ADD COLUMN）+ search_fts 重建加列回填（backfillFtsOnUpgradeV2 先例，回填 ocr_text 全空）+ assetsFtsBm25Args 更新 9 参数。

### 检索呈现（service/assets_search.go + 前端）
- snippetFromTexts 候选列追加 ocr_text（列优先级：title > text > ocr > notes?——按 §2.5 加权序对齐，实现期定）；OCR 来源判定用 FTS5 hits() 列掩码或命中列信息扩展 assetsFtsHit（实现期细化）。
- 前端：命中摘要「OCR」标 + 低置信度标（<0.6）+ 详情侧栏单页重试按钮 + 设置面板开关。

### PR 拆分
- **PR1 schema+设置**：迁移（slides 加列 + FTS 重建 + bm25 9 参数）+ AppSettings.assets.ocr 段（normalize/clamp 仿 thumbs）+ 设置 UI + wailsjs 绑定三处同步 + mock。无 OCR 行为变化，全绿可独立合入。
- **PR2 引擎+队列**：引擎包装层（进程管理/协议/超时/重启/平台桩）+ backlog 入队/消费/写回/取消/事件 + web_serve applySink 补行 + 单页重试 API。
- **PR3 检索+呈现**：snippet 候选列扩展 + 命中来源标 + 低置信度标 + 前端重试按钮/引擎状态。
- **PR4 分发+PoC+冒烟**：下载脚本 + build 白名单 + CI stub 化 + PoC 清单执行（score 分布定标阈值 0.6 校准）+ 真机冒烟（含八/九/十批遗留清单）。

## Decision (ADR-lite)

**Context**：引擎选型是本批最大不确定点，两路 research 交叉验证（ocr-engine-comparison.md 四维对比 + windows-media-ocr-go.md WinRT 桥实测）。
**Decision**（2026-10-19 用户拍板）：
1. 主引擎 = PaddleOCR-json：置信度 score 0-1 官方文档实证（Windows.Media.Ocr 无置信度成员——官方文档 + 本机 Get-Member 双重实证，硬约束不满足）；中文质量最强背书（PP-OCR 系 + Umi-OCR 4.8 万星同源）；88.4MB 分发增量可接受（用户确认）。
2. 默认开：引擎随包分发零额外成本，开箱即用。
3. 存储 = slides 行加列（否决独立表+位置重绑/hash 精确复用）：OCR 属页内容派生数据，与缩略图失效重导同构。
4. 触发 = 扫描后异步队列补跑 + 单页手动重试；开关全局段；FTS ocr_text 权重 1.5；置信度均值口径阈值 0.6；摘要带 OCR 来源标。
**Consequences**：文本微改但图未变的文件整文件重跑 OCR（接受）；识别质量与耗时数字均为 research 推断级，PR4 PoC 实测定标前不作排期依据；Windows.Media.Ocr 桥可行性已固化（go-bindings-winrt 本机跑通 31-43ms/页），未来可作「快速模式」可选档。

## Acceptance Criteria

- [ ] 图片型幻灯片（无文本框有截图文字）经 OCR 后关键词可检索命中该单页，摘要带「OCR」来源标。
- [ ] 低置信度（<0.6，PoC 校准后取值）命中页带「识别可信度低」标记。
- [ ] OCR 开关关闭：不触发补跑；已有 ocr_text 照常检索与呈现。
- [ ] 引擎缺失/未就绪：扫描、缩略图、检索全链路不受影响，设置面板显示未就绪。
- [ ] 文件 unchanged 重扫描不重复 OCR。
- [ ] 文件变更重解析后旧 OCR 结果作废重跑；补跑写回前 hash 校验防串写。
- [ ] FTS 重建迁移后旧数据检索行为不变；ocr_text 命中权重位于 slide_text 与备注之间（bm25 分数可证）。
- [ ] OCR 队列可取消；单页超时/失败不阻塞队列；引擎崩溃自动重启；assets-ocr:* 事件桌面与浏览器通道均可达。
- [ ] 引擎测试经 stub 注入，无 sleep/真实事件等待；service ≥76% / 前端 ≥70% 门禁。
- [ ] PoC 清单执行完毕（score 分布、单页耗时 P50/P95、常驻进程稳定性），阈值 0.6 经真机数据校准。
- [ ] 真机冒烟通过（含八/九/十批遗留清单）；引擎链路冒烟禁 fixture 掩蔽（真实引擎进程）。

## Definition of Done

- 单测 + 真机冒烟通过；trellis-check + 独立审核（cavecrew-reviewer 两轮制）通过。
- spec 沉淀（docs/spec/assets-ocr.md 新域）+ CLAUDE.md 关键规则同步。
- README/功能说明/部署说明更新（引擎分发与下载脚本）。

## Out of Scope

- 云端 OCR API（违背 SRS 本地不上云硬约束）。
- Windows.Media.Ocr 作主引擎（无置信度，硬约束不满足）；「快速模式」可选档留未来。
- .ppt 老格式、PDF/图片素材的 OCR（本批仅 pptx 单页）。
- OCR 结果 AI 摘要（summary 保留列继续恒空）。
- 多进程并行 OCR（PoC 后单进程吞吐不足再议）。
- 引擎在线下载/自动更新（固定分发 + 手动脚本）。
- 独立 OCR 结果表 + 页图 hash 精确复用（已否决方案 B/C）。
- 颜色检索等 P1 其他项。

## Technical Notes

- **PoC 必测清单**（research/ocr-engine-comparison.md §9，PR4 前置）：真实售前 PPT 30-50 页横评、score 分布直方图与 0.6 阈值校准、单页耗时 P50/P95、常驻进程数千页内存/句柄曲线与崩溃重启、解压后体积、LTSC 无 VC 运行库探测（VCOMP140.DLL 缺失降级提示）。
- PaddleOCR-json 协议细节（v1.4.1）：常驻进程 stdin JSON 任务（image_path 模式）/ stdout JSON 回包（data[].text/score）；init 参数含模型路径；实现期以官方 README 协议文档为准，协议解析层独立单测。
- FTS 迁移顺序：v+1 单迁移内完成 slides 加列 + search_fts 重建；重建回填 INSERT SELECT 须带 ref_type/ref_id/file_id 冗余列；迁移后 TestZZProbeZeroTokenPhrase 等守护用例回归。
- bm25 参数序 = 列序：新列 ocr_text 插在 slide_notes 之前（file_name 4.0, file_note 1.0, slide_title 3.0, slide_text 2.0, ocr_text 1.5, slide_notes 1.0），重建表列序与 assetsFtsBm25Args 同步。
- 写回竞态收口：入队时快照 files.hash，写回事务内比对；不一致丢弃（文件已被重解析链路接管）。
- 设置链路复用 thumbs 先例：段缺失全默认、SettingsService 共享指针只读禁改、运行期改设置重启生效口径须在设置 UI 注明（或热生效——实现期按 thumbs 口径对齐）。
- 引擎路径探测：启动时探测固定相对路径存在性，结果缓存服务级；设置面板只读展示。
- 事件前缀 assets-ocr:* 独立注册；LocalChanges 等共听组件须 EventsOn 返回闭包精准注销（共听契约）。

## Research References

- [research/ocr-engine-comparison.md](research/ocr-engine-comparison.md) — 四维对比：PaddleOCR-json 首选 / RapidOcrOnnx 体积备选 / Tesseract 第三 / Windows.Media.Ocr 无置信度否决主引擎；PoC 清单 §9
- [research/windows-media-ocr-go.md](research/windows-media-ocr-go.md) — WinRT 桥实测：go-bindings-winrt 纯 Go 跑通 31-43ms/页；OcrWord 无置信度双重实证；MTA 模型与 STA 队列不同构；语言包静默 null 坑
