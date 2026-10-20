# EP 2.14.5 升级确认型弹框行为回归排查 + 素材拖拽 E2E 增补

## Goal

element-plus 经 41c4b1a（package-lock caret 漂移 `^2.13.7` → lock 2.14.5）升级后，确认型错误经 ElMessageBox.confirm 递归重弹的组件行为漂移已在素材导出 runExport 一处实测（10-20-e2e 对齐修复）。全仓排查其余确认型错误消费组件并补 drop-folder 目录级拖拽 E2E；另评估 onFolderDropFolder 双重刷新去重方案（仅评估不动代码）。

## What I already know

* EP 升级实质：package.json 声明 `^2.13.7`（caret 范围），41c4b1a 更新 lock 后实际安装 2.14.5（minor 漂移非显式升级）
* 漂移定性（bb7a1fa 修复注释提炼）：组件权威行为 = 「确认型错误恒弹 confirm（递归语义）」；EP 2.13.7 时代 message-box 关闭动画期间重开异常（第二次 confirm 不可见/异常 reject 落入 handleError warning 分流）；EP 2.14 起重开正常显示 + 关闭后实例驻留 DOM（同名选择器命中多元素，E2E 须 .first()/.last() 区分弹出序）
* 修复先例：bb7a1fa frontend/e2e/assets-export.spec.js——断言从「重调仍拒走 handleError warning」改为「再次弹确认框（.last()）」
* ElMessageBox.confirm 生产调用面：24 组件文件约 48 处
* vitest 面：ElMessageBox.confirm 全局 mock（src/test/setup.js），EP 运行时漂移被 mock 屏蔽 → vitest 断言面不受 EP 升级影响（以组件代码分支为准）
* E2E 面：真 EP 运行时，仅「同一链路可能弹第二次 confirm」的消费点受漂移影响
* A 类清单（错误驱动/二段 confirm，约 15 处 / 12 组件）：
  * AssetExportDialog.vue:235（E_AssetsExportTargetExists 覆盖确认重调）——已修复+E2E 守护已有
  * AssetsPanel.vue:2425（AssetsRelinkMismatch 重链确认 → force 重调）
  * AssetsPanel.vue:3074（目录删除 catch → 空目录确认重试）
  * LocalChanges.vue:368（catch 内 confirm）
  * GitBranches.vue:214（catch (err) 内 confirm）
  * FileTreePanel.vue:1376（catch 内 confirm）
  * Obsidian vault 两段链 ×3：DirectoryTree.vue:672+680 / ContentPanel.vue:817+825 / FileTreePanel.vue:1506+1514（catch 判错 → 提示 confirm → 确认后二次 confirm，背靠背弹出直接命中关闭动画重开）
  * AiChatPanel.vue:426/612/889（catch 内 confirm ×3）
  * AiTaskHistoryPanel.vue:291/313（catch 内 confirm ×2）
  * PortListPanel.vue:150+166 / ProcessManagerPanel.vue:164+184（终止 → 失败 → 强杀确认二段链）
* B 类（单次操作前确认，行为稳定）：其余约 33 处
* drop-folder E2E：无既有用例（10-20-e2e Out of Scope 兑现项）；wails-mock-defaults.js:239 已登记 MoveAssetFolder；先例 assets-folder-drag.spec.js 三用例（dispatchEvent + 真实 DataTransfer 构造）
* 双重刷新现场：onFolderDropFolder（AssetsPanel.vue:3180）显式 refreshDataAfterMutation({reloadFolders:true}) + 后端自动重扫 assets-scan:done → onScanDone → resetAndReloadView 第二轮
* scan done 消费面：前端 assets-scan:done 唯一消费方 = AssetsPanel 自身（onScanDone，:4557 注册），无面板外消费方

## Decision (ADR-lite)

**Q1 排查范围 = A 类全查 + B 类仅机械核对 E2E 可达链路**（用户确认）
Context：A 类 15 处是漂移直接命中面；B 类 33 处单次确认行为稳定，且 vitest 面 ElMessageBox 全局 mock 屏蔽 EP 运行时漂移。
Decision：A 类 15 处逐处核对（组件权威行为 vs 用例断言 vs 真机行为）；B 类仅核对既有 E2E 断言中 B 类弹框选择器是否命中多实例问题，vitest mock 面跳过。
Consequences：核对面收窄到 E2E 可达链路；核对表 A 类逐处 + B 类 E2E 可达面登记，B 类其余不登记。

**Q2 双重刷新去重 = 仅评估，不动代码**（用户确认）
Context：assets-scan:done 前端唯一消费方 AssetsPanel 自身，影响面窄；但方案一（事件带来源标记）牵后端事件契约、方案二（抑制窗口）牵时序语义，均超出本任务主体。
Decision：方案对比（来源标记 vs 抑制窗口）产出写进 research/，含推荐结论；实现独立成任务。
Consequences：onFolderDropFolder 双重刷新现状保持（正确性优先，10-20-e2e 既定决策不变）。

## Open Questions

（已全部解决）

## Decision (ADR-lite) 追加

**Q3 Obsidian 顺序两段链守护 = vitest 三处各补两段链用例**（用户确认）
Context：#7/#8/#9 逐字节同构但无共享单点，一处独立改坏他处不知；是 15 处中唯一「EP 2.13.7 下第二段可能异常 reject 落静默 catch 且任何测试不可知」的零守护缺口；#11 删除会话 times(2) 断言先例现成。
Decision：DirectoryTree/ContentPanel/FileTreePanel 三处 vitest 各补约 2 用例（两段 confirm 确认到底：confirm times(2) + AutoRegisterAndOpen 调用；第二段取消静默：AutoRegisterAndOpen 不调用）。
Consequences：同构三处独立守护；不补 E2E（Obsidian 链需 OpenInObsidian not-registered 构造，vitest 已覆盖两段语义，E2E 收益低）。

