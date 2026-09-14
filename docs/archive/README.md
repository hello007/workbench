# 历史文档归档（archive）

> 本目录为 WorkBench 历史设计文档与实施计划归档区，2026-09-14 自 `docs/plans/` 与 `docs/superpowers/{plans,specs}/` 经 `git mv` 迁入，保留 git 历史可追溯，不再维护。

## 用途

存放项目演进过程中已完成或搁置功能的设计文档（design）与实施计划（plan）。这些文档记录了各功能从设计到落地的决策脉络，仅供历史回溯与背景查阅，不再更新内容。

- 活跃规范沉淀请见 `docs/spec/`（跨层契约、测试门禁等持续维护的规范）
- 当前功能说明请见 `docs/功能说明.md` 等顶层文档

## 目录结构

| 目录 | 来源 | 内容 |
|---|---|---|
| `plans/` | 原 `docs/plans/` | 2025-04 起各功能设计文档与实施计划（含 `samples/` 抓样数据） |
| `superpowers/plans/` | 原 `docs/superpowers/plans/` | 2026-05 至 2026-06 superpowers 工作流计划文档 |
| `superpowers/specs/` | 原 `docs/superpowers/specs/` | 对应 superpowers 计划的设计规格文档 |

## 注意事项

- 文档内部相互引用的路径为迁入前的原始路径（如 `docs/plans/...`），属冻结历史记录，未同步更新
- 如需查阅某功能的设计背景，可按文件名日期定位，或通过 `git log --follow <file>` 追溯完整历史
