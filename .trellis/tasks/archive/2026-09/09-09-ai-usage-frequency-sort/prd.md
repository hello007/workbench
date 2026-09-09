# AI 功能项使用频次智能排序

## Goal

在 pinned 手动置顶基础上增加「按使用频次自动排序」维度。排序优先级：pinned > 频次（开关开启时）> 配置文件原序。频次数据复用第 3 批 `AiTaskHistory` 归档（按 functionId 聚合运行次数），无需新存储。

**来源**：docs/plans/2026-09-09-ai-usage-frequency-sort.md（详细设计，待确认点已全部确认）

## Requirements

1. 后端新增 `UsageCounts() (map[string]int, error)`（service/ai_task_history.go），遍历历史元数据聚合 functionId → 次数；**canceled 状态不计入**
2. `app.go` 新增 `GetFunctionUsageCounts` App 方法，同步 `frontend/wailsjs/` 绑定（App.js / App.d.ts / models.ts）
3. `AiFunctionPanel.vue`：搜索框右侧排序模式切换（el-switch 或 el-radio-group）；`filteredFunctions` 排序加入频次维度
4. 频次数据 `loadFunctions` 后异步拉取，late-join 重排，不阻塞首渲染
5. 频次角标（如 `12 次`）**仅频次模式开启时显示**
6. 排序模式选择**持久化到 localStorage**
7. 频次聚合测试（含 canceled 不计入分支）+ 前端频次排序测试

## 已确认决策

| 待确认点 | 决策 |
| --- | --- |
| canceled 是否计入频次 | 不计入（取消=未有效使用） |
| 角标展示策略 | 仅频次模式开启时显示 |
| 排序模式持久化 | localStorage |
| 时间窗口 | 先全量聚合，需求出现再加窗口 |

## Acceptance Criteria

* [ ] 开启频次模式后，未 pinned 功能项按运行次数降序排列，次数相同保持配置文件顺序
* [ ] pinned 始终优先于频次排序
* [ ] 关闭频次模式恢复第 5 批行为（pinned + 原序）
* [ ] canceled 任务不计入运行次数
* [ ] 角标仅频次模式显示；模式关闭时无角标
* [ ] 刷新页面后排序模式保持上次选择（localStorage）
* [ ] 频次数据异步加载，列表先渲染不阻塞
* [ ] `go test ./...` 通过（含新增聚合测试）
* [ ] 前端 `npm test` 通过（含新增排序测试）
* [ ] wailsjs 绑定同步（App.js / App.d.ts）

## Definition of Done

* 测试覆盖后端聚合分支（canceled 排除）
* 前端排序逻辑测试
* README.md / docs/功能说明.md 行为变化更新
* 不破坏现有 pinned 行为

## Out of Scope

* 时间窗口频次（最近 N 天）
* 频次写入配置文件持久化
* 历史数据结构变更

## Technical Approach

排序核心逻辑（AiFunctionPanel.vue `filteredFunctions`）：

```js
// 排序优先级：pinned > 频次（开启时）> 原序
.sort((a, b) => {
  if (!!a.pinned !== !!b.pinned) return a.pinned ? -1 : 1
  if (sortByFrequency.value) {
    const fa = usageCounts.value[a.id] || 0
    const fb = usageCounts.value[b.id] || 0
    if (fa !== fb) return fb - fa
  }
  return 0 // 稳定：保持配置文件顺序
})
```

## Decision (ADR-lite)

**Context**: 频次数据来源有两个选项——前端拉全量历史自行聚合 vs 后端聚合返回 map。
**Decision**: 后端新增 `GetFunctionUsageCounts` 返回 map（设计文档建议）。避免 2000 条历史元数据过 IPC。
**Consequences**: 后端多一个轻量方法；后续加时间窗口只需改后端聚合参数，前端无感。

## Technical Notes

* 设计文档：docs/plans/2026-09-09-ai-usage-frequency-sort.md
* 关联：2026-09-08-ai-run-history-design（历史归档）、第 5 批 pinned（已实施）、2026-09-09-ai-history-stats-export（同样复用历史聚合）
* 关键规则：App 方法签名变更须手动同步 frontend/wailsjs/ 三处（docs/spec/cross-layer-contracts.md）
