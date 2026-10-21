# E2E 覆盖第二批（矩阵候选 #8-#20）+ wails-init 硬化

> 父任务：10-21-verify-upgrade-batch（顺序 3，最后一批）

## Goal

第一批（Top 5 链路，72→87 用例）后收尾矩阵留痕：候选 #8-#20 复核时效后落地第二批 ≤12 用例 + wails-init 事件派发两处低危硬化。

## Requirements

1. **wails-init.js 硬化（先做）**：per-handler try/catch（某 handler 抛错不中断其余）+ EventsOnce 倒序遍历消 splice 索引位移（硬化方向已沉淀 e2e-testing.md §6）；补 1 条基建自证用例（多 handler 含抛错 handler 时其余仍收到事件）
2. **矩阵候选复核**：第一批后组件改动可能使 #8-#20 部分过时/新增（git log 增量核对），出第二批清单（≤12 用例，brainstorm 定）
3. **落地用例**：候选倾向素材 facet 多维筛选 / 批量评分 / 终端 z-index 层叠与全屏 ESC / 看板 pin 链 / AI 对话选择题卡片（以矩阵为准）
4. **两遗留评估面**：EventsOn 闭包注销缓冲区断言、KillProcess 强杀链（pty 事件驱动缺 bound method 挂点）——评估可行性后实施或明确留痕
5. **文档同步**：测试策略.md 数字、e2e-testing.md 按需、矩阵状态标注

## Acceptance Criteria

- [x] wails-init 硬化 + 自证用例绿（dispatch 快照正序 try/catch + EventsOn/OnMultiple/Once 注销闭包；wails-mock.spec.js 3 条自证）
- [x] 第二批用例落地全绿（12 条：矩阵 #8/9/11/12/13/14/15/17/18/19 + 无编号 2 条全屏 ESC/AI 选择题，另强杀链收口 #4 留痕；web 87→107）
- [x] vitest / E2E 全量零回归（vitest 96 文件 2022 用例 / E2E web 107 / desktop 3）
- [x] 两遗留面有结论（均实施：EventsOn 闭包契约补齐+自证；KillProcess 强杀链挂点存在已落地）
- [x] 文档与矩阵状态同步（测试策略 32 spec/107、e2e-testing §6 收口+嵌套限制留痕、归档矩阵 §4.2、AssetFilterPopover 注释纠偏）

## Definition of Done

- AC 全勾；CI 全 job 绿

## Open Questions（brainstorm 待定）

- 第二批清单（矩阵复核后定）
- 两遗留面可行性的评估结论

## Out of Scope

- desktop 通道扩容
- 后端集成测试扩展

## Research References

- 矩阵：archive/2026-10/10-21-e2e-coverage/research/coverage-matrix.md（§4.1 候选状态）
- 硬化点：docs/spec/e2e-testing.md §6（forEach 中断 + EventsOnce 索引位移）
