# 素材清单刷新链统一化 + 文件夹树拖拽 E2E 补齐

## Goal

素材管理面板（AssetsPanel）数据变更后的幻灯片清单（slideEntries）刷新目前散落五处、口径不一，且都以 `viewMode==='slide'` 为补拉条件——文件视图停留时批量操作/拖拽后切回幻灯片视图见陈旧段。收敛为统一刷新入口，消灭「视图停留时清单陈旧」整类问题；并补齐拖拽链路 Playwright E2E（现仅单测）。

## What I already know

* 上一任务 10-20-assets-folder-tree-drag 归档遗留项；契约基线 docs/spec/assets-management.md §8.7
* 刷新链现状（AssetsPanel.vue）：
  * `refreshAfterBatchAnnotation`:1775（批量打标签/评分 completed 入口，refreshAfterBatchTag/Rating 薄包装）——标签树 reload + fileEntries 重拉 + 检索态 runSearch(true)/浏览 slide 视图 loadSlidesPage(true)+ensureFilteredSlidesLoaded；双代次守卫（librarySeq+searchSeq）
  * `refreshAfterMaintain`:2304（清理丢失/前缀替换共用）——多 loadStats，其余同上，无 loadFolders
  * `onFolderDrop`:3169（文件拖拽移动）——fileEntries 重拉 + slide 视图 loadSlidesPage(true)+**ensureFolderSlidesLoaded**（唯一用 folder 兜底的链）+ loadFolders + clearSelection；仅 libSeq 守卫
  * `onFolderDropFolder`:3210（目录级移动）——仅 folderPath 前缀改写 + loadFolders，**零清单刷新**（靠尾部自动扫描 done 事件 → refreshAfterScan → resetAndReloadView 兜底；扫描失败降级时 done 不来，陈旧无救）
  * `onSaveSlideMeta`:3954 浏览分支——loadSlidesPage(true)+ensureFilteredSlidesLoaded + 条目引用同步（保存链特有回读语义）
  * `resetAndReloadView`:2863（语境变更：切库/扫描 done）——全清 + loadStats/loadView/loadFolders/ensureFileEntries 并行
* 陈旧根因：`onViewModeChange`:3481 切幻灯片视图仅 `slideEntries.length===0` 时 loadView 重拉；各数据变更链仅 slide 视图停留才补拉 → 文件视图停留时 slideEntries 陈旧，切视图非空不重拉
* 兜底续拉两路：`ensureFilteredSlidesLoaded`:3768（fileFilterPath 单文件过滤）与 `ensureFolderSlidesLoaded`:3119（folderPath 目录过滤）；两者语境可叠加，现有链各只接一路
* HTML5 DnD 编排：`onCardDragStart`:3147（载荷经 utils/assetFolder 单点 MIME）+ FolderTree 组件 drop-files/drop-folder 事件分派宿主
* E2E 基建：frontend/e2e/fixtures.js（test/expect 注入 Wails mock）、src/test/wails-mock-defaults.js 单一数据源、禁 waitForTimeout；新增通道目录须同步 vitest.config.js test.exclude
* Wails 方法 mock：MoveAssetFiles/MoveAssetFolder/ListFolders/ListAssetFiles/ListAssetSlides 等须在 wails-mock-defaults.js 登记

## 影响面结论（brainstorm 报告，2026-10-20）

收敛影响面未超预期：仅前端 AssetsPanel.vue 刷新编排 + 测试，后端零改动；不牵连导出/购物栏刷新（locateCartEntry 为语境进入路径非数据变更链，导出统计弹窗链独立）。

## Decision (ADR-lite)

**Q1 解法形态 = B stale 标记惰性重拉**（用户确认「全按推荐来」）
Context：五条数据变更链仅幻灯片视图停留时补拉 slideEntries；onViewModeChange 又只在空清单时重拉——文件视图停留时操作后切回见陈旧段。备选：A 无条件补拉（每次操作白拉首页且截断已加载段）、C 切视图无条件重拉（纯切视图丢懒加载进度）。
Decision：数据变更刷新链在非幻灯片视图停留时置 `slidesStale=true` 不补拉；onViewModeChange 切入幻灯片视图遇 stale 即重拉首页（复位 stale）。
Consequences：省请求、保留懒加载段语义，代价一个标记位；数据变更点位矩阵新增 stale 登记面。

