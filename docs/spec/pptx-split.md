# PPT 按页拆分（PptxSplit）契约

> 来源：任务 10-13-ppt-split-per-page（2026-10-13）。技术选型与最小编辑集依据见任务 `research/pptx-split-approaches.md`。

## 1. Scope / Trigger

工具箱功能：对 .pptx 按页拆分，输出到同级同名文件夹，每页一个独立新 pptx。纯 Go OOXML 实现（标准库 `archive/zip` + `encoding/xml`，零第三方依赖），禁引入 COM / 外部 Office 依赖（CI 不可测 + 无安装保证）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 核心 | `SplitPptxPerPageWithProgress(ctx, srcPath string, onProgress func(done, total int)) (model.PptxSplitResult, error)`；`SplitPptxPerPage(ctx, srcPath)` 为 nil 回调直通 | `service/pptx_split.go` |
| 编排 | `PptxSplitService.Split(ctx, srcPath) (model.PptxSplitResult, error)`；自持 `opMu + cancels` 注册表，复用包内 `cancelEntry` | `service/pptx_split_service.go` |
| 批量编排 | `PptxSplitService.SplitFolder(ctx, dirPath) (model.PptxBatchResult, error)`（2026-10-19 增补：递归扫描 + 串行拆分 + 失败续跑，复用同一取消注册表与事件出口） | `service/pptx_split_batch.go` |
| App 绑定 | `SplitPptxPerPage(srcPath string) (model.PptxSplitResult, error)` / `SplitPptxFolder(dirPath string) (model.PptxBatchResult, error)` / `CancelPptxSplit() bool`（无参 = 全量取消，同时命中单文件与批量在途项） | `app_pptx.go` |
| 事件 | `pptx-split:progress` `{srcPath, done, total}` / `pptx-split:done` `{srcPath, result}` / `pptx-split:error` `{srcPath, message}` / `pptx-split:batch:progress` `{dirPath, index, total, currentFile}`（文件级，批量独有） | EventSink 通道 |
| 结果模型 | `model.PptxSplitResult{OutputDir string; Files []string; PageCount int}`；`model.PptxBatchResult{DirPath string; Total int; Items []PptxBatchItem}`，Item `{SrcPath, OutputDir, PageCount, Error}`（均普通字段，无具名 string 类型，不触发 models.ts 别名规则） | `model/pptx.go` |

## 3. Contracts

- **最小编辑集**（拆分 = 复制整个 zip 后做一次列表编辑，保留 part 逐字节原样拷贝）：
  1. `ppt/presentation.xml` `<p:sldIdLst>` 删其他页条目
  2. `ppt/_rels/presentation.xml.rels` 删对应 Relationship
  3. `[Content_Types].xml` 删除已删 part 的 `<Override>`
  4. 删 `ppt/slides/slideN.xml`（+ `.rels`）与所属 `ppt/notesSlides/notesSlideN.xml`（+ `.rels`）
  5. 剥离 `<p:custShowLst>` 与根级 `<p:extLst>`（sections 在 extLst 内，整块剥离）
  6. 保留页 rels 中指向已删页的 jump-to-slide 关系清理（rel type 以 `/slide` 结尾，保留 External 外链与自身跳转）
- **B 档体积 GC**（2026-10-14 增补，任务 10-14-ppt-srs-2-2-3）：
  - 动机：最小编辑集只删页/备注页 part，其余页独占媒体与未用母版/版式/主题全量随包携带（实测 10MB 级源拆出 9MB/页）。
  - 机制：保留页真实渲染链路「页 → 版式（rels `/slideLayout` 后缀）→ 母版（rels `/slideMaster` 后缀）→ 主题」+ 包根 .rels（docProps）+ presentation 级非页引用（presProps/viewProps/tableStyles/notesMaster）为根做 rels 图可达性闭包（`splitGCContext`，图一次构建全页复用）；被剪母版/版式作屏障（其下挂资源不可达）；闭包外 part 全部进删除集。
  - 联动编辑四处：presentation.xml `<p:sldMasterIdLst>` 剪至保留母版条目、presentation.xml.rels 悬挂关系清理（既有谓词覆盖）、保留母版 rels 移除指向被剪版式的 Relationship、**保留母版本体 `<p:sldLayoutIdLst>` 剪除被剪版式条目**（母版 XML 逐字节拷贝但条目经 r:id 引用版式关系——rels 删除后条目残留即悬空引用，真实文件每母版默认 11 版式必触发 PowerPoint 修复弹窗；独立审核 P0 抓出，fixture 母版空元素曾结构性掩蔽）。
  - **降级红线**：页 rels 缺版式跳 / 版式 rels 缺母版跳 / 保留母版不在 sldMasterIdLst → 该页整段 GC 关闭（母版全保留，仅删页与备注页 = 旧行为）。GC 是体积优化非正确性依赖，保真优先。
  - 主题不显式剪：保留母版的主题经图可达保留，被剪母版的主题因屏障自然剪除。
  - 收益实测（48 页图文密集源）：单页解压 13.44MB → 平均 3.12MB；体积下限受源母版背景媒体约束（保真必须携带）。
