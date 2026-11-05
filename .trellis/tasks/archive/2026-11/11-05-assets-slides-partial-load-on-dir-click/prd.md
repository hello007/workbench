# 素材管理偶发目录点击只显示部分幻灯片

## Goal

修复素材管理面板 bug：浏览幻灯片视图点击子目录（如 `武汉农商`，61 页）时偶发只显示部分页（如 7 页），需在其他位置点击/滚动几次后才逐渐补全。期望点击目录后自动拉齐该目录全部页。

## Root Cause（已定位）

**`ensureFolderSlidesLoaded`（AssetsPanel.vue:3280）续拉终止条件语义错误：「出现即停」≠「拉齐」。**

链路事实：

* 浏览幻灯片视图 = `ListAssetSlides` **库级分页**（PAGE_SIZE=50，`ORDER BY f.path, s.slide_num` 字节字典序，service/assets_db.go:1136），目录过滤是已加载段上的**客户端收敛**（AssetsPanel.vue:2833 `folderPrefixMatch`）。
* 续拉兜底循环条件为 `!slideEntries.some(s => folderPrefixMatch(...))`——只要已加载段中存在**任意一条**目录命中即不续拉。
* 复现路径：首页 50 条中目标目录命中 7 条 → 循环判定「已出现」直接退出 → 视图过滤后仅显示 7 页；后续滚动/点击触发库级续拉逐渐补齐（用户观察到的「点几次才显示全部」）。
* 对比单文件先例 `ensureFilteredSlidesLoaded`（:4044）：「出现即停」对单文件成立（同文件各页排序连续，出现即全在）；目录下多文件散布多个批次，先例语义照搬后不成立。

排序连续性依据：`ORDER BY f.path` BINARY 字典序下，以目录前缀（组件级 `folderPath + '/'`）开头的路径构成**连续区间**（字典序基本性质；同源前缀目录如 `武汉农商/` 与 `武汉农商备份/` 由首差异字符整体定序不交错）。故「已见过命中 + 本批零新增 = 目录段已越界，后续批次必无命中」可安全提前停止。

## 附带发现（同族隐患）

续拉循环在 `loadSlidesPage(false)` 失败时不追加条目（零增长）：单文件先例与目录兜底在「目标不存在于库 + 加载持续失败」场景下均存在**死循环隐患**（旧条件永不满足退出）。新实现以「length 零增长即退出」统一防死循环。

## Requirements

* 目录切换后续拉兜底必须拉齐该目录全部页（或全局拉完），不再「出现即停」。
* 利用排序连续性提前停止（已见命中 + 本批零新增 → 越界退出），避免为小目录拉全库。
* 加载失败零增长时退出循环（防死循环）。
* **顺带修**：单文件先例 `ensureFilteredSlidesLoaded` 同族死循环隐患——以零增长守卫加固。
* 保持既有幂等语义：watch(folderPath) 与 onFolderSelect 显式调用并发安全（loadingMore 防重不变）。

## Acceptance Criteria

* [ ] 目录条目跨批次（首页部分命中 + 次批剩余）时，点击目录后自动续拉至全部页显示。
* [ ] 目录段越界后停止续拉（断言加载批次数不超目录段所需 +1）。
* [ ] 目录无命中（空目录/不存在路径）时拉完全局即停，不死循环。
* [ ] 加载失败（零增长）退出循环（目录兜底与单文件先例两处）。
* [ ] 续拉中切目录/切检索/切视图安全退出（既有守卫保留）。
* [ ] 组件单测覆盖上述场景且绿。

## Decision (ADR-lite)

**Context**：目录续拉兜底照搬单文件先例的「出现即停」语义，但目录条目跨分页批次，出现 ≠ 拉齐，导致部分显示。
**Decision**：方案 A——前端修 `ensureFolderSlidesLoaded` 终止条件（拉到目录段越界或全局拉完；依据 `ORDER BY f.path` BINARY 字典序下目录条目构成连续区间，已见命中 + 本批零新增即可安全判定越界），并顺带以零增长守卫加固单文件先例。用户确认（2026-11-05）。
**Consequences**：不动后端契约（方案 B 后端目录过滤分页出局，登记 Out of Scope）；目录语境仍为「客户端收敛 + 续拉兜底」既有架构；越界提前停保证小目录不为拉齐而拉全库。

## Out of Scope（拟）

* 后端按目录过滤分页（方案 B，架构演进，与既有「客户端收敛」决策冲突，改动面大）。
* 目录语境的 total/计数显示口径（既有形态，不因本 bug 联动改）。

## Technical Notes

* 涉及文件：`frontend/src/components/AssetsPanel.vue`（`ensureFolderSlidesLoaded` :3280-3293、watch :3297、先例 `ensureFilteredSlidesLoaded` :4044）、测试 `frontend/src/components/__tests__/AssetsPanel.spec.js`（5824 行，尚无目录续拉用例）。
* 排序口径：service/assets_db.go:1136 `ORDER BY f.path, s.slide_num`（SQLite BINARY）。
* 匹配口径：frontend/src/utils/assetFolder.js `folderPrefixMatch`（组件级前缀、正斜杠、大小写敏感，与 BINARY 排序同口径）。
