# Research: 目录级拖拽后 scan done 双重刷新去重方案评估

- **Query**: 评估 onFolderDropFolder 显式刷新与后端自动重扫 assets-scan:done → onScanDone → resetAndReloadView 的双重刷新去重方案（方案一事件来源标记 / 方案二抑制窗口 / 方案三代次比对）
- **Scope**: internal（前端 AssetsPanel.vue + 后端 service/assets_*.go + 测试面）
- **Date**: 2026-10-20

## TL;DR

**推荐「方案零：维持现状，不实施任何去重方案」**——评估发现任务前提不成立：`onScanDone` 自素材面板 MVP（013032e）起就只复位进度态、从不触发数据刷新（`refreshAfterScan → resetAndReloadView` 仅由手动扫描/重建的 Promise 成功分支调用），且有专门单测守护（AssetsPanel.spec.js:1852「done 事件仅复位进度（数据刷新由 Promise 分支统一执行防双拉取）」）。目录级移动的数据刷新**自始至终只有显式一轮**，且因后端 `autoScan` 是同步调用，这一轮恰好发生在扫描收敛之后，读到最终态——时序语义已然最优。三个去重方案均无作用点，强行实施是零收益改动；真正的遗留问题是 AssetsPanel.vue:3177 注释中「与扫描 done 的双重刷新一轮属可接受代价」措辞失实（沿袭自 10-20-e2e PRD 的误认），建议由后续独立任务仅修正注释与 PRD 记载，不动任何逻辑。

---

## 1. 现状精读

### 1.1 四函数实现（frontend/src/components/AssetsPanel.vue）

| 函数 | 位置 | 职责 | 数据拉取面 |
|---|---|---|---|
| `onFolderDropFolder` | :3180 | 目录级拖拽移动入口（FolderTree `drop-folder` 事件，:477 绑定） | 无直接拉取，委托刷新入口 |
| `refreshDataAfterMutation` | :1756 | 数据变更统一刷新入口（bb7a1fa 收敛，六链共用） | 按差异面参数表达 |
| `onScanDone` | :4436 | assets-scan:done 事件回调 | **仅 `scanProgress.value = null`，零数据拉取** |
| `resetAndReloadView` | :2838 | 语境变更全量重载（切库落定 / 扫描 Promise 成功分支） | 全清 + 并行四任务 |

**onFolderDropFolder 关键段**（:3180-3200）：

```js
const result = await MoveAssetFolder(currentLibraryId.value, sourcePath, targetPath)
// ... ElMessage + folderPath 前缀改写（浏览语境保留）
await refreshDataAfterMutation({ reloadFolders: true })
```

**refreshDataAfterMutation({reloadFolders:true}) 的拉取面**（:1756-1805）：

| 段 | 行为 | 条件 |
|---|---|---|
| 底册 | `ListAssetFiles` 重拉 `fileEntries` | 恒执行 |
| 检索态 | `runSearch(true)` + 清聚合展开缓存 | `searchMode` 为真 |
| 幻灯片视图 | `loadSlidesPage(true)` 复位首页 + `ensureFolderSlidesLoaded`/`ensureFilteredSlidesLoaded` 双路兜底 | `viewMode==='slide'` |
| 其他视图 | 置 `slidesStale=true` 惰性重拉，不白拉首页 | 非 slide 视图停留 |
| 文件夹树 | `loadFolders()` 重拉 | `reloadFolders=true` |
| 统计 | **不拉**（本链未传 `reloadStats`） | — |
| 语境 | **保留** folderPath / 选中 / 预览（数据变更点位，§8.7 矩阵） | — |

双代次守卫（`librarySeq`+`searchSeq` 发起时快照、写回逐个比对）全程在位。

**onScanDone 完整实现**（:4436-4442）：

```js
const onScanDone = (payload) => {
  if (!payload || payload.libraryId !== currentLibraryId.value) return
  // 仅复位进度展示；数据刷新由扫描/重建 Promise 成功分支的 refreshAfterScan 统一
  // 执行——done 事件先于 Promise 返回到达，双路径各刷一次会使大库全量清单拉取
  // 翻倍（审核 12：二选一，Promise 分支为准）
  scanProgress.value = null
}
```

**resetAndReloadView 拉取面**（:2838-2862）：全清语境（folderPath 置空回「全部素材」、清选中、关详情/预览、清过滤、清聚合缓存）+ 并行 `loadStats` / `loadView`（检索态 runSearch / 文件视图 ListAssetFiles / 幻灯片视图 loadSlidesPage）/ `loadFolders`，非文件视图追加 `ensureFileEntries`。

