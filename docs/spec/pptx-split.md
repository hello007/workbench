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
- **输出**：同级 `<原文件名>/` 文件夹，文件名 `<原文件名>-<三位页序号>.pptx`（2026-10-14 起 %02d→%03d：≥100 页字典序与页序一致；`isPptxSplitDerivative` 页序段要求 ≥2 位纯数字（两位/三位/四位通吃），排除面零影响；1 位页序用户文件不排除）；文件夹已存在覆盖重生成。**核心不清理目录**：页数减少/改名残留的旧文件由调用方处理——素材拆分链路**核心成功后**才收敛旧页（`removeSplitArtifactsBeyond` 删页序 > 本次 PageCount 的产物，以及页序 ≤ PageCount 但为历史 `%02d` 形态、非当前 `%03d` 形态的产物；主干段 Windows 折叠大小写；mtime 晚于上次 `split_at` + 2s 视为用户文件保留；删除路径联动 `deleteIndexRowsForPaths`；source/cache 同口径，**禁拆分前删除或 RemoveAll 整目录**，核心失败/取消时上一次成果不再被前置删除（核心已同名覆盖写的页属已知限制，彻底解决需核心写临时目录再原子移动），用户自建内容不动）；工具箱链路维持旧行为（重拆后新旧命名并存属已知取舍）。素材域 replace 落点在核心输出后二次改名为 `<NNN>-<主干>.pptx` 并进索引，见 §9。
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
| `cache`（后端空串归一；素材面板前端弹窗默认 `replace`） | `data/assets/<libId>/derived/<镜像源目录>/<源主干>/<源主干>-NNN.pptx` | 否（派生排除） | 保留 |
| `source` | `<源目录>/<源主干>/<源主干>-NNN.pptx` | 否（派生排除） | 保留 |
| `replace` | `<源目录>/<源主干>/<NNN>-<名字主干>.pptx` | 是（每页一条 `files` 行） | 全页成功后移入系统回收站 |

- 「名字主干」= `replaceArtifactBase(源主干)`：源主干为 ≥3 位纯数字时追加 `-p`（`123` → `123-p`），见 9.2。
- 落点参数归一单点 `normalizeSplitTarget`：空串兼容旧调用等价 `cache`（后端口径；素材面板前端弹窗默认传 `replace`）；非法值明确拒绝，禁静默落到意外落点。
- `cache` 根目录文件恰为 `derived/<源主干>/`，子目录文件多一层镜像目录（同库不同目录同名 pptx 不撞目录）。

### 9.2 可索引命名契约与隐式解耦

- **最终命名**：`replaceArtifactName(page, base)` = `fmt.Sprintf("%03d-%s.pptx", page, replaceArtifactBase(base))`；`base` = 源文件名去扩展名。
- **中间态**：核心仍按本文件 §3 输出 `<base>-NNN.pptx`，编排层逐页 `os.Rename` 为 `<NNN>-<stem>.pptx`，随后 `os.Stat` 校验（存在且 `Size > 0`）；页序取核心产物文件名尾段（`pageSeqFromArtifactName`），解析失败回退结果切片序号 + 1。
- **纯数字主干规避**：`replaceArtifactBase` 对「≥3 位纯数字」主干追加 `-p`（`123` → `123-p`）。原因是 `isPptxSplitDerivative` 判定「父目录名 + `-` + ≥2 位纯数字」，而 replace 产物父目录恰为 `base`、页序段 `%03d` 最小宽度 3 位（≥1000 页不补零）——`base=123` 第 123 页会产出 `123-123.pptx` 被扫描静默排除。1~2 位数字不可能与 ≥3 位页序段等长，无需后缀。
- **隐式耦合（改判定函数必读）**：`isPptxSplitDerivative` 的形态判定（父目录名 + `-` + **≥2 位**纯数字，R2 起）是 replace 可索引性的前提，三态区分完全依赖命名形态。后续若放宽为「任意父目录 + `-` + 数字」等更宽规则，会误伤 replace 产物使其退出索引。修改该判定函数须同步本节与回归用例。
- **用户侧边界**：把产物重命名为 `<源主干>-001.pptx` 会重新命中排除规则，下次扫描退出索引；就地替换弹窗的强确认区已显示该禁用命名提示（单文件回显实际主干，批量用 `<源名主干>` 占位）。

### 9.3 删源安全时序（`splitOneFileReplace`）

严格按序执行，任一步失败口径见下：

| 步 | 动作 | 失败处置 |
|---|---|---|
| 1 | **重算**源哈希（快照非空且与重算不一致即拒绝）+ 源 `Stat` 基线（size + mtime）+ 空主干/二次拆分校验 | 任一失败 → 单文件失败（不产生产物） |
| 2 | 读取既有记录（步骤 3 落点归属判定用）：replace **不做增量跳过**——源 `files` 行仍在（成功替换后已删）只对应「源从回收站还原」或「上次删源失败」两种须完整重拆的状态 | 重拆；旧产物保留到步骤 8b 前 |
| 3 | 旧产物登记（延迟清理）：目录确属本文件 `target=replace` 记录时收集同形文件名集合 `oldArtifacts`，**不删除**；记录缺失/落点不符而目录已有同形文件则拒绝——**例外（R1）**：记录非空且目录内 stray 文件经 `replaceOutDirOwnedByRecord` 认定（`ownershipFingerprintForName` 命中且 `fingerprintMatch` 内容全等，凭证可为本次指纹或 v11 retained）时视为本记录拥有该落点，照常快照并继续（其余用户同名文件仍由步骤 5 逐页拒绝）；`ownsOutDir` 与放宽分支均经 `firstUnownedReplaceCoreArtifactName` 另查核心中间态同名文件，只拦无归属凭证（或凭证内容不符）者（凭证覆盖的历史产物放行，见下条） | 拒绝即失败，目录内文件与源均不动 |
| 4 | 调拆分核心（产物落 `outDir`，仍为核心原生命名） | 取消/失败 → 回滚产物 |
| 5 | 逐页 rename 为可索引命名 + 逐页 `Stat`（存在且非空）；目标命中 `oldArtifacts` 时**先做归属校验**（`ownershipFingerprintForName`：先本次指纹再 v11 `retained_fingerprints`，按名唯一命中且 `size + SHA-256` 全等），通过才备份旧文件；归属存在但内容不符 / **无任何归属条目且指纹可用**均拒绝覆盖（文案区分见下条）；指纹与 retained 均不可用（v10 前旧记录 / 空串 / 坏 JSON / 条目数不符）时保持按名认定行为 | 回滚全部产物并还原备份 |
| 6 | 逐页 `processScanFile` 建索引，回读产物 `fileID`；产物路径命中库排除规则则拒绝 | 回滚产物与已写索引行 |
| 7 | `migrateSplitAnnotations` 单事务预迁移标注 | 回滚产物与索引行，源不删 |
| 8a | 删源前取消检查点 + **取消标志复查**（注册条目已被 `CancelAssetSplit` 摘除即命中）+ 源 `Stat` 复核（size/mtime 与基线一致） | 取消或源被修改 → 回滚产物与索引行并还原备份，源不删 |
| 8b | 历史旧页收尾（删 `oldArtifacts` 中不属于本次新页的文件与索引行；删除失败页归属写 v11 `retained_fingerprints`）→ `util.MoveToRecycleBin(源)`；移入失败补写 `target=replace, source_removed=0` 记录 | 删除旧页失败（重试一次后）仅告警并留存归属凭证（下次重拆消费）；移入失败不回滚，返回删源失败码（产物与源均保留） |
| 9 | `finalizeReplaceSource` 单事务：重迁移晚到标注 + 删源索引行 | 仅告警，下次扫描标 lost |
| 10 | `upsertSplitRecord(target=replace, source_removed=1)` | 仅告警（留痕职责） |
| 11 | 返回 `sourceRemoved=true` 成功结果 | — |