## Research References

* [research/ep-confirm-consumers.md](research/ep-confirm-consumers.md) — 15 处 A 类核对：0 处现存漂移，组件代码与 vitest 断言全一致；「递归恒弹」闭环仅 #1（已修+守护）；#7/#8/#9 Obsidian 顺序两段链双面无守护；B 类裸选择器 9 处全单次链无风险；EP 2.14.5 下 17 spec 全绿零破坏实证
* [research/scan-done-dedup.md](research/scan-done-dedup.md) — 双重刷新不存在：onScanDone 仅复位进度从不刷数据（三重证据），refreshAfterScan→resetAndReloadView 仅手动扫描分支；推荐方案零维持现状；AssetsPanel.vue:3177 注释措辞失实待修正

## 排查结论（PR1 产出，2026-10-20）

* **现存需修漂移 0 处**：bb7a1fa 修复本质 = E2E 断言侧漂移（旧断言与组件递归权威语义不符），组件代码从未漂移
* 确认型弹框消费三形态分类学：递归恒弹（仅 #1）/ 顺序两段（#7/#8/#9/#11）/ 单次确认（其余 10 处）——EP 关闭动画重开差异仅暴露于前两形态
* E2E 多实例安全选择器两范式已落地：`.first()/.last()`（assets-export）与 `:visible`+`filter(hasText)`（assets-backup-rules）
* mock 修正：ElMessageBox mock 为各 spec 文件级 `vi.mock('element-plus')`，setup.js 仅 mock Wails 绑定（效果等价）
* 双重刷新前提失实：任务书第 3 项前提不成立，去重无对象；E2E 增补无扫描事件模拟负担（mock 同步 resolve 与真实同步扫描时序等价）

## Requirements

* R1 A 类 15 处逐处核对：组件权威行为 vs 用例断言 vs 真机行为，漂移处对齐组件权威行为 + 补守护用例（对齐 bb7a1fa 先例模式）
* R2 逐组件核对表登记任务 research/（A 类逐处 + B 类 E2E 可达面；含 EP 升级对其他 EP 组件既有断言影响核对：弹窗/下拉/树类）
* R3 drop-folder E2E 增补：目录拖到树节点，断言 MoveAssetFolder 调用参数 + folderPath 前缀改写 + 清单刷新；沿用 10-20-e2e dispatchEvent + 真实 DataTransfer 先例；新通道目录须同步 vitest.config.js exclude
* R4 双重刷新去重评估报告进 research/（方案对比 + 推荐，不动代码）
* R5 共性契约沉淀 docs/spec（若有）：候选新建 confirm 型弹框消费契约文档或并入 logging-and-errors.md 错误分流节，实现阶段定

## Acceptance Criteria

* [x] research/ 核对表覆盖 A 类 15 处逐处结论（一致/漂移 + 依据）+ B 类 E2E 可达面
* [x] 漂移排查结论：0 处现存漂移（组件代码与用例断言全一致），无需修复性改动
* [ ] vitest 三处 Obsidian 顺序两段链守护用例绿（times(2) + 取消静默，对齐 #11 先例）
* [ ] AssetsPanel.vue:3177 注释措辞修正（「双重刷新」失实记载）
* [ ] drop-folder E2E 用例绿（断言 MoveAssetFolder 参数 + folderPath 前缀改写 + 清单刷新；无需模拟扫描事件）
* [ ] vitest 全量 + E2E 全量 + go test 相关面零回归
* [x] research/ 含双重刷新去重评估（结论：前提失实，方案零维持现状）
* [ ] 共性契约沉淀 docs/spec（三形态分类学 + E2E 多实例选择器两范式）

## Definition of Done

* vitest / E2E / go test 相关面全绿
* research/ 三份产出：核对表、去重评估、（EP 组件断言核对并入核对表）
* docs/spec 沉淀 + README.md 更新确认

## Out of Scope (explicit)

* B 类 33 处逐处登记（仅核对 E2E 可达面）
* 双重刷新去重实现（仅评估，独立成任务）
* EP 版本回退或 lock 钳制
* 触屏拖拽适配
* vitest mock 面的 EP 行为核对（mock 屏蔽，无意义）

## Implementation Plan (small PRs)

* PR1（已完成）：A 类 15 处核对 + B 类 E2E 面 + 去重评估 → research/ 两份产出
* PR2：vitest 三处 Obsidian 两段链守护用例 + AssetsPanel.vue:3177 注释修正 + drop-folder E2E 增补（assets-folder-drag.spec.js 新用例）+ 全量回归（npm test / npm run e2e）
* PR3：共性契约沉淀 docs/spec + CLAUDE.md 关键规则行 + README 确认

## Technical Notes

* 关联：frontend/src/components/*.vue、frontend/e2e/、frontend/src/test/setup.js（ElMessageBox 全局 mock）、wails-mock-defaults.js、vitest.config.js
* 41c4b1a 提交：chore(deps) 2026-09-14，package-lock.json 172 行
* 10-20-e2e 归档：.trellis/tasks/archive/2026-10/10-20-e2e/（bb7a1fa）
* docs/spec/assets-management.md §8.7 统一刷新入口契约
* 真机核对策略：bb7a1fa 已证 EP 2.14.5 重开正常（正向变化），核对重心 = 组件代码分支与用例断言一致性；真机抽验仅限漂移处
