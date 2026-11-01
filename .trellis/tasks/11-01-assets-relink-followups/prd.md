# 素材拆分与重链链路后续收口（父任务）

## Goal

收拢 11-01-relink-split-records / 11-01-relink-split-record-fixes 两轮审核遗留的三项缺口，按影响面拆成三个子任务顺序执行。本任务是规划与追踪节点，**不直接实现代码**；执行在新会话中逐个 child 推进。

## 背景

前两轮完成：文件级重链在 `files.path` 同事务迁移 `split_records.source_rel_path` 键、`output_dir` 冻结为产物实际目录；产物定位收口 `splitArtifactPath`（直拼 + 目录页序反查）；目录级迁移重算落点前加前缀前提校验；`clearLostFiles` 与 force 重链同事务删记录。

审核（两路独立 agent）仍留出以下缺口，已确认可复现或代码可达：

1. **重拆清理按当前主干删旧页**：改名重链后 `removeSplitArtifacts(outDir, baseName)` 用新主干，旧主干产物残留且仍在索引内，可再次进批量源。
2. **产物目录随源搬迁无收敛路径**：用户把源文件连同同名产物目录一起搬走后重链，`output_dir` 仍指旧位置——打开单页失效，再次 replace 拆分对新位置同名产物报「已存在同名文件」，无自动收敛。
3. **`moveFilePath` 拆分键裸 UPDATE**：撞主键时事务回滚而磁盘 rename 已发生，索引与磁盘短暂不一致（既有缺口，键迁移放大占用面）。

## 子任务与执行顺序

| 顺序 | 子任务 | 优先级 | 依赖 |
|---|---|---|---|
| 1 | `11-01-split-cleanup-renamed` 重拆清旧页兼容改名重链 | P1 | 无 |
| 2 | `11-01-relink-artifact-dirsync` 重链时识别产物目录已随源搬迁 | P2 | 需先完成 1（共用主干解析与产物形态判定口径） |
| 3 | `11-01-move-split-key-conflict` moveFilePath 拆分键冲突消解收口 | P2 | 无（可与 2 并行，但按序执行更稳） |

## 执行约定

- 新会话依次处理：读取本任务 `prd.md` → 进入第一个未完成 child → 各自走 trellis 全流程（brainstorm 确认 → start → 实现 → trellis-check → 文档沉淀 → 提交 → `/trellis:finish-work`）。
- 子任务 2 开始前须确认子任务 1 已合并（主干解析单点不被重复实现）。
- 每个子任务完成后更新本任务 `task.json` 的子任务状态记录，全部完成后归档父任务。

## 验收（整体）

- [ ] 三个子任务全部归档。
- [ ] `go test ./...` 全通过；覆盖率门禁不降。
- [ ] `docs/spec/assets-management.md`、`docs/spec/pptx-split.md`、`CLAUDE.md` 相关规则行同步。

## Out of Scope

- 前端交互改动（`UpToDate` 提示消费不在本父任务范围内）。
- `split_records` 表结构变更。
- 与三项缺口无关的重构。