- **3-7 任一失败**：`rollbackReplaceArtifacts` 只删本次新增（核心调用前目录快照之外）的产物、索引行与缩略图；被本次覆盖的历史产物由 `restoreReplaceArtifactBackups` 从 `.replace-bak` 备份换回原位并尽力重解析刷新索引行。源文件与源行原样保留，明细 `rolledBack=true`。**旧产物不再在拆分前删除**——此前「先删后拆」在拆分失败时会连带丢掉上一次成果（审核修复）。
- **目标占用拒绝**：rename 前目标名已存在且不在 `oldArtifacts`（目录形态、并发新增等非历史产物）时中止并回滚，禁覆盖用户数据。**备份让位前归属校验（v10/v11）**：命中 `oldArtifacts` 只证明命名形态，归属凭证经 `ownershipFingerprintForName` 单点查询（先 `fingerprintForName` 本次指纹、未命中再查 v11 `retained_fingerprints`，均要求按名唯一命中，`sameNameFold` 仅 Windows 折叠），内容（size + 全文 SHA-256）全等才允许备份让位。失败文案按 R4 区分：**归属存在但内容不符** → 「第 N 页目标文件「X」内容与拆分记录不一致（可能已被修改），请确认后移走或还原」；**无任何归属条目**（用户同名文件）→ 「第 N 页目标文件「X」已存在且不属于历史拆分产物，请先移走或重命名后重试」。**与清理侧口径区分**：mtime 回落仅限 `cleanupSplitArtifactsInDirByPage` / `removeSplitArtifactsBeyond`；`cleanupStaleReplaceArtifacts` 与 retained 消费无 mtime 守卫；备份让位没有 mtime 语义，指纹与 retained 均不可用（v10 前旧记录 / 空串 / 坏 JSON / 条目数不符）时保持按名认定行为兼容。**TOCTOU 收口（R3）**：备份 rename 后对备份文件复验 `fingerprintMatch(bak, fp)`，不符则换回原路径并按「内容与拆分记录不一致」文案拒绝；**换回失败时 `backupReplaceArtifactForSurrender` 返回仍留在磁盘的备份路径**，文案为「第 N 页目标文件「X」内容与拆分记录不一致（可能已被修改），备份已保留在「<绝对路径>」，请人工确认后移走或还原」，备份**不登记进 `backups`**（`removeReplaceArtifactBackups` 不删，保留人工恢复入口）。**拒绝路径残留**：校验/复验换回成功时零 `.replace-bak` 残留；换回失败保留备份待人工恢复。
- **核心中间态同名文件守卫（R5/R3）**：`ownsOutDir` 为真或落点归属放宽生效时，调拆分核心**之前**检查 `outDir` 内是否存在 `<base>-<≥2 位纯数字>.pptx`（统一走 `firstUnownedReplaceCoreArtifactName`，整名折叠与 `isReplaceArtifactName` 同口径：精确优先、仅 Windows 折叠、含扩展名），**只拦「无归属凭证（或凭证内容不符）」的文件**——凭证（本次指纹或 v11 `retained_fingerprints`）按名命中且内容全等的历史产物不拦截，命中无凭证者即 `failed`（提示「目录中已存在与拆分核心产物同名的文件（%s），请先移走或重命名后重试」），核心零调用、目录零改动。**两分支统一口径（2026-11-03，R3）**：source→replace 同目录切换后旧 source 残留以 `<stem>-NNN.pptx` 形态留在 retained 时，占用解除后重试必须成功（消费/覆盖该文件），不再被无条件守卫永久拒绝；`firstReplaceCoreArtifactName` 已删除（单点收敛）。**已知限制**：核心直接写落点、无临时目录，凭证覆盖的中间态文件在拆分失败时可能被核心同名覆盖写且不可回滚。**位数口径（R1/R2）**：页序段要求至少 2 位纯数字（历史 `%02d`、当前 `%03d`）——核心与编排永不写 1 位页序，`方案-9.pptx` 不拦（用户文件），`方案-09.pptx` / `方案-009.pptx` 仍拦截。**同一 ≥2 位口径已单点化到全部页序解析点**（R2，`isArtifactPageSeq`）：`isReplaceArtifactName` 两形态、`isReplaceCoreArtifactName`、`isReplaceArtifactNameAnyStem`、`parseSplitArtifactName` 两分支、`pageSeqFromArtifactName`、`splitArtifactPageSeq`、`isPptxSplitDerivative`（派生排除）——1 位页序用户文件（`方案-9.pptx` / `9-方案.pptx`）在所有删除/定位/移动拦截/批量剔除路径均不被当作产物；2/3/4 位（历史 `%02d`、当前 `%03d`、≥1000 页）全部命中。核心同名覆盖写会丢失用户内容，故守卫早于任何写入。
- **备份残留**：`.replace-bak` 仅存在于「备份让位」到「删源前收尾/回滚还原」之间；成功路径删源前清理、回滚路径换回原位（**换回失败保留备份并 `Warn`，待人工恢复**——「零残留」仅承诺还原成功时）。进程崩溃留下的 `.replace-bak` 不匹配 `.pptx`/产物形态，不参与扫描索引、批量剔除与下次重拆清旧，属无害残留（不自动清扫，可人工删除）。
- **二次拆分拒绝**：批量源清单剔除位于 replace 记录 `output_dir` 下且匹配产物形态的文件（`filterReplaceArtifactSources`，Go 侧按记录集合比较，禁 SQL 拼路径）；单文件 `replace` 入口对同形产物直接拒绝，防「页的页」嵌套派生。
- **源哈希重算**：快照哈希与当前重算不一致 → 「源文件内容已变化，请先重新扫描素材库后再拆分」，不产生产物（陈旧哈希会误判跳过并写失真 `source_hash`）。空主干（文件名恰为 `.pptx`）会让落点退化为源目录，入口明确拒绝。
- **取消判定点**：核心返回取消错误、核心返回后、删源前 `ctx`、删源前注册表复查（`splitCancelledAfterCheckpoint`，覆盖 `CancelAssetSplit`「先摘条目再 cancel」的窗口）。取消同样回滚后返回：单文件入口 `mapAssetSplitCancelError` 映射 `E_ASSETS_CANCELLED`（复用码，不新增）；批量入口改走正常收尾（见下条）。**残余窗口**：复查之后紧接到达的取消仍可能把源移入回收站（可恢复删除兜底），该窗口无法原子消除，仅缩小并在此记明。
- **8b 失败**：产物、索引行、标注副本均保留，源与源行亦保留，返回 `E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`（warning）；同时补写 `target=replace, source_removed=0` 记录，使目录内同形文件保有产物归属（批量剔除与重试重拆都依赖该记录）。用户关闭占用程序后可重试，重拆覆盖产物。
- **批量入口失败分两类**：重命名/Stat/产物解析不可用等业务失败逐项填入 `Item{status:failed, code, rolledBack}` 继续后续文件；数据库写错误、标注事务错误属系统性错误，整体中止返回 error + `assets-split:error`。**取消不属系统性错误**：批量以已完成 `Items` + `Canceled=true` + nil error 收尾，终态发 `assets-split:done`，进行中的文件不追加 Item（前序已完成文件保持结果），详见 [assets-management.md §15.7](assets-management.md)。

