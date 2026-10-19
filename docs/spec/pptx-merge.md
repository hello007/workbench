# 多源页合并导出与素材购物栏契约（PptxMerge / AssetCart）

> 来源：任务 10-19-assets-cart-page-collect-merge-export（2026-10-19，PR1/PR2 合并核心 + PR3 购物栏装配与前端）。技术选型依据（路线 A 基底注入式合并，四路线对比）见该任务 `research/ooxml-merge-approaches.md`；拆分侧既有契约见 `pptx-split.md`（合并大量复用其读取/编辑原语），素材域契约见 `assets-management.md`。

## 1. Scope / Trigger

- 触发：改动 `service/pptx_merge.go`（合并核心）、`service/pptx_merge_service.go`（合并编排）、`service/asset_cart_service.go`（购物栏存储）、`model/pptx.go` / `model/asset_cart.go`、`app_asset_cart.go`、前端 `AssetCartDrawer.vue` / AssetsPanel 购物栏链路。
- 能力定位：素材管理「购物栏」——跨库页级收集（条目键 `(libraryId, fileId, slideNum)`）+ 拖拽排序 + 按购物栏顺序合并导出为单一 .pptx。纯 Go OOXML 标准库实现（`archive/zip` + `encoding/xml`），**禁 COM / 外部 Office 依赖**（与拆分同策略，CI 可测）。

## 2. Signatures

| 层 | 签名 | 位置 |
|---|---|---|
| 合并核心 | `MergePptxPages(ctx, items []model.PptxMergeItem, outPath string, onProgress func(done, total int)) (model.PptxMergeResult, error)` | `service/pptx_merge.go` |
| 合并编排 | `PptxMergeService.Merge(ctx, items, outPath)` / `CancelPptxMerge() bool`（取消注册表键 = normalizeRepoKey(outPath)，事件 `assets-cart:export-progress/done/error` 经 sinkHolder） | `service/pptx_merge_service.go` |
| 购物栏存储 | `AssetCartService.List/Add([]AssetCartItem) (AssetCartAddResult, error)/Remove(keys)/Clear/Reorder(orderedKeys)/MergeCartToPptx(ctx, outPath)/CancelAssetCartExport()` | `service/asset_cart_service.go` |
| App 绑定 | `ListAssetCartItems` / `AddAssetCartItems` / `RemoveAssetCartItems` / `ClearAssetCart` / `ReorderAssetCart` / `MergeAssetCartToPptx(outPath)` / `CancelAssetCartExport()` | `app_asset_cart.go` |
| 装配 | `AppServices` 加 `pptxMergeSvc` + `assetCartSvc` 两字段两构造行；`web_serve.go` `applySink` 补 `pptxMergeSvc.SetEventSink`（购物栏服务本身不发事件，合并编排才是事件出口持有者） | `app_services.go` / `web_serve.go` |
| 模型 | `model.AssetCartItem{libraryId, fileId, slideNum, srcPath, fileName, slideTitle, libraryName, addedAt, stale}`（Key() = `<libraryId>|<fileId>|<slideNum>`）、`model.AssetCartAddResult{added, duplicates}`、`model.PptxMergeItem{srcPath, pageNum}`、`model.PptxMergeResult{outputPath, pages, pageCount, skippedItems, sldSzMixed}` | `model/asset_cart.go` / `model/pptx.go` |
| 前端键单点 | `assetCartItemKey(item)`（utils/assetCart.js，与后端 `AssetCartItem.Key` 同构，两处禁漂移） | `frontend/src/utils/assetCart.js` |

## 3. Contracts

### 3.1 合并核心（基底注入式，等价 PowerPoint「保留源格式」）

- **基底 = 清单第一项所在源文件**：先跑集合化 GC（`deleteSetForPages`，与 PR1 子集导出共用）只保留其被勾选页；其余源文件逐个提取保留页闭包（页 → 版式 → 母版 → 主题 → 媒体，rels 图 BFS，未勾选页与全部备注页为屏障）并经全局分配器重命名后注入。母版/版式/主题**整链注入不合并 theme**。
- **presentation.xml 是基底文件本体不注入**：sldIdLst 按购物栏顺序重建（基底页沿用原 rId 与数字 id，注入页追加分配）、sldMasterIdLst = 基底保留母版条目 + 注入母版条目追加、`p:sldSz` 恒取基底；custShowLst/extLst/embeddedFontLst 剥离。
- **三重冲突消解（三个独立命名空间分别唯一，禁混用）**：
  1. **part 名**：`mergeNameAllocator` 以基底包全部条目名为基线、跨注入源共享同一实例；原名可用则保留，冲突时按「去尾数字递增」找空位，同时保证 part 名与派生 `.rels` 名均不冲突；
  2. **presentation.xml.rels 的 rId**：`nextRelIDAllocator` 起点 = 现有 Id 最大数字后缀 +1（正则 `^rId(\d+)$`），跳过既有取值；
  3. **sldId@id（≥256）与 sldMasterId@id（≥2147483648）**：`nextIntAllocator` 各自取基底现有最大值 +1，两个数字命名空间互不相干。