- **页序三跳映射**：sldIdLst → rId → presentation.xml.rels → part 名。slide 文件名与展示页序无关，禁按文件名序拆。
- **输出**：同级 `<原文件名>/` 文件夹，文件名 `<原文件名>-<三位页序号>.pptx`（2026-10-14 起 %02d→%03d：≥100 页字典序与页序一致；`isPptxSplitDerivative` 纯数字判定两位三位通吃，排除面零影响）；文件夹已存在覆盖重生成。**核心不清理目录**：页数减少/改名残留的旧文件由调用方处理——素材拆分链路重拆前清理（derived 落点 RemoveAll 整目录；源目录落点仅逐文件删产物命名形态，用户自建内容不动）；工具箱链路维持旧行为（重拆后新旧命名并存属已知取舍）。素材域 replace 落点在核心输出后二次改名为 `<NNN>-<主干>.pptx` 并进索引，见 §9。
- **原子写**：`os.CreateTemp` 同目录 → 写入 → Sync → `os.Rename`（复刻 `util/json.go` SaveJSON 序列；二进制 zip 不适用 SaveJSON 本体）。
- **取消**：`context.WithCancel` + `registerCancel`/`unregisterCancel` defer LIFO（先注销后 cancel），键 `normalizeRepoKey(srcPath)`，取消错误 `%w` 包装 `context.Canceled` 经 `mapCancelError` 映射 `ErrOperationCancelled`（**复用** `E_GIT_CANCELLED`，不新增错误码）。
- **文件夹批量拆分**（2026-10-19 增补，任务 10-19-ppt）：`SplitFolder` 递归扫描（`filepath.WalkDir` 字典序）目录下全部 .pptx 串行逐个拆分；产物排除复用素材域 `isPptxSplitDerivative` 单点（rel 路径先 `filepath.ToSlash` 归一，Windows 反斜杠兼容）；目录不可入降级 SkipDir 记 Warn 不中断。单文件失败经 `appErrMessage` 记 `Item.Error` 后继续（不中断整批）；取消双检查点（每文件开始前 `ctx.Err()` + 单文件错误返回后 `errors.Is(context.Canceled) || ctx.Err()` 兜底）——取消非单文件业务失败，整批映射 `E_GIT_CANCELLED` 上抛、Items 仅含已完成。**批量模式不逐文件发 done/error 事件**（终态仅经返回值汇总，单文件失败展示唯一通道 = `Item.Error`）；整批取消时 error 非 nil，Wails 通道丢弃返回值（reject 收尾），已完成文件仅物理保留在磁盘。
- **事件注入**：走 `s.emitCurrent`（EventSink 持锁投递契约），禁直调 `runtime.EventsEmit`；`web_serve.go` `applySink` 已补 `SetEventSink`（现七服务，含 pptxSplit/assets）。

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 扩展名非 .pptx（大小写不敏感，.PPTX 接受；.ppt 拒绝） | 明确错误「仅支持 .pptx」 |
| zip 损坏 / 缺核心部件 / sldIdLst 空 / rId 悬空 | 中文可读错误，不留半态（临时文件 defer 清理） |
| ctx 已取消 | `E_GIT_CANCELLED` warning 中性提示 |
| 节内嵌 `p14:sldId`（与页条目局部名相同） | depth==2 限制 + 删除中跳过谓词评估双保险，不误编辑 |

## 5. Good/Base/Bad Cases