**Q2 onFolderDropFolder 纳入统一刷新**（用户确认）
Context：目录级拖拽现零清单刷新，靠尾部自动扫描 done 事件兜底；扫描失败降级时（§8.7 P2：不抛错、done 不来）陈旧无救。
Decision：纳入统一刷新函数（fileEntries + 幻灯片清单 stale 语义 + loadFolders）。代价：与扫描 done 事件双重刷新一轮，正确性优先。

**Q3 E2E 范围 = 任务书 3 条**（用户确认）
文件卡拖到树节点成功 / 禁落点拒绝 / 多选拖拽批量。目录级 drop-folder 拖拽列 Out of Scope（有单测覆盖，E2E 收益低）。

**附带收口：续拉兜底双接**（勘察发现，随收敛一并修）
现状各链续拉兜底只接一路（onFolderDrop 用 ensureFolderSlidesLoaded、其余用 ensureFilteredSlidesLoaded），folderPath 与 fileFilterPath 语境可叠加；统一函数内两路兜底都接。

## Requirements

* R1 幻灯片清单刷新收敛单一入口（承接各数据变更链差异面：stats/tagTrees/folders 可选段），保留双代次守卫与续拉兜底语义
* R2 消灭「文件视图停留时操作 → 切回幻灯片视图见陈旧段」（Q1 stale 标记方案）
* R3 onFolderDropFolder 纳入统一刷新（Q2）
* R4 统一函数内 folder/filtered 两路兜底都接
* R5 拖拽 E2E：文件卡拖到树节点成功、禁落点拒绝、多选拖拽批量（遵守 e2e-testing.md：fixtures.js import、wails-mock-defaults 单一数据源、禁 waitForTimeout）
* R6 回归：go test ./service/ + cd frontend && npm test + npm run e2e 全绿
* R7 docs/spec/assets-management.md §8.7 补统一刷新函数契约

## Acceptance Criteria

* [ ] 统一刷新入口单点，五处数据变更链（批量打标签/批量评分/维护/文件拖拽/目录拖拽）全走该入口，差异面经参数表达
* [ ] 文件视图停留时批量操作/拖拽后切幻灯片视图清单为最新（vitest 断言 stale 置位与切入重拉）
* [ ] 续拉兜底双接有测试断言
* [ ] E2E 三用例绿（成功/禁落点/多选）
* [ ] 既有批量打标签/评分用例零回归
* [ ] §8.7 契约补统一刷新函数与 stale 语义

## Definition of Done

* 后端/前端测试通过（service ≥76%、前端 ≥70% 门禁）
* 文档更新：§8.7 + README 确认
* 新 E2E 目录若新增须同步 vitest.config.js exclude

## Out of Scope (explicit)

* 后端 service 刷新逻辑改动
* onSaveSlideMeta 保存链回读语义重构（条目引用命中/miss 分支保持独立，仅清单重拉段收敛）
* 目录级 drop-folder 拖拽 E2E（单测已覆盖）
* 触屏拖拽适配

## Implementation Plan (small PRs)

* PR1：统一刷新函数 `refreshDataAfterMutation`（stats/tagTrees/folders 可选段 + slide 清单 stale 语义 + 双路兜底 + 双代次守卫）——五处数据变更链迁移接入 + onViewModeChange stale 消费 + vitest 断言
* PR2：拖拽 E2E 三用例（成功/禁落点/多选）+ wails-mock-defaults 补 MoveAssetFiles 等登记 + 回归全量（go test ./service/ + npm test + npm run e2e）
* PR3：§8.7 契约沉淀 + README 确认

## Technical Notes

* 关联文件：frontend/src/components/AssetsPanel.vue、frontend/e2e/、frontend/src/test/wails-mock-defaults.js、frontend/vitest.config.js（若新增 E2E 通道目录须同步 exclude）
* 现有不一致附带发现：兜底续拉两路各链只接一路；onFolderDropFolder 零清单刷新
* vitest 现有用例在 frontend/src/components/__tests__/AssetsPanel.spec.js
* onSaveSlideMeta 浏览分支的重拉段可走统一函数，但条目引用命中/miss 回读分支保留原地
* resetAndReloadView（语境变更链）不在收敛面：全清语义与数据变更链不同构，仅登记边界
