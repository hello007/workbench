# brainstorm: 跨仓库研发看板

## Goal

探索「跨多仓库」看板的必要性、可展示内容与实现成本，判断是否值得做、做哪一档。
本阶段只做探索与决策，不实施。

## What I already know（现状盘点）

* 本仓（WorkBench）已有一个一级面板「状态看板」（`frontend/src/views/DashboardView.vue`，`app_dashboard.go`、`service/dashboard.go`），定位是**手动 pin 关注仓库 + 跨仓状态快照表格**。
* 现有看板列：仓库 / 分支 / 工作区脏净与改动数 / 最后提交时间 / stash 数 / 未推送 ahead / 未拉取 behind / 上游缺失 / 跳转 / 取消关注；支持跨工作目录批量添加 pin、失效仓库灰显、异常仓库警示。
* 现有「仓库统计」（`StatsView.vue` + `GetRepoStats(path, rangeKey)`）是**单仓库**提交趋势与贡献者统计，需先在文件树选中仓库，不跨仓聚合。
* 数据获取全部来自本地 git 命令，无 GitHub/GitLab API 依赖；跨仓状态计算已做并发。
* 一级面板全集：工作目录 / AI 功能 / AI 对话 / 工具箱 / 素材管理 / 状态看板 / 仓库统计 / 分支对比。
* 已有可复用的跨仓资产：工作目录注册表（`directories.json`）、仓库筛选器元数据（`repo_meta.json`：简述/标签/README 摘要）、AI 能力框架（AiFunction / AiChat）、素材库、终端。

## Assumptions (temporary)

* 用户所指「不是单独一个仓库的看板」= 跨多仓统一视图，而非单仓详情页。
* 用户可能未把现有「状态看板」视为想要的形态（或希望在其上扩展），需先校准。

## Open Questions

* 【关键分叉】目标形态是哪一类：① 现有状态看板增强 ② 跨仓研发活动看板 ③ 任务/交付看板（PR/Issue/CI）④ 极简「今日风险清单」。
* 主要使用者与使用频率（每日开工?周会汇报?旁观者零散查看?）——决定信息密度与刷新策略。
* 是否接受引入外部平台 API（GitHub/GitLab）与凭据管理成本。

## Requirements (evolving)

* 待形态确认后填写。

## Acceptance Criteria (evolving)

* [ ] 待形态确认后填写。

## Definition of Done (team quality bar)

* 本任务结论：产出「做/不做/做哪一档」的决策记录（ADR-lite），不产出代码。

## Out of Scope (explicit)

* 本阶段不实现任何 UI 或后端采集逻辑。

## Technical Notes

* 现有看板实现：`frontend/src/views/DashboardView.vue`（427 行）、`model/dashboard.go`（RepoStatus）、`app_dashboard.go`、`service/dashboard.go`、`data/dashboard_pinned.json`。
* 现有统计：`app_git.go:229 GetRepoStats(path, rangeKey)`，单仓、最近 5000 条采样上限。
* 性能约束：跨仓扫描成本 = 仓库数 × git 命令数，须复用缓存与并发上限（参考 `docs/spec/perf-baseline.md`）。
* 契约约束：新增 App 方法须同步 `frontend/wailsjs/` 三处；新增带事件 service 须在 `web_serve.go applySink` 补 `SetEventSink`；纯桌面数据面方法须同步 `RPCHiddenMethods`。