- Good：多页乱序命名（slide3→slide1→slide2）拆分后每包 sldIdLst 恰 1 页、页序正确、保留 part 逐字节一致
- Base：单页 PPT 拆出 1 个；覆盖重生成已存在输出目录
- Bad：按 slide 文件名序拆（页序错）；用 Encoder 重序列化整个 XML（保真度损失——本项目用「解析定位 + 字节删除」`dropXMLSpans`，基于 `Decoder.InputOffset` 切区间）

## 6. Tests Required

fixture 用 `archive/zip` 程序化构造最小合法 pptx（禁依赖本机 Office / PowerShell / sleep）：
- 数量断言：N 页出 N 文件；每输出包 sldIdLst 仅 1 页
- 结构断言：无悬空 Override、无悬空关系、已删 part 不存在、custShowLst/extLst 已剥离
- 字节断言：未触碰 part 与被编辑文件未删区间逐字节一致
- GC 断言（`pptxFixture.masters` 多母版 + `orphanMedia` 孤儿媒体 + 版式独占媒体）：被剪母版/版式/主题及其媒体不在输出；sldMasterIdLst 剪至一条；保留母版 rels 无被剪版式关系；**保留母版本体 sldLayoutIdLst 无被剪版式条目**；保留链 part 逐字节一致；链路不可解析（`noLayout`）时母版全保留且母版本体零编辑（降级形态）。通用断言面含**part→rels 方向 r:id 可解析**（rels→part 与 Override 悬空之外的反向引用面，fixture 母版带 sldLayoutIdLst 真实形态——空元素母版曾结构性掩蔽悬空引用 P0）
- 取消：ctx 预取消驱动全链路（错误码 + error 事件 + 注册表无残留）
- 进度：逐页回调时序（done 1→N/total N）+ 事件序列载荷断言
- 服务层：provider fake 注入（对齐 `ProcessProvider` 模式）

## 7. Wrong vs Correct

**Wrong**：OPC 关系 Target 解析以 `_rels/` 目录为基准。
```go
// 错：Target 相对 _rels 所在目录
base := filepath.Dir(relsPath) // ppt/_rels → Target "slides/slide1.xml" 解析错
```
**Correct**：Target 相对**关系源 part 所在目录**。
```go
// 对：presentation.xml.rels 的源 part 是 ppt/presentation.xml，Target 相对 ppt/
base := filepath.Dir(strings.TrimSuffix(relsPath, "/_rels/presentation.xml.rels"))
```
全包悬空关系检查测试可暴露该错误（已实测暴露并修正）。

## 8. 前端契约

- 2026-10-19 起工具箱全窗口面板化：`PptSplitDialog.vue` 弹窗已删除，事件监听方为工具箱内嵌 `PptSplitPanel.vue`（单文件/文件夹批量双模式，`component :is` 切换即卸载重建，监听器生命周期 = 组件挂载/卸载）；`FileTreePanel.vue` 右键入口刻意不监听（Promise 直返 + message 反馈），规避多组件共听面。组件卸载用 `EventsOn` 返回闭包精准注销，禁 `EventsOff`。
- 关闭/切走即重置输入/结果/在途态；后端在途任务继续跑，事件载荷经过滤防串入：单文件按 `pendingSrcPath`、批量文件级按 `pendingDirPath`、批量页级 `progress` 按当前文件 `batchCurrentFile`。
- error 事件只复位状态不弹提示（提示由 Promise reject 的 `handleError` 统一分流，防双弹）；`E_GIT_CANCELLED` 命中 WARNING_CODES 中性提示。
- wailsjs 三处绑定（App.js / App.d.ts / models.ts）经 `wails generate module` 重生成，不入库。

## 9. 素材域拆分落点三态（B 方案，2026-10-31）

> 素材管理面板复用本文件拆分核心（`SplitPptxPerPageWithProgress` 签名与行为不动），落点编排集中在 `service/assets_split.go`。工具箱面板不受影响，恒为「源目录同名文件夹」单态。

### 9.1 三落点对照

| 落点 | 产物路径 | 是否进索引 | 源文件 |
|---|---|---|---|
| `cache`（默认） | `data/assets/<libId>/derived/<镜像源目录>/<源主干>/<源主干>-NNN.pptx` | 否（派生排除） | 保留 |
| `source` | `<源目录>/<源主干>/<源主干>-NNN.pptx` | 否（派生排除） | 保留 |
| `replace` | `<源目录>/<源主干>/<NNN>-<名字主干>.pptx` | 是（每页一条 `files` 行） | 全页成功后移入系统回收站 |