- **命名空间精确解析缺陷（parsePptxListIDs 存在的根本原因）**：encoding/xml 的结构体标签 `xml:"id,attr"` 对任意命名空间的同名属性都命中且后出现者覆盖前者——sldId 条目同时带无前缀 `id` 与 `r:id` 两属性，标签解析恒取 r:id 值，数字 id 与关系 Id 的区分**必须按属性 Name.Space 精确解析**（`listIDEntryFromAttrs`：Space 为空 = 数字 id，Space 为 relationships 命名空间 = rId）。禁用结构体标签形态解析这两类条目。
- **XML 编辑一律「解析定位 + 字节删除/替换/拼接」**（`editXMLSpans`/`dropXMLSpans`/`appendXMLBeforeClosingTag`/`rebuildXMLListSpan`），禁 Encoder 重序列化；重建列表条目沿用文档既有命名空间前缀（`xmlListPrefix` 提取，防引入未声明前缀）。例外：注入 part 的 `.rels` 整体再生（纯关系表，字段全集 Id/Type/Target/TargetMode 固定，再生无损且规避属性级字节手术）。
- **OPC Target 语义**：Target 相对关系源 part 所在目录解析（pptx-split.md 踩坑先例）；注入后跨 part 引用换算经 `relTargetBetween`（同目录取基名、跨目录相对路径清洗 `../`，失败防御回退包根绝对路径形态）。
- **Content_Types**：注入 part 的 Override 优先照搬，无 Override 按扩展名补 Default（基底缺失该扩展名时，扩展名小写归一防重复补）。
- **跳过语义**：非基底源文件打不开/损坏/页号越界只跳过对应项继续（`SkippedItems` 报告）；基底打不开为整体失败；(源文件, 页号) 重复项与非法项跳过并报告；输出为原子写（`writeZipAtomic`），失败不留半态；outPath 禁与任何源文件同路径。

### 3.2 MVP 边界（与 PRD Out of Scope 对齐）

| 边界 | 行为 |
|---|---|
| 备注页 | 剔除（含基底保留页与注入源全部备注页；notesMaster 搬运列为后续增强） |
| 嵌入字体 | 剥离（presentation.xml.rels 内部 `/font` 后缀关系指向的 fntdata part + embeddedFontLst），缺失字体机器回退系统字体 |
| 尺寸混排 | 不缩放；任一注入源 p:sldSz 与基底不一致仅置 `SldSzMixed`，前端 warning「各源页面尺寸不一致，外来页可能变形」 |
| 跨页超链接 | 指向未勾选页/备注页的关系删除（拆分同语义）；外链（TargetMode=External）原样保留 |

### 3.3 合并编排与取消（logging-and-errors 取消链路契约）

- `PptxMergeService.Merge` 内部派生可取消 ctx + `registerCancel`/`unregisterCancel` defer LIFO（先注销后 cancel），键 = `normalizeRepoKey(outPath)`；`CancelPptxMerge` 全量取消（删全部注册项 + 逐个 cancel），无在途幂等 false。
- 取消错误经 `mapCancelError` 统一映射 `ErrOperationCancelled`（**复用** `E_GIT_CANCELLED`，前端 warning 中性提示）；进度/终态经 `emitCurrent`（`assets-cart:export-*` 前缀，素材购物栏域独立命名），serve 模式经 applySink 切换。

### 3.4 购物栏存储（data/asset_cart.json）

