# 看板补全工作树状态列

## Goal

全局状态看板（DashboardView）目前只有分支同步状态（pin 仓库 + ahead/behind），多仓库巡检须逐个点开确认「哪些仓库有未提交变更」。补全工作树状态列：dirty 文件数、最后提交时间、stash 数，一眼完成多仓库健康度巡检。同类参照：Fork / Grape 的 repo overview。

## Requirements

1. **工作区列升级为改动计数**：`RepoStatus` 新增 `DirtyCount`（`git status --porcelain` 输出行数，含 untracked 目录折叠条目，与既有 `Dirty bool` 同一次调用同源同语义）；tag 显示「N 处改动」/「干净」。
2. **新增最后提交时间列**：`RepoStatus` 新增 `LastCommitUnix`（`git log -1 --format=%ct`，Unix 秒）；前端复用 CommitHistory 的 formatTime 口径（X 分钟/小时/天前，超 30 天转日期），hover title 显示完整时间。
3. **新增 stash 数列**：`RepoStatus` 新增 `StashCount`（`git stash list` 输出行数，只读计数）；无 stash 显示 0。
4. **边角降级不阻塞**（对齐既有 ComputeRepoStatus 降级矩阵）：
   - Missing / 非仓库（IsRepo=false）→ 三列均显示「-」
   - 空仓库（unborn，无任何 commit）→ LastCommitUnix=0 → 前端显示「-」；DirtyCount/StashCount 正常计算
   - detached HEAD → 有 HEAD commit，最后提交时间正常显示
   - 单项 git 调用失败 → 该项置零值 + 既有 Error 字段记录，不阻塞整行
5. **wailsjs 契约同步**：`model.RepoStatus` 扩字段后手动同步 `frontend/wailsjs/go/models.ts`（方法签名未变，App.js / App.d.ts 无需改动，须核对确认）。

## Acceptance Criteria

- [ ] 看板表格新增「最后提交」「Stash」两列，工作区列由「有改动/干净」升级为「N 处改动/干净」
- [ ] pin 仓库逐行展示三项新数据；大仓库/无上游/空仓库/detached/Missing 等边角不阻塞整表渲染
- [ ] 后端单测覆盖：dirty 计数、last commit 时间、stash 计数、unborn 降级、单项失败降级
- [ ] 前端契约同步无 MISSING_EXPORT（npm run build 通过）
- [ ] `go vet ./...` / `go test ./service/` / `npm test` 全绿

## Definition of Done

- 后端 service 单测更新（dashboard_test.go）
- 前端 DashboardView.spec.js 更新（新列渲染断言）
- 覆盖率维持门禁：service ≥76%、前端 ≥70%
- README 如涉及看板功能描述须确认是否更新

## Decision (ADR-lite)

**Context**：brainstorm 三条 Open Questions（dirty 口径 / 时间格式 / 性能）+ stash 列接入时机 + 排序，2026-11-07 用户确认按推荐方案。

**Decision**：
1. dirty 口径 = `git status --porcelain` 行数（含 untracked 目录折叠计 1）——`ComputeRepoStatus` 本就跑此命令置 `Dirty bool`，计数零增量成本，语义天然一致；不用 `-uall`（大仓库展开性能风险）。
2. 最后提交时间 = 相对时间展示（复用 CommitHistory formatTime 口径，hover 显绝对时间）；后端传 Unix 秒。
3. stash 数 = 本任务顺手接入只读计数（`git stash list` 毫秒级），stash-management 任务后续做增删操作 UI，数据口径一致不冲突。
4. 排序 = 不做，保持 pin 列表顺序（现有契约「返回顺序与 pin 列表一致」）。
5. 性能 = 维持 `GetStatuses` 并发 8 sem+wg 范式，不做流式渐进；`util.GitCommand.Execute` 自带超时，慢仓库降级 Error 不挂表。

**Consequences**：每仓库新增 2 次轻量 git fork（log -1 / stash list），并发 8 下 20 仓库增量 <100ms；`RepoStatus` 扩 3 字段向后兼容（旧前端忽略新字段）；stash 计数先行落地，stash-management 无需重复计算口径。

## Out of Scope

- 看板排序切换（决策 3A，后续按需）
- stash 增删改/pop/apply 操作 UI（属 11-07-stash-management）
- fetch 增强 / 事件驱动增量刷新（`RefreshDashboardStatuses` 预留口，本任务不动）
- 流式渐进渲染（pin 仓库典型 <20，一次性返回足够）

## Technical Notes

- 涉及面：`model/dashboard.go`（RepoStatus 扩 3 字段）+ `service/dashboard.go`（ComputeRepoStatus 扩计算）+ `service/dashboard_test.go` + `frontend/wailsjs/go/models.ts` + `frontend/src/views/DashboardView.vue`（列渲染 + formatTime）+ `frontend/src/views/__tests__/DashboardView.spec.js`
- 现状锚点：`ComputeRepoStatus` 已在跑 `status --porcelain`（service/dashboard.go:227）；`GetStatuses` 并发 8（service/dashboard.go:151）；`Execute` 自带 timeout（util/git.go:87）
- 相对时间先例：CommitHistory.vue:514 formatTime（timestamp 为 Unix 秒）；绝对时间先例：BranchCompare 的 dayjs.unix(sec)
- 新增 git 调用均为只读，遵循 go-git 出清规则（禁重新引入 go-git）
