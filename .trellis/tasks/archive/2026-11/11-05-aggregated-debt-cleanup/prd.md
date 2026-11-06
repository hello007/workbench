# 聚合视图欠账清理总纲（12 条）

## Goal

集中清理聚合视图（虚拟工作区）遗留的 12 条欠账（登记于 `docs/agent-resources/temp-memory/TEMP-MEM-activeContext.md`），按风险与契约影响分三批实施，每批完成后从清单销账。

## 条数与子任务映射

| 子任务 | 欠账条目 | 风险 |
|---|---|---|
| `11-05-aggregated-debt-p0-normalize` | ①git 徽标缓存失效 ③脏收藏展示名 ⑨isFavorited 归一 ⑩onSetAsWorkDir 空操作 ⑪尾分隔符统一 | 低 |
| `11-05-aggregated-debt-p1-interaction` | ⑥菜单基准与空白区 ⑦收藏变化刷新聚合根 ⑧+⑫并发探测 | 中 |
| `11-05-aggregated-debt-p2-contract` | ②Stat 错误归因 ④切换工作目录语义 ⑤失效收藏占位文案 | 中（含产品口径） |

## 执行顺序与依赖

1. **P0 → P1 → P2 串行**。P1 的收藏变化监听建立在 P0 的路径归一口径之上；P2 的失效文案依赖 P1 确定的刷新语义。
2. 每个子任务独立提交、独立销账；父任务在三个子任务全部归档后归档。
3. 每次销账只删除 `TEMP-MEM-activeContext.md` 中本轮已解决的条目，未解决条目保留。

## 全局验收

- [ ] 12 条欠账全部有处理结论（已解决或明确不做并写明原因）
- [ ] `go vet ./...`、`go test ./service/ .-short`、`npm test`、`npm run build` 全绿
- [ ] 涉及 `model/` 字段或 App 方法签名的变更同步 `frontend/wailsjs/` 三处
- [ ] `docs/spec/aggregated-workspace.md` 更新为最终语义
- [ ] `TEMP-MEM-activeContext.md` 完成销账，剩余条目数 ≤ 10
