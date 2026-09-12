# 仓库统计审核修复

## Goal

修复仓库统计功能 code-review 高强度审核发现的 10 个 findings（4 correctness + 2 efficiency + 4 cleanup/altitude），消除用户可感知缺陷与维护隐患。

## Requirements

### Correctness（必修）

1. **Home.vue:8 main-panes 互斥**：v-show 条件 `activePanel !== 'ai'` 未排除 'stats'，stats 激活时三栏区仍 visible 争抢空间。改白名单 `activePanel === 'directory' || activePanel === 'toolbox'`，与 AiFunctionPanel 对称。
2. **app_git.go:231 吞错误**：超限回退 `all, _ = fetchCommitHistoryFromGit(...)` 丢弃 error，git 故障误显为「采样+0 提交」。fetchCommitHistoryFromGit 返 error 时直接 return。
3. **StatsView.vue:67 请求竞态**：loadStats 无串行化，连点档位旧响应覆盖新。加 requestSeq，仅采纳最新请求结果。
4. **StatsView.vue:87 误触发**：watch(repoPath) 全局触发，非统计页也打后端。loadStats 开头加 `if (uiStore.activePanel !== 'stats') return` guard。

### Efficiency（应修，合并 #8 根因）

5. **app_git.go:228 双扫 + 双分支重复**：fullScanCommits 已扫 5001 条丢弃，再调 fetchCommitHistoryFromGit 二次扫。改 fullScanCommits 返 `(commits, overflow bool)`，overflow 时 GetRepoStats 直接用已收集前 5000 条作采样，消除双扫 + 合并两分支重复采样逻辑。
6. **service/repo_stats.go:178 aggregateHeatmap 不 break**：遍历全量建 map 但仅输出 365 天。commits 倒序，加 `if c.Timestamp < sinceTs { break }`。

### Cleanup/Altitude（建议）

7. **service/repo_stats.go:79 1y 闰年/DST**：duration=365*24h 闰年少一天、DST 差 1 小时。1y 改 `now.AddDate(-1,0,0)`，短期档可接受或同用 AddDate。
9. **repoStatsOptions.js:96 reverse 耦合**：authors/counts 各 reverse + tooltip `length-1-idx` 三处耦合。改 yAxis `inverse: true` 消除 reverse，tooltip 直接 `data[p.dataIndex]`。
10. **service/repo_stats.go:199 死代码**：ResolveStatsRangeLabel 无生产调用。删函数 + 测试。

## Acceptance Criteria

* [ ] 点活动栏「仓库统计」三栏区隐藏、StatsView 占满上半区
* [ ] git 仓库故障时前端显示「加载失败」而非「采样+0 提交」
* [ ] 连点档位/快速切仓库，stats 始终匹配最新选中态
* [ ] 非统计页选中文件树节点不触发 GetRepoStats
* [ ] 超限仓库（>5000）单次扫描不双扫
* [ ] go test ./... 全过，service 覆盖率 ≥76%
* [ ] 前端 vitest 全过，覆盖率 ≥70%
* [ ] 跨层契约 wailsjs 三处一致（改签名才需同步，本任务不改 App 签名）

## Out of Scope

* 行数统计（Phase 2）
* StatsView 抽 useAsyncData composable（13+ 组件同模式，单独任务做）

## Technical Notes

* 审核原始 findings 见上一轮 code-review 输出
* #8（双分支采样重复）是 #2/#5 根因，合入 #5 一起修
* fullScanCommits 返 overflow 需同步改 GetCommitHistory 调用点（仅判 nil → 判 overflow）
