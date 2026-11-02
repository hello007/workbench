# derived 跨卷旧落点索引清理告警噪音（P2）

## Goal

清理旧落点后调用 `deleteIndexRowsForPaths`，它对每个被删文件执行 `filepath.Rel(lib.RootPath, p)`。当旧落点位于 derived 目录（`data/assets/<id>/derived`，库根之外）：
- 同卷：`Rel` 返回 `../...` 相对路径，查 `files` 表零命中（derived 产物不入索引），静默无副作用；
- 跨卷（data 与库根不同盘符）：`Rel` 报错，每个被删文件打一条「计算旧产物相对路径失败（跳过索引行清理）」Warn —— 纯噪音，审核实测 2 文件 2 条。

## Requirements

1. `deleteIndexRowsForPaths` 对**库根外**路径直接跳过索引行清理（不告警）：先判定 `rel` 是否以 `..` 开头或 `Rel` 出错——视为库外，静默跳过。库内路径行为不变。
2. 库外判定单点辅助 `relPathWithinRoot(root, abs string) (string, bool)`（如需可复用现有越界遏制语义，但不返回错误、只返回是否库内）。
3. 测试：库内路径正常删行；库外（derived）路径零告警零删行；跨卷模拟困难时用 `Rel` 语义直接单测辅助函数 + 断言 logger 无 Warn。
4. 文档：`docs/spec/pptx-split.md §9.5` 或 `assets-management.md` 相关段补「库外旧落点不做索引行清理」。

## Acceptance Criteria

* [ ] 清理 derived 旧落点后无「计算旧产物相对路径失败」Warn（同卷/跨卷均无）。
* [ ] 库内旧落点索引行仍正常删除。
* [ ] `go test ./...` 通过，覆盖率不降。

## Technical Approach

* `deleteIndexRowsForPaths` 计算 `rel` 后：`err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))` → `continue`（Debug 级可选）。
* 不改变 `fileStore.deleteFileRecord` 等其他路径。

## Out of Scope

* 不改缩略图清理逻辑（`invalidateFileThumbs` 仅在库内行命中时调用）。