### 9.4 回收站语义（`util.MoveToRecycleBin`）

- 公共前置校验：非空、绝对路径、`Lstat` 存在；失败即源文件原位不动。
- Windows：`SHFileOperationW` + `FOF_ALLOWUNDO | FOF_WANTNUKEWARNING | FOF_NOERRORUI`（`recycleFlags` 单点）。**禁 `FOF_NOCONFIRMATION` / `FOF_SILENT`**：回收站关闭、超配额或网络盘时 Shell 会退化为永久删除，静默标志会让该退化无提示且返回 0；`FOF_WANTNUKEWARNING` 使永久删除前弹警告，用户取消 → `fAnyOperationsAborted != 0` → 返回错误、源保留。Linux：freedesktop Trash（`$XDG_DATA_HOME/Trash`，`.trashinfo` 先写后移动，失败清理信息文件，跨设备复制路线回滚目标副本）。
- 语义 = 可恢复删除，不是永久删除；这是删源不可逆提示以外用户唯一的恢复手段。Windows 两场景（回收站关闭 / 超配额）须实机验证。

### 9.5 拆分记录（schema v10）与重拆

- `split_records` v8 增两列：`target TEXT NOT NULL DEFAULT 'cache'`、`source_removed INTEGER NOT NULL DEFAULT 0`；存量记录默认值语义正确（旧落点从不删源）。v8 迁移经 `migrateSchemaV8` 列存在性预判 + 单事务，半迁移库可重开。
- **v9 增 `artifact_stem TEXT NOT NULL DEFAULT ''`**（2026-11-02，清理误删/漏清总收口）：持久化本次拆分**实际产物命名主干**——source/cache 落点为 `splitSourceBaseName(rel)`，replace 落点为 `replaceArtifactBase(base)`（纯数字主干加 `-p`）。该列是旧产物清理锚定与重链探测归属校验的**唯一权威事实**，取代「从目录名/文件名反推」的启发式；`rewriteSplitRecordKeyTx` 只改源键与 `output_dir`，不触碰主干（主干与路径无关）。迁移经 `migrateSchemaV9` 列存在性预判 + 单事务，半迁移库可重开；旧记录 `artifact_stem=''` 读取侧回落目录启发式保持兼容。
- **v10 增 `artifact_fingerprints TEXT NOT NULL DEFAULT ''`**（2026-11-02，任务 `11-02-artifact-page-fingerprint`，清理误删总收口）：JSON 数组按页序升序，每项 `{"page":1,"name":"讲义-001.pptx","size":7340032,"sha256":"<64位小写hex>"}`（结构 `splitArtifactFingerprint`）。`name` 只存 basename（目录由 `output_dir` 提供），重链/目录迁移不重写（产物文件名不变），备份还原随整库走。**写入时机**：source/cache 在核心成功且旧产物清理完成后、与记录同一条 upsert（`splitOneFileWithHash` 步骤 6）；replace 在逐页 rename+Stat 通过后计算，删源失败补写与成功记录共用同一份（`splitOneFileReplace` 步骤 5/8b/10）；任一页 `hashFileSHA256` 失败降级为空指纹 + `Warn`，不回滚已成功拆分。**清理侧三处统一**：`fingerprintsUsable`（非空且条目数 = `page_count`）为真时指纹即 allow-list——`cleanupSplitArtifactsInDirByPage` 逐页按 `dir/<name>` 校验 `size + 全文 SHA-256` 全等才删（`name` 纯 basename 校验防路径穿越，非常规文件跳过，`excludeStem` 仍保护 M1 新产物）；`removeSplitArtifactsBeyond` 只遍历指纹中 `page > keepPage` 或非当前形态的条目；`cleanupStaleReplaceArtifacts` 只删 `oldArtifacts` 中能在**本次指纹**按名命中（`fingerprintForName`，`sameNameFold`，多命中不可判定）且内容全等的文件（**不查 retained、无 mtime 守卫**）；指纹不可用（空串/坏 JSON/条目数不符）时回落**按名删除**（v10 前兼容，不看 mtime）；指纹可用但不在清单中的候选保留（用户同形文件，任意 mtime），若其同时是 retained 条目则由下一单点消费。retained 的消费由 `cleanupRetainedSplitArtifacts` 独立单点负责，避免同一文件两处重复尝试与重复登记。**replace 备份让位（v10/v11 第四处消费）**：`splitOneFileReplace` 步骤 5 命中 `oldArtifacts` 后、备份 rename 前经 `ownershipFingerprintForName`（先 `fingerprintForName` 本次指纹、未命中再查 v11 `retained_fingerprints`，均要求按名唯一命中，`sameNameFold`）且 `fingerprintMatch`（size + 全文 SHA-256）全等才备份覆盖；备份 rename 后**再复验** `fingerprintMatch(bak, fp)`（R3 TOCTOU），不符换回原路径拒绝。失败文案区分：归属存在但内容不符 →「内容与拆分记录不一致（可能已被修改）」；无任何归属条目 →「已存在且不属于历史拆分产物」。**口径区分**：`cleanupSplitArtifactsInDirByPage` / `removeSplitArtifactsBeyond` 指纹缺失回落 mtime；`cleanupStaleReplaceArtifacts` 与 retained 消费无 mtime 守卫；备份让位没有 mtime 语义，凭证缺失（空串/坏 JSON/条目数不符/v10 前记录）按名认定。**指纹非空但某页无条目 = 该页不是本记录产物，保留且不回落**。**禁止自动回填**：现有目录可能已被用户替换，回填即把用户文件认证为产物。迁移经 `migrateSchemaV10` 列存在性预判 + 单事务，半迁移库可重开；`verifyBackupDatabase` 不要求新列，v9 备份照常放行。
- **v11 增 `retained_fingerprints TEXT NOT NULL DEFAULT ''`**（2026-11-02，任务 `11-02-replace-fingerprint-followup`；跨 target 收敛见 `11-03-replace-retained-cross-target`）：语义 = **清理失败未能删除、但仍属本记录历史产物的凭证**，元素结构与 `artifact_fingerprints` 完全一致（`{page,name,size,sha256}`）。独立成列的动机：`artifact_fingerprints` 契约恒为「条目数 = `page_count`」，把清理失败页混入会破坏 `fingerprintsUsable` 的损坏判定。**写入时机**：仅 `cleanupStaleReplaceArtifacts` 同目录旧页删除失败时，把该页已有归属条目（本次指纹）收进返回列表，调用方写入本次 upsert（删源失败补写与成功记录两处一致）；成功删除或文件已不存在不保留。**消费（R1 独立单点）**：`cleanupRetainedSplitArtifacts(store, lib, record, dir, excludeStem)` 只处理 `record.retained`，`dir` 须与 `record.outputDir` 折叠相等（否则原样返回，跨目录不消费；`dir` 恒取 `record.outputDir` 旧落点，与新 target/新落点无关）——名字合法、未命中 `excludeStem`、`fingerprintMatch` 内容全等才删；删除成功或文件不存在 → 条目丢弃（自愈），内容不符 → 条目丢弃，删除失败**同目录**时条目保留随新记录携带（不静默清空）、**异目录**时条目丢弃并逐条 Warn（见下条 R1/P1-1）。调用点：source/cache 在核心成功、旧页清理之后；replace 在 `cleanupStaleReplaceArtifacts` 之后。两条链路消费前都先剔除与**本次新产物同名**的条目（`recordWithoutRetainedNames` 按 `foldNameInSet` 过滤：字节确定性下旧凭证内容可能与新页全等，直接消费会误删刚生成的产物）——**该过滤仅在本次落点（source/cache 取 `result.OutputDir`、replace 取 `outDir`）与 `record.outputDir` 同目录时生效**；异目录（如 replace→cache，新落点 B、凭证在 A）basename 相同不代表同一物理文件，按名过滤会误丢 A 真实残留（R2），故全量条目交消费单点。**跨落点失败条目丢弃（R1，P1-1）**：`cleanupRetainedSplitArtifacts` 返回的「仍删不掉」条目相对 `record.outputDir` 解析，若本次落点与 `record.outputDir` 不同（`!samePathFold`），写入新记录会错锚到新落点、旧残留反而失去凭证（永久孤儿）——两条调用链在异目录时把这些失败条目**一律丢弃并逐条 `Warn`**（含 `dir`/`name`，文案「跨落点切换：旧落点残留页删除失败，归属凭证不再随记录携带（需人工清理）」），同目录失败条目继续续携；**不变式：写入记录的 retained 条目恒锚定写入后的 `output_dir`**。replace 再与 stale 失败条目按 name 去重合并（`mergeSplitFingerprintsByName`，`sameNameFold`）后写入两处 upsert——异目录条目已在合并前丢弃，故按 name 去重在同目录场景正确，跨目录同名不会互相吞并（R3）。**存续范围**：同一 `output_dir` 的任一次成功拆分（任意 target）都会尝试消费；切 target、改名重链、目录迁移不丢失（随记录行迁移）。备份让位与落点归属放宽（`replaceOutDirOwnedByRecord`）仍经 `ownershipFingerprintForName`（先指纹后 retained，均唯一命中）查询归属。**禁止自动回填、禁止从目录内容反推**。迁移经 `migrateSchemaV11` 列存在性预判 + 单事务，半迁移库可重开；存量行默认空串。
- **容器目录迁移与单页产物移动约束**（2026-11-01）：replace 产物容器目录（`output_dir`）随所在目录重命名/移动/前缀替换在同一事务迁移（`migrateSplitRecordsTx` 对 `target=source/replace` 且库根内相对路径命中旧目录前缀的记录改写 `output_dir`；源键同时命中时按「库根 + 新源目录 + 新源主干」重算，cache 保留原值，详见 [assets-management.md §8.7](assets-management.md)）。`MoveAssetFiles` 对 replace 单页产物（`isReplaceArtifactOutput` 命中）拒绝单独移动，提示整体移动其所在产物容器目录——产物位置只能由记录的 `output_dir` 表达，单独移出会使批量剔除与溯源同时失据。
- **文件级重链后的产物定位与落点判定**（2026-11-01）：`RelocateAssetFile` 在 files.path 同事务迁移 `split_records` 源键并**默认保持** `output_dir` 原绝对路径（重链只改索引指针，产物目录物理未动）；**例外**——产物容器目录随源一起搬迁时（`resolveRelocatedSplitOutDir` 判定候选 `<新源目录>/<新主干>` 存在、首页产物形态命中**且其解析主干属于记录主干**（`artifact_stem`，空值回落记录 `output_dir` 基名与 replace 数字主干变体；主干不符不采纳，防止把同目录其他同名源的产物容器改指过来、后续清理误删他人产物），仅 `target=source/replace`）改指新落点，仍与 `files.path` 同事务（cache 与「仅移动源」保持原值）；产物命名主干仍是拆分时点的旧源名，故打开单页 / 导出 `single_pptx_path` / 增量判定首页存在性统一走 `splitArtifactPath`（当前主干直拼，失败则 `output_dir` 内按页序反查唯一候选，多候选不猜）；`isReplaceArtifactOutput` 在目录独占前提下补宽松形态 `<页序>-<任意主干>.pptx`，防改名后旧产物脱离剔除。目录级迁移重算 `output_dir` 前经 `splitOutDirUnderPrefix` 确认现落点位于被迁移前缀下（重链冻结在别处的不重算）。force 重链（哈希不等且确认，内容已变）与 `clearLostFiles` 均在同一事务删除对应拆分记录。
- **落点目录归属校验（P0 数据正确性，2026-11-02）**：source/cache 落点由当前文件名直接推导，可能已被**另一源文件**的拆分记录占用。可达损坏场景：`A.pptx` 拆分（记录 `output_dir=<root>/A`、主干 `A`）后改名为 `B.pptx` 并重链（记录键迁到 B，`output_dir`/主干冻结为 A），随后新建 `A.pptx` 再拆分——新 A 落点正是同一 A 目录，旧实现会直接 `removeSplitArtifacts`/`RemoveAll` 删掉并覆盖 B 的产物。`splitOneFileWithHash` 在算出 `outDir` 后、**任何删除动作之前**调用 `store.splitRecordByOutputDir(outDir, rel)` 单点（`SELECT ... WHERE source_rel_path <> ?` 后 Go 侧 `samePathFold`，SQL 无法跨平台折叠大小写）；命中返回失败结果 + `code=E_ASSETS_SPLIT_OUTPUT_CONFLICT`（warning，文案含占用源相对路径），不删除/覆盖任何文件，同源记录由查询排除故正常重拆与改名重链后重拆不受影响。**同时** `splitArtifactPath` 的页序反查命中后须 `parseSplitArtifactName` 解析主干并校验属于 `splitArtifactStemCandidates(record, record.outputDir)`（精确或 `EqualFold`），不符则不采纳（返回直拼路径 + `false`）——否则历史冲突数据下增量判定会把他人首页产物误判为本记录产物而跳过拆分，打开单页也会指向错误文件。不引入 `output_dir` UNIQUE 约束（历史数据可能冲突，迁移风险高）。**同源键折叠排除**：SQL `<>` 大小写敏感，Go 侧再按 `samePathFold` 折叠 `source_rel_path`（Windows 上 `A.pptx`/`a.pptx` 是同一物理文件，不折叠会把同源记录误判为「另一源占用」而拒绝正常重拆）。**replace 落点同样接入该检查（2026-11-02 加固，原「不接入」结论不完整）**：`splitOneFileReplace` 在算出 `outDir` 后、**任何写入/覆盖动作之前**调用同一 `store.splitRecordByOutputDir(outDir, rel)`；命中其他记录返回同一失败结果 + `code=E_ASSETS_SPLIT_OUTPUT_CONFLICT`，目录零改动。原判定只有 `ownsOutDir`（本记录 target=replace 且落点相同）与 `firstReplaceArtifactName`（目录内已有同形文件）两条启发式，二者都不成立即放行——「旧记录指向该目录但产物已缺失」（改名重链后旧目录产物被清理/移走）恰落入空白：同目录新同名源的就地替换产物会被旧记录认领，旧记录再次 replace 时按旧 `pageCount` 删除新产物与索引行。归属以记录集为权威后该形态被拒；同源记录由查询排除，`ownsOutDir` 分支语义不变（本记录拥有目录且无其他记录同时占用时正常重拆放行；两条记录同指一目录属历史冲突数据，按拒绝处理）。**导出 `single_pptx_path` 的页序反查同样须携带 `artifact_stem`**（`assetExportRow.splitArtifactStem`）：产物目录随源改名搬迁后目录基名与记录主干不一致，缺该字段会出现「打开单页可用、导出恒空」的两面分叉。
- **增量跳过判定纳入落点 target 一致性（2026-11-02）**：`splitOneFileWithHash` 的跳过条件 = `record.sourceHash == hash && record.target == target && 首页产物存在`（三验）。落点切换（source↔cache、replace→source 等）即使源未变也须真正换落点重拆——否则旧落点产物与索引行永不收敛，且跳过结果回显的 `Target` 与实际落点不符；跳过结果一律回显记录字段（`Target` 取 `record.target`）。**只比 target 不比 `output_dir`**：改名重链后记录落点冻结旧目录、本次实际落点为源新目录，但 target 未变且产物经 `splitArtifactPath`（直拼 + 页序反查）仍可定位，故仍判「已拆未变」跳过（11-01 契约：改名重链不触发全量重拆）；仅用户显式切换落点才重拆。
- replace **不做增量跳过**（见 9.3 步 2）：源行存在只对应「源从回收站还原」（`source_removed=1`）或「上次删源失败、产物完整」（`source_removed=0`）两种状态，均须完整重拆——跳过会留下源与产物并存或产物失去归属。
- 重拆清旧改为**延迟清理**：拆分前只登记 `oldArtifacts` 不删除；全部页 rename+Stat+索引+标注迁移通过、删源前才删除本次新页之外的历史页，并联动 `deleteIndexRowsForPaths` 删索引行与缩略图——防页数减少残留僵尸素材，同时保证失败/取消时旧产物（含被覆盖页）原样保留。
- **source/cache 落点延迟清理（2026-11-02）**：`splitOneFileWithHash` 核心调用前**不删除任何旧页**（此前 source 前置 `removeSplitArtifacts`、cache 前置 `RemoveAll` 整目录）；核心成功拿到 `result.PageCount` 后统一调 `removeSplitArtifactsBeyond(store, lib, dir, baseName, PageCount, splitAt)`（`splitAt` 取**上一次**记录的分拆时间，`record == nil` 传空串）——删除条件二选一：页序 > PageCount；或页序 ≤ PageCount 但文件名不是当前形态 `fmt.Sprintf("%s-%03d.pptx", baseName, seq)`（历史 `%02d` 产物 `演变-01.pptx` 与新 `演变-001.pptx` 并存时前者收敛）。主干段经 `sameNameFold` 比较（Windows 仅改大小写改名后 `Abc-003.pptx` 仍识别，非 Windows 大小写敏感不识别）；删除前经 mtime 守卫（晚于 `splitAt` + 2s 视为用户后放入的同名文件保留）；成功删除的路径批量调 `deleteIndexRowsForPaths`（库外 derived 自动静默跳过）。目录内用户自建内容与同名异构文件（`<主干>-草稿.pptx`）一律不动。核心失败/取消时上一次拆分成果不再被前置删除（核心已同名覆盖写的页属已知限制，彻底解决需核心写临时目录再原子移动，见任务 Out of Scope）；删除失败先经 `removeSplitArtifactFile` 重试一次，仍失败仅告警，文案不承诺自动收敛。`removeSplitArtifacts(dir, baseName)` 已并入 `keepPage=0` 的 `removeSplitArtifactsBeyond`（无独立调用点）。
- **replace 产物名大小写折叠（2026-11-02，Windows 语义限定）**：`isReplaceArtifactName` 核心中间态与已改名形态判定**精确匹配优先，仅 Windows 下追加一次大小写折叠**（经 `sameNameFold`/`ToLower` 双侧归一后复用精确逻辑，页序段仍按原始串切片）。Windows NTFS 保留大小写但不敏感，源文件仅改大小写（`abc.pptx` → `Abc.pptx`）后重拆，上次产物 `001-abc.pptx` 与本次目标 `001-Abc.pptx` 是同一物理文件——按敏感比较会判为「非历史产物」并因 `fileExists` 为真永久回滚失败。**Linux/macOS 大小写敏感，`001-ABC.pptx` 与 `001-abc.pptx` 是不同文件，折叠会把用户文件收入历史产物、成功后删备份造成内容丢失（P1）**，故平台守卫不可移除。`snapshotReplaceArtifactNames` / `firstReplaceArtifactName` 自动受益；下游消费同须折叠：rename 循环经 `foldNameInSet` 取磁盘实际名搬迁备份（Windows 不折叠会导致按新名定位同一物理文件失败），`cleanupStaleReplaceArtifacts` 的 newNames 比对同走 `foldNameInSet`（精确比对会把刚生成的新页当陈旧页删除）。**落点目录大小写归一**：仅 Windows 下核心成功后 `resolveExistingDirCase` 按 parent 目录实际条目名归一 `outDir`（既有产物目录仍是旧大小写时，按新名推导会让索引相对路径与磁盘 `WalkDir` 实际名不一致，下一轮扫描把新行标 lost 并另插一行——同一物理文件双行且标注挂在幽灵行）；多命中（大小写敏感目录标志开启）或读取失败按原值；非 Windows 直接原样返回、不读父目录。`cleanupStaleReplaceArtifacts` 对仅大小写差异的旧产物额外删除其旧大小写索引行（只删行不删文件，Windows 上旧路径仍解析到新页）。**单一折叠点**：文件名/目录名等价判定统一走 `sameNameFold(a, b)`（精确相等 或 `runtime.GOOS == "windows" && strings.EqualFold`），绝对路径走 `samePathFold`（Clean 后委托 `sameNameFold`）；禁在文件名/路径判定处裸用 `strings.EqualFold`（`splitStemInCandidates` 的产物主干归属折叠为本任务刻意豁免项，见 §9.5 旧产物清理条）。`isReplaceArtifactName` 与 `isReplaceCoreArtifactName` 的**整名折叠**（含扩展名）是同一机制的同款实现：精确逻辑优先，仅 Windows 下双侧 `ToLower` 后复用精确逻辑（页序/主干切片按折叠串长度，不中途混用原始串）；**禁**为省事改用裸 `strings.EqualFold` 比较整名（会绕过页序段纯数字校验）。两形态页序段均要求 **≥2 位**纯数字（与 `isReplaceCoreArtifactName` 同口径，R1）：1 位页序用户文件（`方案-9.pptx` / `9-方案.pptx`）在任何平台都不进入历史产物集合。
- **旧产物目录清理（记录主干锚定，2026-11-02 收口）**：重链 / 跨落点切换 / 同目录改主干重拆均可能留下「记录键已变、旧产物目录与产物主干仍是旧值」的残留。source / replace 两条链路在**拆分成功后**（源落点核心返回成功后、replace 删源前）调用 `cleanupSplitArtifactsInDirByPage(store, lib, record, dir, excludeStem)` 单点清理。判定面（v10 指纹可用时改走指纹 allow-list，见上条） = **`record.artifactStem` 锚定**：记录主干非空 → 精确匹配该主干，无精确命中才接受 `sameNameFold`（仅 Windows 折叠）唯一命中，折叠多命中不可判定；主干为空（v9 前旧记录）→ 候选集合 `{filepath.Base(dir), replaceArtifactBase(基名)}`（去重）逐个精确、再逐个折叠（仅 Windows；候选集合已精确覆盖 replace 数字主干 `123` ↔ 产物 `123-p` 的合法不一致）。`dominantSplitArtifactStem` 按候选集合选组，页数覆盖不参与选组（部分缺失如只剩页 2 同样清理），单页记录也没有「任意唯一同形文件即产物」豁免。`collectSplitArtifactsByPage` 单次读目录，按 `parseSplitArtifactName`（与 `findSplitArtifactByPage` 同源形态：replace `<页序>-<主干>`、其余 `<主干>-<页序>`，**页序 ≥2 位**）收集 `1..record.pageCount` 候选，只取 `e.Type().IsRegular()` 常规文件（符号链接/junction/子目录跳过）。删除时只删该主干组内同页序唯一的常规文件，异主干候选一律视为用户文件；`excludeStem` 与候选主干**精确相等**才跳过（同目录重拆 M1 保护本次新产物；仅 Windows 下大小写差异视为同一主干，由门控挡在调用之外——折叠比较会让旧页整组漏清）。同主干同页序多候选（`<主干>-001.pptx` 与 `<主干>-0001.pptx`）不可判定：该页保留并 `Logger().Warn`（含 `dir`/`page`/`stem`），不再静默；1 位页序（`<主干>-1.pptx`）自 R2 起不参与候选解析，属用户文件。安全侧：折叠多命中，或回落启发式（`artifact_stem` 为空）时目录基名与所有组主干均不匹配 → 整目录零删除 + `Warn`（`libraryId`/`dir`/`artifactStem`/`pageCount`/`stemGroups`）；**平台分流后果（Linux/macOS）**：`dominantSplitArtifactStem` 折叠禁用后，记录主干仅与目录组大小写不同时不再被选为产物组（整目录零删除，不误删用户组）；该变体组仍存在时 `splitStemGroupPresent` 经 `splitStemInCandidates` 的刻意折叠判为存在，走上面 `Warn` 分支而非静默——即「原本可清理」的形态在非 Windows 退化为「保留 + 告警」（失效方向为漏清不误删，安全侧合理）；记录主干非空但目录内已无该主干组（上次重拆已清理 / M1 同目录重拆只剩本次新产物）属正常路径静默；目录不存在或无任何形态候选同样静默；其他读错误 `Warn` 后跳过。目录删空后 `os.Remove`，非空/不存在（`ENOENT`/`ENOTEMPTY`，Windows `ERROR_DIR_NOT_EMPTY`）静默保留。删后联动 `deleteIndexRowsForPaths` 清索引行与缩略图。**库外旧落点（derived，库根之外）不做索引行清理**——其产物本就不入索引；库内判定单点 `relPathWithinRoot`，同卷时 `Rel` 返回 `..` 前缀、跨卷时 `Rel` 直接出错，两者均视为库外静默跳过（Debug 留痕），不逐文件告警（11-02-derived-cross-volume-warn-noise）。**mtime 守卫**：删除候选前 `Stat`，`ModTime` 晚于 `record.splitAt` + `splitArtifactUserFileGrace`（2 秒）视为用户后放入的同名文件保留（不告警，Debug 留痕）；`splitAt` 解析失败守卫不生效（保持旧行为），`Stat` 失败按不可判定保留。产物 mtime 恒早于记录写入时间，失效方向为「漏清」而非「误删」。**v10 前旧记录（指纹为空）在用户显式重拆前仍可能误删**同主干同页序且 mtime 早于 split_at 的用户文件——无基线哈希只能回落 mtime；v10 起成功拆分写入指纹后由 allow-list 拦截（任意 mtime 均保留）。`findSplitArtifactByPage` 保持「单页唯一定位」语义（打开/导出/增量判定，只定位不删除）。**删除失败先重试一次**（`removeSplitArtifactFile`，50ms 间隔），仍失败 `Warn` 明确「残留需人工清理，本次重拆不再自动收敛」——本路径（`cleanupSplitArtifactsInDirByPage`，source/cache 旧落点与 M1/M4）不产出 retained，记录主干/落点此时已被新值覆盖，残留页不再被任何记录引用，不得再声称「下次重拆收敛」；**replace 同目录普通重拆的旧页清理走 `cleanupStaleReplaceArtifacts`，删除失败页归属写入 v11 `retained_fingerprints`（见上条）**，下次重拆可识别并消费。
- **四种清理形态与门控（M1/M4，2026-11-02 补齐跨 target）**：决策单点 `splitArtifactCleanupPlan(record, currentTarget, currentStem, currentOutDir)`（source 与 replace 两链路共用，禁各写一套）；`record == nil` 不清理，其余四分支——① `record.outputDir` 与本次落点不同（**不按 target 分叉**，cache↔source↔replace 双向收敛旧落点产物与索引行，M4；改名重链后记录落点冻结旧目录、本次实际落点为源新目录同属此支）；② 同目录但 `record.target != 本次落点`：旧产物形态由 `record.target` 解析（replace `<页序>-<主干>` vs source/cache `<主干>-<页序>`），新产物形态不会被收集，`excludeStem` 传空；③ 同目录同 target 但 `record.artifactStem != "" && record.artifactStem != 本次新主干 && !sameNameFold(record.artifactStem, 本次新主干)`：清理旧主干并以 `excludeStem=本次新主干` 保护新产物（M1）；**仅 Windows 下**大小写差异折叠相等视为同一主干不进入（本次产物覆盖同名物理文件，旧页由既有链路处理）；Linux/macOS 大小写敏感按不同主干清理旧组；④ 同目录同 target 同主干（普通重拆）：不调用，旧页由 `removeSplitArtifactsBeyond`（source/cache）与 `cleanupStaleReplaceArtifacts`（replace）既有链路处理。失败/取消路径不触达，上一次成果原样保留。
- `OpenAssetSplitPage` 按 `record.target` 选命名：replace 用 `<NNN>-<名字主干>.pptx`，其余用 `<主干>-<NNN>.pptx`。
- `GetAssetSplitRecord` 回显 `target`/`sourceRemoved` 供展示与诊断；前端当前仅用 `exists` 决定「打开拆分单页」入口显隐，不据此禁用重拆（源还原后允许重拆）。

