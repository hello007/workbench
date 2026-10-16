# 字段加权方向实测核对（SRS §2.5 附带项）

**结论：方向一致，无需校准，不动权重。** 2026-10-16（任务 10-16-facet-srs-2-5 PR1）。

- 权重定义单点 `service/assets_db.go` `assetsFtsBm25Args = "0.0, 0.0, 0.0, 4.0, 1.0, 3.0, 2.0, 1.0"`（列序 = search_fts 定义序，UNINDEXED 三列占位 0），与 SRS 方向一致：file_name 4.0 > slide_title 3.0 > slide_text 2.0 > file_note/slide_notes 1.0。
- 实测证据（bm25 负值口径，分值更小 = 更相关）：
  - 文件名(4.0) > 标题(3.0)：`TestAssetSearch_FileNameWeightOverTitle` 通过（文件条目严格排在幻灯片标题命中之前）；
  - 标题(3.0) > 正文(2.0)：`TestAssetSearch_AggregateAndFlatten` 通过（标题命中页排首位，聚合条目取标题命中页为最高分页）；
  - 正文(2.0) > 备注(1.0)：临时验证用例实测通过（slide_notes 命中排 slide_text 命中之后），既有断言未覆盖备注列，本次一次性补测后删除（未固化，避免为非契约面增加维护负担）。
- 偏差处理：无偏差，未改任何权重；「标题命中 vs 正文命中」既有分层断言保持不变。
