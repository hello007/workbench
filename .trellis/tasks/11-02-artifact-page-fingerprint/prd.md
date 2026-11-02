# 拆分产物页级指纹（P2，需 brainstorm 选型）

## Goal

`cleanupSplitArtifactsInDirByPage` 现靠「记录主干 + 页序 + `mtime > split_at + 2s`」判定旧产物。用户文件若与产物同主干、同页序且 mtime 早于 `split_at`（复制、解压、手工改时间），仍会被删除，且不可恢复。

需引入页级指纹，使「是否为该记录产物」成为可验证事实，而非启发式。

## Requirements

1. **先 brainstorm 出 2-3 个方案**，至少覆盖：
   - a) `split_records` 增页级清单（页号/文件名/大小/哈希）JSON 列；
   - b) 独立表 `split_artifacts`（source_rel_path, page, name, size, hash）；
   - c) 仅存产物页内容 hash（重算成本与 OOXML 确定性评估）；
   - d) 维持现状（说明为何风险可接受）。
   每个方案给：误删率、漏清率、迁移成本、重拆/重链/目录迁移的同步点、失败回滚复杂度。
2. 推荐方案须明确：
   - schema 版本（v10）与迁移路径（列存在性预判 + 单事务）；
   - 写入时机（拆分成功后）；
   - 清理判定改用指纹的顺序（指纹缺失时的回落策略）；
   - 重拆页数变化、产物改名（大小写）、目录迁移、备份还原时的指纹一致性；
   - 指纹计算算法（页文件全文 SHA-256 或大小+mtime 组合）与性能预算。
3. 测试：指纹命中删除、指纹不符保留、指纹缺失回落、迁移/重链/还原后仍命中。
4. 文档：`docs/spec/pptx-split.md §9.5`、`docs/spec/assets-management.md` schema 章节。

## Acceptance Criteria

* [ ] brainstorm 结论写入 PRD（ADR），并经用户确认。
* [ ] 用户自建同主干同页序文件（任意 mtime）不再被删。
* [ ] 正常旧产物照常清理，无漏清回归；`go test ./...` 与覆盖率门禁通过。

## Out of Scope

* 不做页内容预览/缩略图联动改造。
* 不改变现有 mtime 守卫（指纹缺失时仍作为回落）。
