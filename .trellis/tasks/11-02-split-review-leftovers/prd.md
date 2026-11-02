# 审核剩余项总收口（父任务）

## Goal

`c7f5d58` / `87860af` 两轮审核后剩余 4 组问题，按数据安全优先级拆为 4 个子任务，逐个实施并独立提交。

## 子任务与顺序

| 顺序 | 子任务 | 级别 | 核心问题 |
|---|---|---|---|
| 1 | `11-02-backup-restore-version-precheck` | P0 | 还原无版本预检：版本过新备份先 rename 覆盖库文件再报错，原库与备份双失 |
| 2 | `11-02-split-output-dir-ownership` | P0 | 落点目录归属无校验：A 改名 B 后放入新 A，新 A 拆分删/覆盖 B 产物；`splitArtifactPath` 反查不校验主干 |
| 3 | `11-02-split-failure-semantics` | P1 | source 落点核心失败前已删旧页；Windows 仅大小写改名 replace 重拆必失败；删除失败后记录被覆盖、「下次重拆收敛」不成立 |
| 4 | `11-02-derived-cross-volume-warn-noise` | P2 | derived 旧落点在库根外，`deleteIndexRowsForPaths` 跨卷 `filepath.Rel` 报错，每个文件一条 Warn 噪音 |

## 依赖

* 子任务 3 依赖子任务 2 的 `split_records` 查询能力（`output_dir` 归属查询可复用）。
* 子任务 4 不依赖其他子任务。
* 每个子任务独立 PRD、独立测试、独立提交；父任务仅跟踪，不写代码。

## 统一约束

* 不改前端与 Wails 绑定（若无必要）。
* 新增错误码须三处同步（`model/app_error.go`、`frontend/.../error.js`、文档）。
* 日志用 `log/slog`；测试不得依赖环境外部状态。
* 每个子任务：`go test ./...` 通过 + 覆盖率门禁不降 + 文档同步。

## Out of Scope

* 产物清单表 / 页级哈希（彻底消除同主干同页序误删）。