### 1.2 真实双重刷新时序（目录级拖拽全链）

```
前端                                后端
────                                ────
onFolderDropFolder
  │ await MoveAssetFolder ────────▶ runFolderRelocate (assets_folders.go:469)
  │                                   ├─ tryLockScan（被并发扫描抢占则直接报错返回）
  │                                   ├─ moveAssetFolderCore：磁盘 rename + 索引前缀
  │                                   │  同步 + 排除双向翻转 + folder_meta 级联
  │                                   ├─ 释放扫描锁
  │                                   ├─ autoScan(context.Background(), libID) (:508)
  │                                   │   └─ 【同步】StartScan 完整跑完：
  │   ◀── progress*N（进度条显示）     │      逐文件 assets-scan:progress
  │   onScanProgress → scanProgress   │      终态 assets-scan:done (:160)
  │   ◀── done（Promise 返回前发出）   │      
  │   onScanDone → 仅复位进度条        │
  │ ◀── result（扫描已收敛后）─────────┘
  ├─ ElMessage.success
  ├─ folderPath 前缀改写
  └─ await refreshDataAfterMutation({reloadFolders:true})
       └─【唯一一轮数据刷新，读到的已是扫描收敛后的最终态】
```

### 1.3 覆盖面超集分析（任务书问题 1 结论）

**任务书设想的「第二轮」（done → onScanDone → resetAndReloadView）不存在**，超集判断不适用。但为完整回答，给出假想对照：若 done 真触发 `resetAndReloadView`，它相对显式一轮是「数据超集 + 语境退化」——

| 维度 | 显式轮 refreshDataAfterMutation | 假想轮 resetAndReloadView |
|---|---|---|
| loadStats | 不拉 | 拉 |
| 底册 ListAssetFiles | 拉 | 拉（loadView 或 ensureFileEntries） |
| 当前视图清单 | 拉（含 stale 语义） | 拉 |
| loadFolders | 拉 | 拉 |
| folderPath 浏览语境 | **保留** | **清空回「全部素材」** |
| 选中/详情/预览 | 保留 | 清空 |

即假想第二轮虽是数据超集，但会清空 bb7a1fa 特意保留的 folderPath 浏览语境（:3190-3193 前缀改写将白做）——**若双重刷新真的存在，去重的正确方向也是保显式轮、跳过 done 轮**，这一点与方案一的设计意图一致；但该作用点在当前代码中不存在。

## 2. 后端触发链核定

### 2.1 自动重扫的触发方式：同步，非 goroutine

- `service/assets_folders.go:508`（runFolderRelocate，RenameAssetFolder/MoveAssetFolder 共用）与 `:751`（MoveAssetFiles，`result.Succeeded > 0` 时）均为**同步调用** `s.autoScan(context.Background(), libraryID)`。
- `autoScan`（:761-766）：`autoScanFn` 注入优先（单测 fake），nil 回退生产实现 `StartScan`。**无 goroutine 包裹**——`MoveAssetFolder` 的 Wails 调用在扫描完整跑完后才返回。
- 时序要点：核心段先 `tryLockScan` 持锁做索引同步，**锁段收拢闭包**先释放扫描锁，随后的 `autoScan` 自取锁（:479-496 注释——防自撞 E_ASSETS_SCAN_IN_PROGRESS）。

### 2.2 扫描失败降级三形态与事件行为（任务书问题 2 核实）

| 形态 | 后端行为 | done 事件 | error 事件 | 前端感知 |
|---|---|---|---|---|
| op 执行失败（磁盘/解析层） | `runLibraryScan` 返回 err（assets_scan.go:147-153） | **不发** | 发 `assets-scan:error`（:151） | onScanError 仅复位进度，不弹提示；runFolderRelocate 降级 Warn 照常返回成功 result，前端 Promise 成功分支照常显式刷新 |
| 被并发扫描抢占（E_ASSETS_SCAN_IN_PROGRESS） | `tryLockScan` 失败直接返回（assets_scan.go:119-122） | 不发 | **也不发**（未进入 op 段） | 零事件；runFolderRelocate 降级 Warn；最终一致由在途扫描完成时的 done 兜底（若在途扫描是手动扫描，其 Promise 分支 refreshAfterScan 会全量重载） |
| 取消（CancelScan） | mapAssetsCancelError 映射 E_ASSETS_CANCELLED | 不发 | 发 error | 同上降级 |