- 「名字主干」= `replaceArtifactBase(源主干)`：源主干为 ≥3 位纯数字时追加 `-p`（`123` → `123-p`），见 9.2。
- 落点参数归一单点 `normalizeSplitTarget`：空串兼容旧调用等价 `cache`；非法值明确拒绝，禁静默落到意外落点。
- `cache` 根目录文件恰为 `derived/<源主干>/`，子目录文件多一层镜像目录（同库不同目录同名 pptx 不撞目录）。

### 9.2 可索引命名契约与隐式解耦

- **最终命名**：`replaceArtifactName(page, base)` = `fmt.Sprintf("%03d-%s.pptx", page, replaceArtifactBase(base))`；`base` = 源文件名去扩展名。
- **中间态**：核心仍按本文件 §3 输出 `<base>-NNN.pptx`，编排层逐页 `os.Rename` 为 `<NNN>-<stem>.pptx`，随后 `os.Stat` 校验（存在且 `Size > 0`）；页序取核心产物文件名尾段（`pageSeqFromArtifactName`），解析失败回退结果切片序号 + 1。
- **纯数字主干规避**：`replaceArtifactBase` 对「≥3 位纯数字」主干追加 `-p`（`123` → `123-p`）。原因是 `isPptxSplitDerivative` 判定「父目录名 + `-` + 纯数字」，而 replace 产物父目录恰为 `base`、页序段 `%03d` 最小宽度 3 位（≥1000 页不补零）——`base=123` 第 123 页会产出 `123-123.pptx` 被扫描静默排除。1~2 位数字不可能与 ≥3 位页序段等长，无需后缀。
- **隐式耦合（改判定函数必读）**：`isPptxSplitDerivative` 零改动，三态区分完全依赖命名形态。后续若放宽为「任意父目录 + `-` + 数字」等更宽规则，会误伤 replace 产物使其退出索引。修改该判定函数须同步本节与回归用例。
- **用户侧边界**：把产物重命名为 `<源主干>-001.pptx` 会重新命中排除规则，下次扫描退出索引；就地替换弹窗的强确认区已显示该禁用命名提示（单文件回显实际主干，批量用 `<源名主干>` 占位）。

### 9.3 删源安全时序（`splitOneFileReplace`）

严格按序执行，任一步失败口径见下：

| 步 | 动作 | 失败处置 |
|---|---|---|
| 1 | 源哈希（快照为空时现算）+ 源 `Stat` 基线（size + mtime） | 可读性失败 → 单文件失败 |
| 2 | 增量四验：记录落点 = replace + 哈希一致 + **未删源**（`!source_removed`）+ 首页产物存在 | 命中则 `skipped`，不重拆；源从回收站还原后不放行跳过 |
| 3 | 清旧产物：只删**已索引**的已改名 `<NNN>-stem.pptx` 并删索引行（核心中间态命中派生排除，残留不索引） | 删除失败告警继续 |
| 4 | 调拆分核心（产物落 `outDir`，仍为核心原生命名） | 取消/失败 → 回滚产物 |
| 5 | 逐页 rename 为可索引命名 + 逐页 `Stat`（存在且非空） | 回滚全部产物 |
| 6 | 逐页 `processScanFile` 建索引，回读产物 `fileID`；产物路径命中库排除规则则拒绝 | 回滚产物与已写索引行 |
| 7 | `migrateSplitAnnotations` 单事务预迁移标注 | 回滚产物与索引行，源不删 |
| 8a | 删源前取消检查点 + 源 `Stat` 复核（size/mtime 与基线一致） | 取消或源被修改 → 回滚产物与索引行，源不删 |
| 8b | `util.MoveToRecycleBin(源)` | 不回滚，返回删源失败码（产物与源均保留） |
| 9 | `finalizeReplaceSource` 单事务：重迁移晚到标注 + 删源索引行 | 仅告警，下次扫描标 lost |
| 10 | `upsertSplitRecord(target=replace, source_removed=1)` | 仅告警（留痕职责） |
| 11 | 返回 `sourceRemoved=true` 成功结果 | — |