### 9.6 与扫描的互斥（锁序 scan → split）

replace 要为产物写 `files/slides` 索引（复用 `processScanFile` 单文件重链），与在途扫描并发写会互相覆盖。故 replace 先 `tryLockScan`（失败 `E_ASSETS_SCAN_IN_PROGRESS`）再 `tryLockSplit`，锁序与 `DeleteLibrary` 一致；`cache/source` 不写索引，仅取拆分锁，行为与改动前一致。取消注册表 `splitCancels` 与扫描 `cancels` 键空间独立，互不误杀。

### 9.7 补充测试要求

- 三落点回归：`cache/source` 与改动前逐字一致；`replace` 全链成功（源消失、每页一条索引、页数均为 1）。
- 失败/取消：产物、索引行、缩略图零残留，源保留，`rolledBack=true`。
- 删源失败：`E_ASSETS_SPLIT_SOURCE_DELETE_FAILED`，产物与源均保留。
- 落点归属冲突：A 拆分 → 改名重链为 B → 新建 A 再拆分返回 `E_ASSETS_SPLIT_OUTPUT_CONFLICT`，B 的产物字节与 `files` 行零改动；同源重复拆分放行；`splitArtifactPath` 反查候选主干须属于 `splitArtifactStemCandidates`（记录 `artifact_stem`，空值回落目录启发式）才采纳；`splitRecordByOutputDir` 单点四态（空表/他源命中/同源排除/落点不同）直测；导出页序反查携带记录主干（目录基名与主干不一致场景）。
- 命名边界：纯数字主干 `123` / `1234` 加 `-p`；`isPptxSplitDerivative` 对 replace 产物返回 false。
- 标注迁移：文件级与页级两组断言；回收站实现双平台分别单测。
- 安全回归：源拆分期间被修改 → 回滚；源还原后重拆不跳过；排除规则命中 → 拒绝；v8 半迁移库可重开；回收站标志不含 `FOF_NOCONFIRMATION`/`FOF_SILENT`。
- 落点切换（2026-11-02）：source→cache / cache→source 源未变不得 `Skipped`（旧落点产物与索引行清理、记录 target/落点/主干更新）；同目录 replace→source 旧 `<页序>-<主干>` 与索引行清理、新 `<主干>-<页序>` 保留；改名重链（target 未变）仍 `Skipped` 且旧产物保留在旧目录可打开（跳过判定不比 `output_dir`）。
- mtime 守卫（v10 前旧记录回落路径）：产物（mtime 早于 split_at）删除；用户后放入的同主干同页序文件（mtime 晚于 split_at + 2s）保留；`Stat` 失败保留；split_at 解析失败守卫不生效。
- v10 页级指纹（2026-11-02）：指纹命中删除（索引行联动）；指纹不符（同主干同页序、内容不同、mtime 早于 split_at）保留；**指纹不可用（空/坏 JSON/条目数不符）时口径分两档**——`cleanupSplitArtifactsInDirByPage` / `removeSplitArtifactsBeyond` 回落 **mtime 守卫**，`cleanupStaleReplaceArtifacts` 回落**按名删除、无 mtime 守卫**（v10 前兼容，禁写成 mtime 回落）；指纹非空但缺页保留；三处清理路径各自直测；重链/目录迁移/VACUUM 备份还原后指纹仍命中；路径穿越 name 拒绝且不删；Windows 折叠与非 Windows 精确分流；同一 fixture 连续拆两次逐页 SHA-256 相等（字节确定性）；v9→v10 迁移、半迁移重开、存量行禁自动回填。
- replace 备份让位指纹校验（2026-11-02，followup R1/R3/R4/R5/R7）：拒绝类用例的用户内容与旧产物**等长不同哈希**（9 字节），把 `fingerprintMatch` 的 SHA-256 比较改为恒真须使拒绝类用例变红（变异验证）；旧产物字节未变 → 让位成功且删源前备份清理；凭证缺失（空/条目数不符/坏 JSON）按名覆盖；归属存在但内容不符 → 「内容与拆分记录不一致」文案，无归属条目 / 按名折叠多命中 → 「不属于历史拆分产物」文案；备份 rename 后复验不符 → 换回原路径、拒绝、无残留；核心中间态 `<base>-<≥2 位纯数字>.pptx` 存在 → 核心零调用、文件字节不变、返回 failed（1 位页序 `方案-9.pptx` 不拦；owns 分支与放宽分支同口径，凭证覆盖的中间态文件放行）；失败回滚后断言还原页 `files.size/hash` 与旧值一致（索引行自愈）；平台大小写断言拆 `_windows_test.go` / `_other_test.go`（共享 fixture 留主文件）——**Windows 折叠放行分支仅 Windows 可执行，由 CI `windows-latest` job 覆盖**。
- retained 归属（v11，2026-11-02）：记录读写往返携带 retained（`getSplitRecord` / `splitRecordByOutputDir`）、覆盖为空即清空；v10→v11 迁移补列且存量默认空串、半迁移重开；`ownershipFingerprintForName` 先指纹后 retained、两清单均要求唯一命中；旧页清理删除失败（`removeArtifactFn` 注入）→ 归属写入 retained，随后页数恢复重拆成功且新记录 retained 清空（自愈）。
- retained 跨 target 收敛（R1，2026-11-03）：3 页 replace → 删页失败 + 删源失败（保留源行）→ 切 `target=source`（同落点）→ 残留页被删、索引行清理、retained 清空 → 再切回 `replace` 成功（`replaceOutDirOwnedByRecord` 放行旧形态产物，不被按名拒绝）；切 target 后再次删除失败 → 新记录仍携 retained，切回 replace 正常重拆并最终消费；retained 随备份快照、改名重链、目录迁移存续（`rewriteSplitRecordKeyTx` / `migrateSplitRecordsTx` / `vacuumInto` 后读回不变）；`cleanupRetainedSplitArtifacts` 四态直测（目录不符原样返回 / 内容全等删除并清索引行 / 内容不符与非法名丢弃 / 删除失败保留）；**新页保护（仅同目录）**：两条链路消费前在本次落点与 `record.outputDir` 同目录时剔除本次新产物同名条目（异目录不过滤，R2；source/cache 用例：旧凭证描述 `<主干>-NNN.pptx` 且新页字节全等时不得删除新页）；**放宽分支守卫**：凭证放行后仍拦无凭证核心同名文件（用户文件零改动、核心零调用）；**变异验证**：把 source/cache 链路的 retained 消费（或新页同名过滤）去掉，新增用例须变红。
- 备份让位文案两态（R3，2026-11-03）：`replaceArtifactMismatchMessage` 换回成功不含备份路径、备份保留含绝对路径与「请人工确认后移走或还原」；`backupReplaceArtifactForSurrender` 换回成功返回空 keptBackup、换回失败返回备份路径。
- 页序位数与归属返回（R1/R2/R3，2026-11-03）：**全部页序解析点统一 ≥2 位**（`isReplaceArtifactName`/`isReplaceCoreArtifactName`/`isReplaceArtifactNameAnyStem`/`parseSplitArtifactName`/`pageSeqFromArtifactName`/`splitArtifactPageSeq`/`isPptxSplitDerivative`）表驱动（1 位 false、2/3/4 位 true；最终产物名与核心中间态互不误判）；1 位页序用户文件（`方案-9.pptx` / `9-方案.pptx`）在指纹不可用旧记录的清理、定位、移动拦截、批量剔除路径均保留且索引行零改动（变异：恢复任意位数判定 → 用例变红）；cache 异目录切换 consumed retained（replace 记录 A 带 source 形态 retained，切 cache 落点 B 且新页同名 → A 残留被消费、B 新页保留；变异：去掉同目录条件 → 用例变红）；owns 分支 + retained 核心形态文件占用解除后重试成功（变异：守卫改回不认凭证 → 用例变红）。
- 异目录 retained 丢弃（P1-1，2026-11-03）：source/cache 与 replace 两链路在异目录消费失败时，新记录 `retained_fingerprints` 必须为空（不得携带指向旧 `output_dir` 的错锚条目）、旧残留物理保留且日志含跨落点告警（变异：恢复无条件续携 → 两条用例变红）；同目录失败条目继续续携（既有用例）。
- 落点归属内容不符两分支（P2-2，2026-11-03）：`ownsOutDir` 分支核心同名文件「凭证存在但内容不符」（等长异哈希 9 字节）→ 返回 failed、提示核心同名文件名、核心零调用；落点归属放宽分支目录内全部同形文件「凭证内容不符」→ `replaceOutDirOwnedByRecord` 返回 false、按 stray 拒绝、文件零改动（变异：`fingerprintMatch` 恒真 → 两条用例变红）。
- cache→replace 跨 target（P2-2，2026-11-03）：cache 记录切 replace 正常重拆、旧 derived 落点产物按 `splitArtifactCleanupPlan` 清理、新 replace 产物进索引、记录不携带 retained。
- 旧落点指纹清理删除失败（P2-4，2026-11-03）：`cleanupSplitArtifactsByFingerprint` 仅 `Warn`（残留需人工清理、不随记录携带）——该路径清理旧落点/旧主干，记录落点与主干即将被新值覆盖，登记 retained 会错锚，故刻意不登记。
- 写入值守护：三种落点成功用例断言 `getSplitRecord().artifactStem`（source/cache = 源主干；replace = `replaceArtifactBase`，`123` → `123-p`），删源失败补写分支同样断言；变异为空或缺 `-p` 须变红。
- 清理决策单点：`splitArtifactCleanupPlan` 四分支直测（异目录 / 同目录跨 target / 同目录改主干 exclude / 仅大小写差异按平台分流——Windows 不清理、非 Windows 清理旧组）。
- 延迟清理（2026-11-02）：source/cache 核心失败时旧产物完整保留（cache 前置不再 `RemoveAll`）；页数减少删超界旧页、页数增加保留全部；`removeSplitArtifactsBeyond` 不误伤同名异构与用户文件。
- 旧页收敛细节（2026-11-02 收口）：页序 ≤ keepPage 的历史 `%02d` 产物（`演变-01.pptx`）与当前 `%03d` 产物并存时前者删除、后者保留（fixture 必须含页序 ≤ keepPage 的两位用例，仅放 42/99 越界页会形成假阳性）；删超界页后 source 落点 `files` 行联动清理（零僵尸行）；Windows 仅大小写改名但主干段大小写不同（`abc` vs `Abc-003.pptx`）仍识别（非 Windows skip）；mtime 晚于 splitAt + 2s 的同类文件保留、早于则删除；`splitArtifactPageSeq` 主干含 `-`/数字仍可正确切分。
- 大小写折叠（2026-11-02）：`sameNameFold` 三态（精确 / Windows 折叠 / 非 Windows 不等）与 `foldNameInSet` 表驱动；`isReplaceArtifactName` 精确形态全平台命中、大小写变体仅 Windows 命中（非 Windows 断言 false），含中文主干多字节切片；`snapshotReplaceArtifactNames` 快照折叠按平台断言；`resolveExistingDirCase` Windows 实际名归一，非 Windows 原样返回（不读父目录）；`dominantSplitArtifactStem` 唯一折叠命中按平台分流；Windows 仅大小写改名后 replace 重拆成功、越界旧页收敛、无 `.replace-bak` 残留、新页索引路径沿用磁盘目录大小写、旧大小写行清除、再扫一轮零幽灵 lost 行（非 Windows skip）。




