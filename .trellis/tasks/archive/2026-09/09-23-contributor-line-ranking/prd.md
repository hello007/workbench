# 贡献者行数排名

## Goal

补齐 v1.3/v1.4 统计功能遗留子项（docs/路线图.md L209「贡献者统计……不含行数；行数排名留 Phase 2」）：统计面板贡献者卡片新增按作者的代码行数排名（新增/删除行），与现有提交数排名形成双维度视图。数据通道为 `git log --numstat` 按作者聚合，独立 numstat 缓存复用 HEAD SHA + TTL 失效策略，避免大仓每次全量重算。

## Requirements

* 后端：`git log --numstat` 按作者（名+邮箱）聚合新增/删除行数，排除二进制文件行（`-` 标记）
* 后端：独立 numstat 缓存，key 同 `commitHistoryCacheKey`，headSHA 相同 + TTL 内命中，否则全量重拉（行数是聚合值，无增量 prepend 路径，失效判定比提交历史缓存更简单的二态）
* 后端：numstat 拉取受 5000 条上限约束（`-n 5000`），与提交历史 Sampled 采样口径对齐
* 后端：行数排名跟随现有时间档位（7d/30d/90d/1y/all）窗口过滤，与提交数口径一致
* 后端：`model.Contributor` 增加 `Insertions`/`Deletions` 字段，同步 `frontend/wailsjs/` 绑定
* 前端：贡献者卡片加「提交数 / 行数」维度切换（方案 A），行数视图按总变更行数（新增+删除）排序，tooltip 展示 +新增/−删除 细分
* 文档：同步 docs/功能说明.md 统计功能节、docs/路线图.md 勾选该子项

## Acceptance Criteria

* [ ] `git log --numstat` 解析正确聚合各作者新增/删除行数，二进制文件行（`-`）不计入
* [ ] merge commit 无文件统计行，自然计 0 行数，不干扰解析
* [ ] HEAD SHA 未变且 TTL 内重复请求不重新执行 numstat 扫描（缓存命中）；HEAD 变更或 TTL 过期全量重拉
* [ ] 行数排名跟随时间档位切换（7d/30d/90d/1y/all），窗口过滤与提交数统计口径一致
* [ ] >5000 提交仓 numstat 拉取截断至 5000，Sampled 标记不被行数通道破坏
* [ ] 前端贡献者卡片可在提交数/行数双维度间切换，行数视图排序正确、tooltip 显示 +/− 细分
* [ ] `model.Contributor` 字段变更后 `wails generate module` 同步，`npm run build` 通过（无 MISSING_EXPORT）
* [ ] 后端聚合/解析单测覆盖：二进制排除、merge 0 行、窗口过滤、缓存命中/失效；前端组件测试更新
* [ ] docs/功能说明.md、docs/路线图.md 更新（路线图 L209 勾选并改述）

## Definition of Done

* 后端单测 + 前端 vitest 组件测试全绿；`go test ./...`、`npm test`、`npm run build` 通过
* 覆盖率门禁不回退（service ≥76% 基线）
* 文档同步（功能说明、路线图勾选）
* 视觉遵循 frontend-visual-conventions（维度切换器样式与现有档位切换器一致）

## Technical Approach

**后端数据通道（新文件 service/commit_line_stats.go）**

1. CLI 拉取：`git log --numstat -n 5000 --format=<记录分隔符+字段分隔符>`，模式对齐 `commit_history_cli.go` `loadCommitLogBatch`（`util.NewGitCommand().Execute`）
2. 解析状态机：提交头行（%x1e/%x1f 分隔 SHA/author/email/timestamp）→ 文件行 `<ins>\t<del>\t<path>`；ins/del 为 `-` 跳过（二进制）；merge commit 无文件行自然 0 行；rename 路径 `old => new` 不影响（只取数值列）
3. 产出 `[]model.CommitLineStat`（纯值 struct：author/email/timestamp/insertions/deletions，无切片字段，缓存深拷贝可退化为 slice 复制）
4. numstat 缓存：独立小结构（同构 headSHA+TTL 二态判定），key 同 `commitHistoryCacheKey`
5. 纯函数聚合 `AggregateLineStats(records, sinceTs, untilTs)`：按 author+email 分组 sum，按 ins+del 降序；窗口过滤与 `filterCommitsByTime` 同逻辑

