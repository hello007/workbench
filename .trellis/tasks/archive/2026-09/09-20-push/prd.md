# 提交历史显示本地与远程分支位置差异 + push 按钮

## Goal

在提交历史视图让用户直观看出「本地分支提交位置」与「远程分支（origin/xxx）提交位置」的差异（领先/落后/分叉点、远程头停在哪条 commit），并提供 push / pull / fetch 操作，把提交历史 header 打造成完整的「本地 vs 远程」同步面板。

## Requirements

1. **摘要条**（CommitHistory header 内新增）：显示 `分支名 ↑ahead ↓behind`；
   * ahead>0 显示 push 按钮，一键推送；
   * behind>0 显示 pull **split 下拉按钮**：主按钮执行「拉取(rebase)」，下拉项「拉取(merge)」；
   * fetch 小按钮：连网更新远程引用后刷新摘要与 badge；
   * 时效提示：behind 基于上次 fetch 快照（轻量文案/tooltip）。
2. **refs badge**（提交行）：最小范围三类标记——`HEAD`、当前分支名、其 upstream（origin/当前分支）；远程头停在哪条 commit 一眼可见；远程头与本地头重合时同行共标。
3. **复用现有链路**：push=PushRepo（含无上游 set-upstream 确认流程）、pull=PullRepo(useRebase)、fetch=FetchRepo；结果反馈对齐 PushResultDialog / gitError 范式。
4. **降级**：无上游（不显示计数，push 走 set-upstream 引导）、detached HEAD（仅 HEAD badge + 提示）、fetch 失败（提示不阻塞）均不报错卡死。
5. push/pull 成功后摘要与 badge 自动刷新（pull 后 HEAD 变化由 CommitHistoryCache SHA 链断裂检测自动全量重扫）。

## Acceptance Criteria

* [ ] CommitHistory header 显示分支名与 ↑ahead/↓behind 计数。
* [ ] 提交行正确标注 HEAD、当前分支、origin/当前分支三类 badge，可看出远程头所在 commit。
* [ ] ahead>0 一键 push；无上游时确认 set-upstream；成功后摘要刷新为 ↑0。
* [ ] behind>0 split 下拉 pull，默认 rebase、可切 merge；成功后提交历史与摘要刷新。
* [ ] fetch 按钮更新远程引用并刷新摘要/badge；网络失败有提示不阻塞。
* [ ] 无上游、detached HEAD、空仓库等边界降级展示，无报错、无 loading 卡死。
* [ ] 后端新逻辑有单测 + 集成测试（对齐 dashboard_integration_test 基准范式）；前端组件测试更新。
* [ ] wailsjs 绑定三处手动同步；README/功能说明.md/API参考.md 更新。

## Definition of Done

* `go test ./...`、`go test -tags=integration ./...`、`cd frontend && npm test` 全绿。
* 新增绑定入 API参考.md（方法清单）；行为变化同步功能说明.md。

## Technical Approach

**后端（新增 1 个绑定，其余全复用）：**

* 新绑定 `GetBranchSyncInfo(path)`：一次 IPC 返回摘要 + refs 映射，避免前端多次调用——
  `{ branch, ahead, behind, hasUpstream, detached, headSha, refs: [{sha, kind, name}] }`；
  * ahead/behind 复用/抽取 `ComputeRepoStatus`（service/dashboard.go:172）的单仓计算逻辑（`git rev-list --left-right --count @{u}...HEAD` 语义），抽共享函数防两处漂移；
  * refs 映射：go-git `repo.References()` 遍历，过滤 HEAD / refs/heads/<当前分支> / refs/remotes/origin/<当前分支>；kind=HEAD|local|remote；
  * 不入 CommitHistoryCache（refs 随 push/fetch 变化，独立轻查询，调用时现算）。
* push/pull/fetch 复用现有 `PushRepo` / `PullRepo` / `FetchRepo`，零后端改动。

**前端：**

* CommitHistory.vue header 加摘要条组件（或子组件 BranchSyncBar）：加载时机=mounted + repoPath 变化 + push/pull/fetch 成功回调；
* 提交行渲染：按 commit.sha 匹配 refs 映射渲染 badge（本地分支/远程分支不同色，语义色对齐 frontend-visual-conventions）；
* pull split 下拉用 el-dropdown；结果反馈复用 PushResultDialog 范式；
* wailsjs 三处手动同步 + `model.BranchSyncInfo` 进 models.ts（若有具名类型须手补别名，见 cross-layer-contracts）。

## Decision (ADR-lite)

**Context**: 需在提交历史看出本地/远程位置差异并支持 push；代码已有 push/pull/fetch/ahead-behind 计算能力，缺的是单仓库 UI 展示与 refs 位置标记。
**Decision**: 摘要条 + refs badge 两者都要（问题 1 选 3）；badge 最小范围三类（问题 2 选 1）；pull 对称提供且 split 下拉默认 rebase（问题 3/4）；手动 fetch 按钮控新鲜度（问题 5 选 1）；单一新绑定 GetBranchSyncInfo 聚合摘要+refs，一次 IPC。
**Consequences**: ContentPanel 现有 rebase-switch（默认 merge）与摘要条（默认 rebase）并存两套默认，本任务不改 ContentPanel；refs 不缓存，每次摘要查询现算（refs 少，成本可忽略）；未来若做全分支 decorate / commit graph，refs kind 枚举可扩展，不阻塞。

## Out of Scope

* commit graph 分支线绘制（Sourcetree 式）。
* 全部分支 + tag 的 decorate 标记。
* 打开仓库/刷新时自动 fetch。
* ContentPanel 现有 pull 开关及其默认值改动。
* LocalChanges 现有 push 入口迁移/去重（两处并存）。

## Technical Notes

* 已复用点：`Push`(service/git.go:616)、`Pull`(service/git.go:137)、`Fetch`(service/git.go:1154)、`HasUpstream`(service/git.go:652)、`ComputeRepoStatus`(service/dashboard.go:172)、PushResultDialog 范式、CommitHistoryCache SHA 链断裂自动全量。
* 集成测试基准：dashboard_integration_test.go 的 `itRevListAheadBehind`（CLI 校验）与本地 bare 远程 fixture 范式可直接复用。
* behind 语义 = 相对上次 fetch（model/dashboard.go:14 注释已声明），前端时效提示文案据此。
* 契约约束：改 `model/` 须同步 `frontend/wailsjs/` 三处；具名 string 类型不生成别名须手补（cross-layer-contracts.md）。
