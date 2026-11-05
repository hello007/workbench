# P2 契约与产品决策落地（3 条）

## Goal

按已确定的推荐口径落地三条待决策项，均选择零契约变更或最小变更方案。

## Requirements

### ② Stat 错误归因与提示口径（不改契约）

- `service/filetree.go` `GetRootNodesForPaths` 的 `os.Stat` 失败分支补 `slog.Warn`，记录路径与原始 error（当前无任何日志）。
- **不加 `MissingReason` 字段**：前端统一提示改为「目录不存在或不可访问，可右键移除收藏」，避免「一定是不存在」的误导。
- 不做 `os.IsNotExist` 分流的原因：区分原因必须新增 model 字段并同步 wailsjs 三处，收益仅文案精度，暂不值得扩契约；待后续确实需要按原因做不同操作时再补。
- 验收：网络盘断开时提示不再断言「已移动」；日志可见原始错误。

### ④ 聚合视图下切换工作目录的语义（方案 A：自动退出聚合）

- `frontend/src/views/Home.vue` 的 `onDirectorySelect` 入口：若 `fileTreePanelRef` 处于聚合模式，先 `exitAggregatedMode({ restoreState: false })` 再执行原有切换。
- 命令面板「工作目录」跳转复用同一入口，自动获得该行为。
- 在 `docs/spec/aggregated-workspace.md` 写死：「选择工作目录 = 退出聚合视图并切到该目录；聚合模式仅由工具栏切换按钮进入」。
- 验收：聚合模式下点击左侧任一工作目录，树切回普通树并定位该目录。

### ⑤ 已删除的文件收藏以 Missing 目录占位（不改 Favorite 模型）

- **不给 `model.Favorite` 加 Type 字段**：加字段需改 `Add` 签名并同步 wailsjs 三处，且旧数据无法判定原类型，收益有限。
- 前端把 Missing 占位的提示文案由「目录不存在或已移动」改为「收藏项已失效」，图标与右键菜单保持「移除收藏」，不再暗示这是目录。
- 保留占位而非静默丢弃——用户需要入口清理失效收藏。
- 验收：删除一个文件收藏指向的文件后，聚合树显示「收藏项已失效」占位，可右键移除。

## Acceptance Criteria

- [ ] 后端新增单测：Stat 失败产出占位且写 Warn 日志
- [ ] 前端新增单测：选目录自动退出聚合、失效占位文案
- [ ] `docs/spec/aggregated-workspace.md` 更新两处语义（工作目录切换、失效占位）
- [ ] `go vet ./...`、`go test ./service/ .`、`npm test`、`npm run build` 通过
- [ ] 完成后删除清单中这 3 条，并在 prd 记录「不做」的决策与原因

## Definition of Done

- 不修改 `model/` 导出 struct 字段
- 若实施中发现必须扩契约，退回父任务重新决策