- **持久化**：`util.SaveJSON` 原子写（data-atomic-write 契约，禁 os.WriteFile 截断写）；损坏降级空购物栏 + slog.Warn 不阻塞（AssetSavedSearchService 同模式）；整体覆盖模型 last-writer-wins，运行期手改文件前须关闭 workbench.exe。
- **条目身份键** = `(libraryId, fileId, slideNum)`：fileId 为 files.id（重解析稳定，**禁 slides.id**——每轮重解析删旧插新全变，与选中体系/缩略图缓存键同理由）。跨库条目混装（fileId 仅库内唯一）。
- **快照字段**（srcPath/fileName/slideTitle/libraryName）入栏时定格不随后端回写——srcPath = 库根 + 相对路径（joinAssetPath），是失效探测基准与合并清单来源；展示旧名属登记取舍。
- **stale 探测态**：List 时逐条 os.Stat(srcPath)（轻量非扫描），不存在置 `stale`（`omitempty` 非持久化字段）；前端置灰 + 可手动移除，**不自动移除**；导出不预剔除——合并核心对打不开的源文件按 SkippedItems 报告（跳过口径单点在合并核心，禁两处判定漂移）。
- **Add 幂等去重**：键已在栏跳过计 duplicates，新增按入参顺序追加（批量加页顺序语义）；AddedAt 后端权威生成。
- **Reorder 完整排列校验**：orderedKeys 须为当前条目键的完整排列（长度不符或含未知键拒绝），防静默丢条目。
- **导出互斥**：`exporting` 标志，在途再导出报 `E_CART_EXPORT_IN_PROGRESS`（新增 warning 错误码，三处同步：model/app_error.go + frontend error.js ErrorCode + WARNING_CODES）；导出执行在锁外（锁内快照条目后放锁，导出中购物栏可查改，快照语义 + 前端禁用为第一防线）；取消转发 `PptxMergeService` 取消注册表。

## 4. Validation & Error Matrix

| 条件 | 行为 |
|---|---|
| 空购物栏导出 | 引导错误「购物栏为空」 |
| 在途导出中再次导出 | `E_CART_EXPORT_IN_PROGRESS`（warning，等待完成或先取消） |
| 取消导出 | `E_GIT_CANCELLED`（warning 中性提示），产物不落盘（原子写在末步，取消无半态） |
| 非基底源文件缺失/损坏/页越界 | 跳过该项继续，`SkippedItems` 报告；前端 toast 计数提示 |
| 基底（清单第一项）打不开 | 整体失败，不留半态 |
| (源文件, 页号) 重复项 | 保留首次出现，后续跳过并报告 |
| outPath 与源文件同路径 | 整体拒绝（防原地覆盖损坏源文件） |
| 购物栏文件损坏 | 降级空购物栏 + Warn，不阻塞 |
| Reorder 键集与现存条目不一致 | 拒绝（防静默丢条目），前端重拉还原 |

## 5. Good/Base/Bad Cases

- Good：改合并核心前先确认三重命名空间消解面与 `parsePptxListIDs` 的命名空间精确解析依据；新增错误码三处同步；购物栏键改动同步 utils/assetCart.js 与后端 Key() 两处。
- Base：新增购物栏字段 → model/asset_cart.go + wailsjs 重生成 + wails-mock-defaults.js 单一数据源三处同步。
- Bad：用 `xml:"id,attr"` 结构体标签解析 sldId/sldMasterId 条目（恒取 r:id 值，数字 id 全错）；跳过判定在购物栏层与合并核心各写一份；导出跳过 stale 条目预剔除（与核心 SkippedItems 口径漂移）。

## 6. Tests Required

- 合并核心（`service/pptx_merge_test.go`）：双源双母版主链路（输出页序 = 购物栏序 + part→rels 方向 r:id 可解析）、三源 ID 冲突消解、未勾选页跳转清理、媒体 Default 补齐、嵌入字体剥离；fixture 程序化构造（禁依赖本机 Office）。
- 购物栏存储（`service/asset_cart_service_test.go`）：Add 幂等去重 + 入参顺序、持久化重开回读、损坏降级、Remove 容忍未知键/全未命中报错、Reorder 完整排列校验、stale 探测（存在/缺失）非持久化、合并编排清单构造（fake provider 注入，购物栏顺序透传）、导出互斥（在途拒绝 + 结束复位）、取消映射 E_GIT_CANCELLED（预取消/阻塞 provider 模式）。
- 前端：AssetCartDrawer.spec（渲染/来源行/失效置灰/拖拽 reorder 按键列表/导出运行态）、AssetsPanel.spec（徽标计数、三处加号幂等、批量加页顺序、溯源定位、导出流程含默认名/可选清空/跳过报告/事件仅复位进度）；wails-mock-defaults.js 补七绑定默认值。

## 7. Wrong vs Correct

**Wrong（命名空间不精确解析）**：
```go
type sldIdEntry struct {
    ID  string `xml:"id,attr"`    // 恒命中 r:id 值（后出现者覆盖），数字 id 全错
    RID string `xml:"r:id,attr"`
}
```
**Correct**：
```go
for _, a := range attrs {
    switch {
    case a.Name.Space == "" && a.Name.Local == "id":          // 无前缀数字 id
    case a.Name.Space == pptxRelsNamespace && a.Name.Local == "id": // r:id
    }
}
```

---

**创建**：2026-10-19（任务 10-19-assets-cart-page-collect-merge-export）
