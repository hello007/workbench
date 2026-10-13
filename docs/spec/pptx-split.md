# PPT 按页拆分（PptxSplit）契约

> 来源：任务 10-13-ppt-split-per-page（2026-10-13）。技术选型与最小编辑集依据见任务 `research/pptx-split-approaches.md`。

## 1. Scope / Trigger

工具箱功能：对 .pptx 按页拆分，输出到同级同名文件夹，每页一个独立新 pptx。纯 Go OOXML 实现（标准库 `archive/zip` + `encoding/xml`，零第三方依赖），禁引入 COM / 外部 Office 依赖（CI 不可测 + 无安装保证）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 核心 | `SplitPptxPerPageWithProgress(ctx, srcPath string, onProgress func(done, total int)) ([]string, error)`；`SplitPptxPerPage(ctx, srcPath)` 为 nil 回调直通 | `service/pptx_split.go` |
| 编排 | `PptxSplitService.Split(ctx, srcPath) (model.PptxSplitResult, error)` / `Cancel()`；自持 `opMu + cancels` 注册表，复用包内 `cancelEntry` | `service/pptx_split_service.go` |
| App 绑定 | `SplitPptxPerPage(srcPath string) (model.PptxSplitResult, error)` / `CancelPptxSplit() bool`（无参 = 全量取消，返回是否取消至少一项） | `app_pptx.go` |
| 事件 | `pptx-split:progress` `{srcPath, done, total}` / `pptx-split:done` `{srcPath, result}` / `pptx-split:error` `{srcPath, message}` | EventSink 通道 |
| 结果模型 | `model.PptxSplitResult{OutputDir string; Files []string; PageCount int}`（普通字段，无具名 string 类型，不触发 models.ts 别名规则） | `model/pptx.go` |

## 3. Contracts

- **最小编辑集**（拆分 = 复制整个 zip 后做一次列表编辑，其余 part 逐字节原样拷贝）：
  1. `ppt/presentation.xml` `<p:sldIdLst>` 删其他页条目
  2. `ppt/_rels/presentation.xml.rels` 删对应 Relationship
  3. `[Content_Types].xml` 删 slide 与 notesSlide `<Override>`
  4. 删 `ppt/slides/slideN.xml`（+ `.rels`）与所属 `ppt/notesSlides/notesSlideN.xml`（+ `.rels`）
  5. 剥离 `<p:custShowLst>` 与根级 `<p:extLst>`（sections 在 extLst 内，整块剥离）
  6. 保留页 rels 中指向已删页的 jump-to-slide 关系清理（rel type 以 `/slide` 结尾，保留 External 外链与自身跳转）
- **页序三跳映射**：sldIdLst → rId → presentation.xml.rels → part 名。slide 文件名与展示页序无关，禁按文件名序拆。
- **输出**：同级 `<原文件名>/` 文件夹，文件名 `<原文件名>-<两位页序号>.pptx`；文件夹已存在覆盖重生成。
- **原子写**：`os.CreateTemp` 同目录 → 写入 → Sync → `os.Rename`（复刻 `util/json.go` SaveJSON 序列；二进制 zip 不适用 SaveJSON 本体）。
- **取消**：`context.WithCancel` + `registerCancel`/`unregisterCancel` defer LIFO（先注销后 cancel），键 `normalizeRepoKey(srcPath)`，取消错误 `%w` 包装 `context.Canceled` 经 `mapCancelError` 映射 `ErrOperationCancelled`（**复用** `E_GIT_CANCELLED`，不新增错误码）。
- **事件注入**：走 `s.emitCurrent`（EventSink 持锁投递契约），禁直调 `runtime.EventsEmit`；`web_serve.go` `applySink` 已补 `SetEventSink`（现六服务，含 pptxSplit）。

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

- 事件仅 `PptSplitDialog.vue` 监听；`FileTreePanel.vue` 右键入口刻意不监听（Promise 直返 + message 反馈），规避多组件共听面。关闭对话框与 `onBeforeUnmount` 双时机用 `EventsOn` 返回闭包精准注销，禁 `EventsOff`。
- 关闭即重置输入/结果/在途态；后端在途任务继续跑，事件载荷经 `pendingSrcPath` 过滤防旧结果串入。
- error 事件只复位状态不弹提示（提示由 Promise reject 的 `handleError` 统一分流，防双弹）；`E_GIT_CANCELLED` 命中 WARNING_CODES 中性提示。
- wailsjs 三处绑定（App.js / App.d.ts / models.ts）经 `wails generate module` 重生成，不入库。