- **3-7 任一失败**：`rollbackReplaceArtifacts` 只删本次新增（核心调用前目录快照之外）的产物、索引行与缩略图，源文件与源行原样保留，明细 `rolledBack=true`；拆分前已存在的同形用户文件不动。
- **取消三检查点**：核心返回取消错误、核心返回后、删源前。取消同样回滚后返回，`mapAssetSplitCancelError` 映射 `E_ASSETS_CANCELLED`（复用码，不新增）。
- **8b 失败**：产物、索引行、标注副本均保留，源与源行亦保留，返回 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`（warning）；用户关闭占用程序后可重试，重拆覆盖产物。
- **批量入口失败分两类**：重命名/Stat/产物解析不可用等业务失败逐项填入 `Item{status:failed, code, rolledBack}` 继续后续文件；数据库写错误、标注事务错误与取消属系统性错误，整体中止且不追加 Item（前序已完成文件保持结果）。

### 9.4 回收站语义（`util.MoveToRecycleBin`）

- 公共前置校验：非空、绝对路径、`Lstat` 存在；失败即源文件原位不动。
- Windows：`SHFileOperationW` + `FOF_ALLOWUNDO | FOF_WANTNUKEWARNING | FOF_NOERRORUI`（`recycleFlags` 单点）。**禁 `FOF_NOCONFIRMATION` / `FOF_SILENT`**：回收站关闭、超配额或网络盘时 Shell 会退化为永久删除，静默标志会让该退化无提示且返回 0；`FOF_WANTNUKEWARNING` 使永久删除前弹警告，用户取消 → `fAnyOperationsAborted != 0` → 返回错误、源保留。Linux：freedesktop Trash（`$XDG_DATA_HOME/Trash`，`.trashinfo` 先写后移动，失败清理信息文件，跨设备复制路线回滚目标副本）。
- 语义 = 可恢复删除，不是永久删除；这是删源不可逆提示以外用户唯一的恢复手段。Windows 两场景（回收站关闭 / 超配额）须实机验证。

### 9.5 拆分记录（schema v8）与重拆

- `split_records` 增两列：`target TEXT NOT NULL DEFAULT 'cache'`、`source_removed INTEGER NOT NULL DEFAULT 0`；存量记录默认值语义正确（旧落点从不删源）。v8 迁移经 `migrateSchemaV8` 列存在性预判 + 单事务，半迁移库可重开。
- replace 增量判定为四验（见 9.3 步 2），任一不满足即重拆；`source_removed=1` 但源文件已从回收站还原时同样重拆。
- 重拆清旧只删已索引的已改名形态并联动 `deleteIndexRowsForReplacePaths` 删索引行与缩略图，防页数减少残留僵尸素材；未索引残留与用户自建文件不动。
- `OpenAssetSplitPage` 按 `record.target` 选命名：replace 用 `<NNN>-<名字主干>.pptx`，其余用 `<主干>-<NNN>.pptx`。
- `GetAssetSplitRecord` 回显 `target`/`sourceRemoved` 供展示与诊断；前端当前仅用 `exists` 决定「打开拆分单页」入口显隐，不据此禁用重拆（源还原后允许重拆）。

### 9.6 与扫描的互斥（锁序 scan → split）

replace 要为产物写 `files/slides` 索引（复用 `processScanFile` 单文件重链），与在途扫描并发写会互相覆盖。故 replace 先 `tryLockScan`（失败 `E_ASSETS_SCAN_IN_PROGRESS`）再 `tryLockSplit`，锁序与 `DeleteLibrary` 一致；`cache/source` 不写索引，仅取拆分锁，行为与改动前一致。取消注册表 `splitCancels` 与扫描 `cancels` 键空间独立，互不误杀。

### 9.7 补充测试要求

- 三落点回归：`cache/source` 与改动前逐字一致；`replace` 全链成功（源消失、每页一条索引、页数均为 1）。
- 失败/取消：产物、索引行、缩略图零残留，源保留，`rolledBack=true`。
- 删源失败：`E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`，产物与源均保留。
- 命名边界：纯数字主干 `123` / `1234` 加 `-p`；`isPptxSplitDerivative` 对 replace 产物返回 false。
- 标注迁移：文件级与页级两组断言；回收站实现双平台分别单测。
- 安全回归：源拆分期间被修改 → 回滚；源还原后重拆不跳过；排除规则命中 → 拒绝；v8 半迁移库可重开；回收站标志不含 `FOF_NOCONFIRMATION`/`FOF_SILENT`。