**编排（app_git.go GetRepoStats）**

* 现有提交历史解析不动；新增 numstat 缓存三态解析（缓存注入 nil 时直接现拉，对齐 commitHistoryCache==nil 分支模式）
* 两路结果在 app 层按 author+email 合并进 `RepoStats.Contributors`（Insertions/Deletions 填充）；Sampled 取两路 overflow 之或

**model 层**

* `model.Contributor` 加 `Insertions int` / `Deletions int`（json: insertions/deletions）
* 新增 `model.CommitLineStat`；本地 `wails generate module` 同步绑定

**前端**

* `RepoStatsChart.vue`：贡献者卡片加维度切换器（提交数/行数），行数视图新增 `buildContributorLineOption`（utils/repoStatsOptions.js），横向柱状按 ins+del 降序，tooltip 展示 +新增/−删除
* 样式遵循 frontend-visual-conventions（切换器与现有档位切换器视觉一致）

## Decision (ADR-lite)

**决策 1：行数数据通道 = 方案 A 独立 numstat 缓存**（用户确认 2026-09-23）

* Context：`model.Commit` 无行数字段，行数须走 `git log --numstat` 新通道；缓存挂载位置决定侵入面。
* Decision：新建轻量 numstat 缓存条目（key 同 `commitHistoryCacheKey`，headSHA + TTL 同构失效判定），`GetRepoStats` 内与提交历史缓存平行取数；不扩展现有提交历史缓存、不改 `loadCommitLogBatch` 格式。
* Consequences：提交列表页 `GetCommitHistory` 零影响；>5000 仓 go-git 回退路径不受影响；代价是首次访问多一次 numstat 全量遍历（5min TTL + HEAD SHA 失效窗口内零开销）；两套缓存结构并存（行数聚合无增量路径，缓存失效为更简单的二态判定）。

**决策 2：前端展示 = 方案 A 维度切换**（用户确认 2026-09-23）

* Context：贡献者卡片需承载提交数/行数双维度，布局空间有限。
* Decision：卡片内加「提交数/行数」切换器复用同一图表容器；行数排序默认 ins+del 总和，tooltip 细分 +/−；字段层面 Insertions/Deletions 分开存，后续细分展示不改后端。
* Consequences：布局紧凑、`contributorOption` 图表复用度高；单一时刻只见一个维度（接受，并列双图/表格化留后续演进）。

**口径默认（随选项 A 确认一并接受）**

* 行数排序 = 新增+删除总和；窗口联动现有档位；merge commit 计 0 行；贡献者身份不合并（与 git log 作者身份一致）

## Out of Scope

* 按文件/按目录的行数分布（Phase 2+ 另议）
* 贡献者身份合并（同人多邮箱归一）
* 非当前分支（HEAD）之外的行数统计
* numstat 增量 prepend 路径（聚合值无法增量，HEAD 变更即全量重拉）
* 并列双图 / 榜单表格化展示形态（留后续演进）

## Technical Notes

* 关键文件：service/repo_stats.go（聚合骨架）、service/commit_history_cache.go（失效策略先例）、commit_history_cli.go（CLI 解析先例）、app_git.go:210（GetRepoStats 编排）、model/repo_stats.go（Contributor struct）、frontend/src/components/RepoStatsChart.vue、frontend/src/utils/repoStatsOptions.js
* numstat 输出格式：`<ins>\t<del>\t<path>`；`-` 标记二进制须排除；`git log --numstat` 对 merge commit 默认不输出文件统计行
* 时间窗口过滤在内存做（全量数据落缓存，切档位不重扫），与现有 filterCommitsByTime 模式对齐
* 修改 `model/` 导出 struct 字段须同步 `frontend/wailsjs/` 绑定（本地 `wails generate module`；wailsjs 不入库，CI 先 generate 再 build）
* 关键规则关联：test-stability（禁固定端口/隐式外部状态）、cross-layer-contracts（绑定三处同步）、app-services-assembly（无新 service 则不涉及）