**10-20-e2e PRD 记载「扫描失败降级时不抛错、done 不来」核实成立**：「不抛错」= runFolderRelocate :503-511 降级 slog.Warn 照常返回成功；「done 不来」= 失败路径只发 error（或被抢占时零事件）。这也是 onFolderDropFolder 显式刷新存在的正当性依据（不依赖 done 兜底）。

### 2.3 done 事件发出代码位与载荷结构

- 发出位置：`service/assets_scan.go:160`，`runLibraryScan` op 成功后、`StartScan` 返回前：
  `s.emitCurrent("assets-scan:done", map[string]any{"libraryId": libraryID, "result": result})`
- 载荷：`{libraryId: string, result: model.AssetScanResult}`；`AssetScanResult`（model/assets.go:116-138）为纯计数汇总（total/new/updated/unchanged/failed/encrypted/denied/lost/revived/excluded/removed），**无来源标识字段**。
- 事件经 EventSink 出口派发（`emitCurrent` 未注入 sink 时静默跳过）；桌面与浏览器通道同构（assets 服务已登记 `web_serve.go` applySink）。

## 3. 事件消费面核定（任务书问题 3）

全仓 grep `assets-scan:done`：

| 位置 | 性质 |
|---|---|
| AssetsPanel.vue:4557 | **唯一运行时注册点**（`EventsOn('assets-scan:done', onScanDone)`，onBeforeUnmount 闭包注销） |
| AssetsPanel.spec.js:618 | 单测断言注册面存在 |
| AssetsPanel.spec.js:1873-1884 | **守护用例**：断言 done 到达后 `ListAssetFiles` 调用数不增加（仅复位进度） |
| e2e 目录（assets-folder-drag.spec.js 等） | **零引用**——E2E fixtures 不模拟扫描事件，drop-folder E2E 中 MoveAssetFolder mock 同步 resolve 后仅显式刷新一轮 |
| wails-mock-defaults.js | 仅方法 mock（:239 `MoveAssetFolder`），无事件载荷 mock |

结论：消费面与 10-20-e2e PRD 初核一致，唯一消费方 AssetsPanel 自身，无遗漏（含 e2e mock 面）。

## 4. 方案对比

**前提勘误先行**：三个方案均为「消除第二轮 resetAndReloadView」设计，而第二轮在当前代码中不存在（§1.2/§2 证据链）。以下对比按「假设前提成立」与「实际作用点」两个视角给出。

### 方案一：scan done 事件带来源标记（source/origin 字段）

- **实现要点**：`runFolderRelocate` 调 `autoScan` 时传递来源（需给 autoScan/StartScan 加来源参数或独立事件载荷字段），done 载荷 map 追加 `"source": "folder-move"`；前端 onScanDone 收到该来源且显式刷新已完成时跳过 resetAndReloadView。
- **实际作用点**：**不存在**——onScanDone 本就不调 resetAndReloadView，加字段后无消费逻辑可写，改动是纯死代码。
- **假想牵连面**（若前提成立）：service 层 StartScan 签名或事件载荷契约 + model + AssetsPanel onScanDone + spec.js:1875 载荷构造四处；Wails 事件载荷不经 wailsjs 绑定层（无绑定同步），但载荷结构变更须同步 spec 测试载荷；e2e 零影响（事件不被 e2e 触发）。
- **风险**：向 StartScan 公共契约注入「一次调用来源」参数，污染手动扫描/重建/未来触发点的公共签名；载荷字段向后兼容靠前端 undefined falsy 判断，语义隐晦。
- **测试守护面**：需新增「带 source 的 done 不触发第二轮」用例——守护的是一段本不存在的逻辑。

### 方案二：显式刷新后抑制窗口（前端短窗 2-3 秒忽略下一轮 done）

- **实现要点**：onFolderDropFolder 显式刷新完成后置一次性 skip 标记或时间窗，onScanDone 命中窗口时跳过 resetAndReloadView。
- **实际作用点**：**不存在**——同上，无可跳过的第二轮。
- **假想风险**（若前提成立）：扫描时长不可预估（大库分钟级、小库毫秒级）。同步 autoScan 形态下 done 恒在 MoveAssetFolder 返回前发出（早于显式刷新完成），固定时间窗要么恒命中（等于永久跳过、语义等同方案一但更脆弱）要么恒不命中（零效果）；若未来扫描改异步 goroutine，窗口时序博弈（慢于窗口双刷新回归、快于窗口误伤正常去重）正式成立，固定窗脆弱。
- **测试守护面**：时间窗逻辑需要 fake timer，用例成本高于收益。

