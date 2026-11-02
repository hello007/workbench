# 测试锚定与文档/规则治理（P3）

## Goal

补齐前三个子任务与四提交审核中「修复无测试锚定 / 文档表述反向 / CLAUDE 行超档」的剩余项。

## Requirements

1. **跨卷告警锚定**：`deleteIndexRowsForPaths` 库外跨卷分支（`filepath.Rel` 出错）无有效测试（现用例只走同卷 `..`）。补 Windows 不同盘符用例（无第二盘则 `t.Skip`），断言无 Warn、有 Debug 留痕；非 Windows 用相对根 + 绝对路径触发 `Rel` 出错。
2. **缺口单测**：
   - `splitRecordByOutputDir` 同源大小写折叠排除分支（Windows）；
   - 批量入口 `Item.Code == E_ASSETS_SPLIT_OUTPUT_CONFLICT`；
   - `foldNameInSet` 表驱动直测；
   - `resolveExistingDirCase` 非 Windows 原样返回。
3. **CLAUDE 行治理**：`CLAUDE.md` 关键规则行中超档者（实测 `:123` 328 字符 5 钩子、`:124` 349 字符 6 钩子，均按普通行 ≤250 判超）下沉机理细节到 `docs/spec/pptx-split.md`，行内只留钩子；治理后逐行实测 ≤250 或钩子 ≥7 且 ≤350。
4. **文档修正**：
   - `docs/spec/pptx-split.md` 平台表述（「大小写敏感平台 ENOENT」应改为「Windows 不折叠会导致按新名定位同一物理文件失败」）；
   - `app_assets.go` `SplitAssetFile` 注释补 `E_ASSETS_SPLIT_OUTPUT_CONFLICT`；
   - `cleanupStaleReplaceArtifacts` 删除失败改用 `removeSplitArtifactFile`（重试一次）或把规则表述限定为仅该两处，二者择一并同步 spec 与 CLAUDE 行；
   - `removeSplitArtifactsBeyond`/`cleanupSplitArtifactsInDirByPage` 注释与 `%02d` 收敛口径（若子任务 2 已改则复核）。
5. 测试与文档同步后跑全量。

## Acceptance Criteria

* [ ] 上述每个缺口至少一条用例，回退对应实现时变红（跨卷用 skip 条件说明）。
* [ ] CLAUDE 各行实测字符数记录在报告中，全部合规。
* [ ] `go test ./...` 与覆盖率门禁通过；前端测试通过。

## Out of Scope

* 不重构 CLAUDE 规则表整体结构，只治理超档行。
