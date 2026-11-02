# 四提交审核剩余项收口（父任务）

## Goal

`ed3f1b1` / `518ef7f` / `2446ca7` / `aaa2212` 两路审核后的剩余项，按风险拆 4 个子任务逐个实施、独立提交。

## 子任务与顺序

| 顺序 | 子任务 | 级别 | 核心问题 |
|---|---|---|---|
| 1 | `11-02-fold-platform-guard` | P1 | `isReplaceArtifactName` / `foldNameInSet` / `resolveExistingDirCase` 的 `EqualFold` 未做平台守卫，Linux 上会误删用户大小写变体文件 |
| 2 | `11-02-split-cleanup-convergence` | P2 | 旧 `%02d` 页序在 PageCount 内不收敛；超界页删除不清理索引行；`removeSplitArtifactsBeyond` 大小写敏感 |
| 3 | `11-02-relink-backup-hardening` | P2 | replace 链路未接入落点归属复核；备份版本预检不校验 schema（空 SQLite 可清空索引）；DSN 未转义 |
| 4 | `11-02-review-test-doc-anchors` | P3 | 跨卷 Warn 无测试锚定；`splitRecordByOutputDir` 折叠分支/批量 Item.Code/`foldNameInSet` 未测；CLAUDE 行超档；spec 表述反向 |

## 依赖

* 子任务 2/4 依赖子任务 1 的平台守卫单点（`sameNameFold`）。
* 子任务 4 的测试锚定依赖 2/3 的实现细节。
* 父任务只跟踪，不写代码。

## 统一约束

* 折叠语义统一：平台相关折叠一律走单点（仅 Windows 折叠），禁各写 `EqualFold`。
* 新增错误码三处同步；日志分级与既有口径一致。
* 每个子任务：全量测试 + 覆盖率门禁不降 + 文档同步。
