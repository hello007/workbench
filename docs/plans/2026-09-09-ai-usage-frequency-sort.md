# AI 功能项使用频次智能排序

**日期**：2026-09-09
**优先级**：P2
**状态**：待评审

## 1. 概述

第 5 批已实现 pinned 手动置顶（`AiFunction.Pinned`），功能项继续增长后手动维护置顶成本上升。
本设计在 pinned 基础上增加「按使用频次自动排序」维度，与 pinned 并存：pinned 优先 > 频次 > 原序。
频次数据复用历史归档（第 3 批 `AiTaskHistory` 按 functionId 聚合运行次数），无需新存储。

## 2. 现状与痛点

`AiFunctionPanel.vue` 第 5 批改造后的 `filteredFunctions`（搜索+tag 筛选后）排序逻辑：
pinned 优先，同组按配置文件顺序（稳定排序）。无频次维度——高频功能若未手动置顶，仍与低频混排。

| 痛点 | 说明 |
| --- | --- |
| 手动置顶维护成本 | 功能增多后需手动调整 pinned，跟不上实际使用变化 |
| 频次数据闲置 | `AiTaskHistory` 已有 functionId，可聚合频次，但排序未用 |

## 3. 需求总结

1. 功能项列表增加「按频次排序」选项（开关或排序模式下拉）
2. 频次 = 该功能项在历史归档中的运行次数（success/failed/timeout 计入，canceled 是否计入待确认）
3. 排序优先级：pinned > 频次（开关开启时）> 配置文件顺序
4. 频次数据加载时不阻塞列表渲染（列表先渲染，频次到后重排）

## 4. 设计要点

### 4.1 频次数据来源

复用 `AiTaskHistoryService.List(nil)` 全量历史，前端按 functionId 聚合计数。
或后端新增轻量方法 `GetFunctionUsageCounts() map[string]int`（直接遍历历史元数据聚合，
返回 functionId → 次数 map）。建议后者：避免全量历史过 IPC（元数据虽轻但 2000 条仍冗余）。

### 4.2 排序逻辑

`AiFunctionPanel.vue` 的 `filteredFunctions` 排序增加频次维度：

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

### 4.3 UI 交互

搜索框右侧加排序模式切换（`el-radio-group` 或 `el-switch`）：
- 默认：pinned + 原序（第 5 批行为，不破坏现有体验）
- 频次模式：pinned + 频次降序 + 原序

频次数据在 `loadFunctions` 后异步拉取（`GetFunctionUsageCounts`），到达后触发重排。
列表先渲染不阻塞，频次 late-join。

### 4.4 频次展示

卡片上可选展示运行次数角标（如 `12 次`），让用户感知排序依据。待确认是否展示——
避免视觉噪音，建议仅在频次模式开启时展示。

## 5. 改动范围

| 文件 | 改动类型 | 说明 |
| --- | --- | --- |
| `service/ai_task_history.go` | 修改 | 新增 `UsageCounts() (map[string]int, error)` 方法 |
| `service/ai_task_history_test.go` | 修改 | 频次聚合测试（含 canceled 计入/不计入分支） |
| `app.go` | 修改 | 新增 `GetFunctionUsageCounts` App 方法 |
| `frontend/wailsjs/go/main/App.js` + `App.d.ts` | 同步 | 新方法签名同步 |
| `frontend/src/components/AiFunctionPanel.vue` | 修改 | 排序模式切换 + 频次维度排序 + 异步拉取 |
| `frontend/src/components/__tests__/AiFunctionPanel.spec.js` | 修改 | 频次排序测试 |

## 6. 待确认点

1. canceled 任务是否计入频次？建议不计入（用户取消表示未有效使用）
2. 频次展示：卡片角标始终显示 vs 仅频次模式显示 vs 不显示？建议仅频次模式显示
3. 排序模式是否持久化（记住用户上次选择）？建议持久化到 localStorage（轻量，不入配置文件）
4. 是否需要时间窗口（如「最近 30 天频次」而非全量）？建议先全量，需求出现再加窗口

## 7. 关联

- 来源：[[2026-09-08-ai-optimization-overview]] 后续优化建议
- 依赖：[[2026-09-08-ai-run-history-design]]（历史归档数据）、第 5 批 pinned 排序（已实施）
- 配套：[[2026-09-09-ai-history-stats-export]]（同样复用历史数据聚合）