### 方案三：数据代次/版本比对（onScanDone 比对数据代次，未变则跳过）

- **实现要点**：后端在 done 载荷或查询接口暴露数据版本（如扫描结果计数指纹），onScanDone 比对本地代次未变则跳过 resetAndReloadView。
- **实际作用点**：**不存在**；且「数据未变」的判定口径难收敛——目录移动后底册 path 全部前缀改写（Replaced 计数 > 0），任何计数指纹都变化，代次比对恒判「已变」，方案退化为无跳过。
- **假想牵连面**（若前提成立）：后端版本机制 + 查询接口 + 前端比对逻辑，三方案中牵动面最大。
- **风险**：版本语义与 assets 域既有双代次守卫（librarySeq/searchSeq）语义重叠，引入第三套代次体系。

### 方案零（推荐）：维持现状

- **理由**：数据刷新自始仅显式一轮；同步 autoScan 保证显式轮读到收敛后最终态（若在扫描前刷新，翻转遗留面未收敛反而需第二轮）；失败降级三形态下显式轮均保底（§2.2）。现状即语义精确的最小刷新形态，零改动零风险。
- **唯一真实遗留**：AssetsPanel.vue:3176-3178 注释「不再单纯依赖尾部自动扫描 done 事件兜底……**与扫描 done 的双重刷新一轮属可接受代价，正确性优先**」——后半句与代码事实不符（done 从不刷新数据），系沿袭 10-20-e2e PRD Q2（「代价：与扫描 done 事件双重刷新一轮」）的误认。该措辞会持续误导后续维护者发起同类「去重」评估（本次即一例）。建议后续独立任务仅修正该注释与 10-20-e2e 归档 PRD 的认知记载（如需），不动逻辑；bb7a1fa 引入显式刷新的正当性理由（「扫描失败降级时 done 不来，陈旧无救」）依然成立且充分。

## 5. 影响面边界（任务书问题 6）

维持现状零改动，其余扫描触发点既有刷新链不受影响：

| 扫描触发点 | 刷新链 | 现状 |
|---|---|---|
| 手动扫描（onScan :4092） | Promise 成功分支 → refreshAfterScan → resetAndReloadView | 不变 |
| 手动重建（onRebuild :4108） | 同上 | 不变 |
| 目录移动（onFolderDropFolder）/ 文件移动（onFolderDrop） | 显式 refreshDataAfterMutation（同步 autoScan 后执行） | 不变 |
| 库切换/新建/删除回退 | onLibrarySelectionSettled → resetAndReloadView | 不变 |
| 目录移动与手动扫描并发 | 核心段 tryLockScan 报错返回，handleError 分流；或 autoScan 被抢占降级 Warn | 不变 |
| defineExpose refreshAfterScan（:4600） | 预留面，当前全前端无外部调用 | 不变 |

## Caveats / Not Found

- 10-20-e2e 归档 PRD（.trellis/tasks/archive/2026-10/10-20-e2e/prd.md）与 bb7a1fa 提交信息中「与扫描 done 双重刷新」的记载均与代码事实不符，本报告以代码 + 单测守护用例（spec.js:1852-1884）+ git 历史（refreshAfterScan/onScanDone 自 013032e MVP 起无 -S 命中变更）三重证据为准。
- 本任务（10-20-ep-2-14-5-e2e）R3 的 drop-folder E2E 用例设计不受影响：mock 的 MoveAssetFolder 同步 resolve 与真实后端「同步扫描后返回」在「返回后仅显式刷新一轮」上等价，E2E 无需模拟扫描事件。
- 未评估前端之外对 done 载荷的潜在消费方（如未来 web 通道脚本），当前仓内无此类消费。

## Related Specs

- docs/spec/assets-management.md:43 —— done 事件「仅复位进度态，不刷新数据」契约（2026-10-14 收口记载，与代码一致）
- docs/spec/assets-management.md:229 —— 目录级移动统一核心 moveAssetFolderCore 契约
- docs/spec/assets-management.md §8.7 —— 数据变更刷新统一入口契约（bb7a1fa 沉淀）
